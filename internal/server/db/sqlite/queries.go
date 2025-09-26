package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ccheshirecat/volant/internal/server/db"
)

var timestampLayouts = []string{
	"2006-01-02 15:04:05",
	time.RFC3339,
	time.RFC3339Nano,
}

// executor abstracts *sql.DB and *sql.Tx for shared query logic.
type executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type queries struct {
	exec executor
}

var _ db.Queries = (*queries)(nil)

func (q *queries) VirtualMachines() db.VMRepository {
	return &vmRepository{exec: q.exec}
}

func (q *queries) IPAllocations() db.IPRepository {
	return &ipRepository{exec: q.exec}
}

func (q *queries) Plugins() db.PluginRepository {
	return &pluginRepository{exec: q.exec}
}

type vmRepository struct {
	exec executor
}

var _ db.VMRepository = (*vmRepository)(nil)

type rowScanner interface {
	Scan(dest ...any) error
}

func (r *vmRepository) Create(ctx context.Context, vm *db.VM) (int64, error) {
	pidVal := nullableInt64(vm.PID)
	cmdlineVal := nullableString(vm.KernelCmdline)

	res, err := r.exec.ExecContext(
		ctx,
		`INSERT INTO vms (name, status, runtime, pid, ip_address, mac_address, cpu_cores, memory_mb, kernel_cmdline)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		vm.Name,
		string(vm.Status),
		vm.Runtime,
		pidVal,
		vm.IPAddress,
		vm.MACAddress,
		vm.CPUCores,
		vm.MemoryMB,
		cmdlineVal,
	)
	if err != nil {
		return 0, fmt.Errorf("insert vm: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("vm last insert id: %w", err)
	}
	return id, nil
}

func (r *vmRepository) GetByName(ctx context.Context, name string) (*db.VM, error) {
	row := r.exec.QueryRowContext(ctx, `SELECT id, name, status, runtime, pid, ip_address, mac_address, cpu_cores, memory_mb, kernel_cmdline, created_at, updated_at FROM vms WHERE name = ?;`, name)
	vm, err := scanVM(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &vm, nil
}

func (r *vmRepository) List(ctx context.Context) ([]db.VM, error) {
	rows, err := r.exec.QueryContext(ctx, `SELECT id, name, status, runtime, pid, ip_address, mac_address, cpu_cores, memory_mb, kernel_cmdline, created_at, updated_at FROM vms ORDER BY created_at ASC;`)
	if err != nil {
		return nil, fmt.Errorf("query vms: %w", err)
	}
	defer rows.Close()

	var result []db.VM
	for rows.Next() {
		vm, err := scanVM(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, vm)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vms: %w", err)
	}
	return result, nil
}

func (r *vmRepository) UpdateRuntimeState(ctx context.Context, id int64, status db.VMStatus, pid *int64) error {
	pidVal := nullableInt64(pid)
	if _, err := r.exec.ExecContext(ctx, `UPDATE vms SET status = ?, pid = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;`, string(status), pidVal, id); err != nil {
		return fmt.Errorf("update vm runtime state: %w", err)
	}
	return nil
}

func (r *vmRepository) UpdateKernelCmdline(ctx context.Context, id int64, cmdline string) error {
	if _, err := r.exec.ExecContext(ctx, `UPDATE vms SET kernel_cmdline = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;`, nullableString(cmdline), id); err != nil {
		return fmt.Errorf("update vm cmdline: %w", err)
	}
	return nil
}

func (r *vmRepository) Delete(ctx context.Context, id int64) error {
	if _, err := r.exec.ExecContext(ctx, `DELETE FROM vms WHERE id = ?;`, id); err != nil {
		return fmt.Errorf("delete vm: %w", err)
	}
	return nil
}

type ipRepository struct {
	exec executor
}

var _ db.IPRepository = (*ipRepository)(nil)

func (r *ipRepository) EnsurePool(ctx context.Context, ips []string) error {
	for _, ip := range ips {
		if _, err := r.exec.ExecContext(ctx, `INSERT OR IGNORE INTO ip_allocations (ip_address, status) VALUES (?, ?);`, ip, string(db.IPStatusAvailable)); err != nil {
			return fmt.Errorf("ensure pool entry %s: %w", ip, err)
		}
	}
	return nil
}

func (r *ipRepository) LeaseNextAvailable(ctx context.Context) (*db.IPAllocation, error) {
	row := r.exec.QueryRowContext(ctx, `SELECT ip_address FROM ip_allocations WHERE status = ? ORDER BY ip_address ASC LIMIT 1;`, string(db.IPStatusAvailable))
	var ip string
	if err := row.Scan(&ip); err != nil {
		if err == sql.ErrNoRows {
			return nil, db.ErrNoAvailableIPs
		}
		return nil, fmt.Errorf("select next available ip: %w", err)
	}

	if _, err := r.exec.ExecContext(ctx, `UPDATE ip_allocations SET status = ?, vm_id = NULL, leased_at = CURRENT_TIMESTAMP WHERE ip_address = ?;`, string(db.IPStatusLeased), ip); err != nil {
		return nil, fmt.Errorf("mark ip leased: %w", err)
	}

	return r.Lookup(ctx, ip)
}

func (r *ipRepository) LeaseSpecific(ctx context.Context, ip string) (*db.IPAllocation, error) {
	res, err := r.exec.ExecContext(ctx, `UPDATE ip_allocations SET status = ?, vm_id = NULL, leased_at = CURRENT_TIMESTAMP WHERE ip_address = ? AND status = ?;`, string(db.IPStatusLeased), ip, string(db.IPStatusAvailable))
	if err != nil {
		return nil, fmt.Errorf("lease specific ip: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("lease specific rows affected: %w", err)
	}
	if affected == 0 {
		return nil, db.ErrNoAvailableIPs
	}
	return r.Lookup(ctx, ip)
}

func (r *ipRepository) Assign(ctx context.Context, ip string, vmID int64) error {
	res, err := r.exec.ExecContext(ctx, `UPDATE ip_allocations SET vm_id = ? WHERE ip_address = ? AND status = ?;`, vmID, ip, string(db.IPStatusLeased))
	if err != nil {
		return fmt.Errorf("assign ip: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return fmt.Errorf("assign ip: no rows affected")
	} else if err != nil {
		return fmt.Errorf("assign ip rows affected: %w", err)
	}
	return nil
}

func (r *ipRepository) Release(ctx context.Context, ip string) error {
	if _, err := r.exec.ExecContext(ctx, `UPDATE ip_allocations SET status = ?, vm_id = NULL, leased_at = NULL WHERE ip_address = ?;`, string(db.IPStatusAvailable), ip); err != nil {
		return fmt.Errorf("release ip: %w", err)
	}
	return nil
}

func (r *ipRepository) Lookup(ctx context.Context, ip string) (*db.IPAllocation, error) {
	row := r.exec.QueryRowContext(ctx, `SELECT ip_address, vm_id, status, leased_at FROM ip_allocations WHERE ip_address = ?;`, ip)
	alloc, err := scanIP(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &alloc, nil
}

type pluginRepository struct {
	exec executor
}

var _ db.PluginRepository = (*pluginRepository)(nil)

func (r *pluginRepository) Upsert(ctx context.Context, plugin db.Plugin) error {
	meta := plugin.Metadata
	if meta == nil {
		meta = []byte{}
	}
	_, err := r.exec.ExecContext(ctx, `INSERT INTO plugins (name, version, enabled, metadata, installed_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET version = excluded.version, enabled = excluded.enabled, metadata = excluded.metadata, updated_at = CURRENT_TIMESTAMP;`,
		plugin.Name, plugin.Version, boolToInt(plugin.Enabled), meta,
	)
	if err != nil {
		return fmt.Errorf("upsert plugin: %w", err)
	}
	return nil
}

func (r *pluginRepository) List(ctx context.Context) ([]db.Plugin, error) {
	rows, err := r.exec.QueryContext(ctx, `SELECT id, name, version, enabled, metadata, installed_at, updated_at FROM plugins ORDER BY name ASC;`)
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}
	defer rows.Close()

	var result []db.Plugin
	for rows.Next() {
		plugin, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, plugin)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugins: %w", err)
	}
	return result, nil
}

func (r *pluginRepository) GetByName(ctx context.Context, name string) (*db.Plugin, error) {
	row := r.exec.QueryRowContext(ctx, `SELECT id, name, version, enabled, metadata, installed_at, updated_at FROM plugins WHERE name = ?;`, name)
	plugin, err := scanPlugin(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &plugin, nil
}

func (r *pluginRepository) SetEnabled(ctx context.Context, name string, enabled bool) error {
	res, err := r.exec.ExecContext(ctx, `UPDATE plugins SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?;`, boolToInt(enabled), name)
	if err != nil {
		return fmt.Errorf("set plugin enabled: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	} else if err != nil {
		return fmt.Errorf("set plugin enabled rows: %w", err)
	}
	return nil
}

func (r *pluginRepository) Delete(ctx context.Context, name string) error {
	res, err := r.exec.ExecContext(ctx, `DELETE FROM plugins WHERE name = ?;`, name)
	if err != nil {
		return fmt.Errorf("delete plugin: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	} else if err != nil {
		return fmt.Errorf("delete plugin rows: %w", err)
	}
	return nil
}

func scanVM(row rowScanner) (db.VM, error) {
	var (
		vm         db.VM
		status     string
		runtime    sql.NullString
		pid        sql.NullInt64
		cmdline    sql.NullString
		createdRaw any
		updatedRaw any
	)

	if err := row.Scan(
		&vm.ID,
		&vm.Name,
		&status,
		&runtime,
		&pid,
		&vm.IPAddress,
		&vm.MACAddress,
		&vm.CPUCores,
		&vm.MemoryMB,
		&cmdline,
		&createdRaw,
		&updatedRaw,
	); err != nil {
		if err == sql.ErrNoRows {
			return db.VM{}, err
		}
		return db.VM{}, fmt.Errorf("scan vm: %w", err)
	}

	vm.Status = db.VMStatus(status)
	if runtime.Valid {
		vm.Runtime = runtime.String
	}
	if pid.Valid {
		value := pid.Int64
		vm.PID = &value
	}
	if cmdline.Valid {
		vm.KernelCmdline = cmdline.String
	}

	createdAt, err := coerceTime(createdRaw)
	if err != nil {
		return db.VM{}, fmt.Errorf("parse vm created_at: %w", err)
	}
	updatedAt, err := coerceTime(updatedRaw)
	if err != nil {
		return db.VM{}, fmt.Errorf("parse vm updated_at: %w", err)
	}
	vm.CreatedAt = createdAt
	vm.UpdatedAt = updatedAt
	return vm, nil
}

func scanIP(row rowScanner) (db.IPAllocation, error) {
	var (
		ip     db.IPAllocation
		status string
		vmID   sql.NullInt64
		leased any
	)

	if err := row.Scan(&ip.IPAddress, &vmID, &status, &leased); err != nil {
		if err == sql.ErrNoRows {
			return db.IPAllocation{}, err
		}
		return db.IPAllocation{}, fmt.Errorf("scan ip allocation: %w", err)
	}
	ip.Status = db.IPStatus(status)
	if vmID.Valid {
		value := vmID.Int64
		ip.VMID = &value
	}
	if leased != nil {
		ts, err := coerceTime(leased)
		if err != nil {
			return db.IPAllocation{}, fmt.Errorf("parse leased_at: %w", err)
		}
		ip.LeasedAt = &ts
	}
	return ip, nil
}

func scanPlugin(row rowScanner) (db.Plugin, error) {
	var (
		plugin      db.Plugin
		enabledInt  int64
		metadataRaw []byte
		installed   any
		updated     any
	)

	if err := row.Scan(
		&plugin.ID,
		&plugin.Name,
		&plugin.Version,
		&enabledInt,
		&metadataRaw,
		&installed,
		&updated,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.Plugin{}, sql.ErrNoRows
		}
		return db.Plugin{}, fmt.Errorf("scan plugin: %w", err)
	}

	plugin.Enabled = enabledInt != 0
	plugin.Metadata = append([]byte(nil), metadataRaw...)
	plugin.InstalledAt, _ = parseTimeField(installed)
	plugin.UpdatedAt, _ = parseTimeField(updated)
	return plugin, nil
}

func parseTimeField(value any) (time.Time, error) {
	if value == nil {
		return time.Time{}, fmt.Errorf("time field nil")
	}
	switch v := value.(type) {
	case time.Time:
		return v, nil
	case string:
		for _, layout := range timestampLayouts {
			if t, err := time.Parse(layout, v); err == nil {
				return t, nil
			}
		}
	case []byte:
		str := string(v)
		for _, layout := range timestampLayouts {
			if t, err := time.Parse(layout, str); err == nil {
				return t, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time value %T", value)
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func coerceTime(value any) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v.UTC(), nil
	case string:
		for _, layout := range timestampLayouts {
			if t, err := time.ParseInLocation(layout, v, time.UTC); err == nil {
				return t.UTC(), nil
			}
		}
		return time.Time{}, fmt.Errorf("unrecognised time format: %q", v)
	case []byte:
		s := string(v)
		for _, layout := range timestampLayouts {
			if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
				return t.UTC(), nil
			}
		}
		return time.Time{}, fmt.Errorf("unrecognised time format bytes: %q", s)
	case nil:
		return time.Time{}, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time type %T", value)
	}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
