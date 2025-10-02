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

func (q *queries) VMConfigs() db.VMConfigRepository {
	return &vmConfigRepository{exec: q.exec}
}

func (q *queries) VMGroups() db.VMGroupRepository {
	return &vmGroupRepository{exec: q.exec}
}

func (q *queries) PluginArtifacts() db.PluginArtifactRepository {
	return &pluginArtifactRepository{exec: q.exec}
}

func (q *queries) VMCloudInit() db.VMCloudInitRepository {
	return &vmCloudInitRepository{exec: q.exec}
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
	serialVal := nullableString(vm.SerialSocket)
	groupVal := nullableInt64(vm.GroupID)

	res, err := r.exec.ExecContext(
		ctx,
		`INSERT INTO vms (name, status, runtime, pid, ip_address, mac_address, cpu_cores, memory_mb, kernel_cmdline, serial_socket, group_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		vm.Name,
		string(vm.Status),
		vm.Runtime,
		pidVal,
		vm.IPAddress,
		vm.MACAddress,
		vm.CPUCores,
		vm.MemoryMB,
		cmdlineVal,
		serialVal,
		groupVal,
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
	row := r.exec.QueryRowContext(ctx, `SELECT id, name, status, runtime, pid, ip_address, mac_address, cpu_cores, memory_mb, kernel_cmdline, serial_socket, group_id, created_at, updated_at FROM vms WHERE name = ?;`, name)
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
	rows, err := r.exec.QueryContext(ctx, `SELECT id, name, status, runtime, pid, ip_address, mac_address, cpu_cores, memory_mb, kernel_cmdline, serial_socket, group_id, created_at, updated_at FROM vms ORDER BY created_at ASC;`)
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

func (r *vmRepository) ListByGroupID(ctx context.Context, groupID int64) ([]db.VM, error) {
	rows, err := r.exec.QueryContext(ctx, `SELECT id, name, status, runtime, pid, ip_address, mac_address, cpu_cores, memory_mb, kernel_cmdline, serial_socket, group_id, created_at, updated_at FROM vms WHERE group_id = ? ORDER BY name ASC;`, groupID)
	if err != nil {
		return nil, fmt.Errorf("query vms by group: %w", err)
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
		return nil, fmt.Errorf("iterate vms by group: %w", err)
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

func (r *vmRepository) UpdateSockets(ctx context.Context, id int64, serial string) error {
	if _, err := r.exec.ExecContext(ctx, `UPDATE vms SET serial_socket = ?, console_socket = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?;`, nullableString(serial), id); err != nil {
		return fmt.Errorf("update vm sockets: %w", err)
	}
	return nil
}

func (r *vmRepository) UpdateSpec(ctx context.Context, id int64, runtime string, cpuCores, memoryMB int, kernelCmdline string) error {
	if _, err := r.exec.ExecContext(ctx, `UPDATE vms SET runtime = ?, cpu_cores = ?, memory_mb = ?, kernel_cmdline = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;`, runtime, cpuCores, memoryMB, nullableString(kernelCmdline), id); err != nil {
		return fmt.Errorf("update vm spec: %w", err)
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

type vmGroupRepository struct {
	exec executor
}

var _ db.VMGroupRepository = (*vmGroupRepository)(nil)

type pluginArtifactRepository struct {
	exec executor
}

var _ db.PluginArtifactRepository = (*pluginArtifactRepository)(nil)

type vmCloudInitRepository struct {
	exec executor
}

var _ db.VMCloudInitRepository = (*vmCloudInitRepository)(nil)

type vmConfigRepository struct {
	exec executor
}

var _ db.VMConfigRepository = (*vmConfigRepository)(nil)

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

func (r *vmGroupRepository) Create(ctx context.Context, group *db.VMGroup) (int64, error) {
	res, err := r.exec.ExecContext(ctx, `INSERT INTO vm_groups (name, config_json, replicas) VALUES (?, ?, ?);`, group.Name, string(group.ConfigJSON), group.Replicas)
	if err != nil {
		return 0, fmt.Errorf("insert vm group: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("vm group last insert id: %w", err)
	}
	return id, nil
}

func (r *vmGroupRepository) Update(ctx context.Context, id int64, configJSON []byte, replicas int) error {
	if _, err := r.exec.ExecContext(ctx, `UPDATE vm_groups SET config_json = ?, replicas = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;`, string(configJSON), replicas, id); err != nil {
		return fmt.Errorf("update vm group: %w", err)
	}
	return nil
}

func (r *vmGroupRepository) UpdateReplicas(ctx context.Context, id int64, replicas int) error {
	if _, err := r.exec.ExecContext(ctx, `UPDATE vm_groups SET replicas = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;`, replicas, id); err != nil {
		return fmt.Errorf("update vm group replicas: %w", err)
	}
	return nil
}

func (r *vmGroupRepository) Delete(ctx context.Context, id int64) error {
	if _, err := r.exec.ExecContext(ctx, `DELETE FROM vm_groups WHERE id = ?;`, id); err != nil {
		return fmt.Errorf("delete vm group: %w", err)
	}
	return nil
}

func (r *vmGroupRepository) GetByName(ctx context.Context, name string) (*db.VMGroup, error) {
	row := r.exec.QueryRowContext(ctx, `SELECT id, name, config_json, replicas, created_at, updated_at FROM vm_groups WHERE name = ?;`, name)
	group, err := scanVMGroup(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

func (r *vmGroupRepository) GetByID(ctx context.Context, id int64) (*db.VMGroup, error) {
	row := r.exec.QueryRowContext(ctx, `SELECT id, name, config_json, replicas, created_at, updated_at FROM vm_groups WHERE id = ?;`, id)
	group, err := scanVMGroup(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

func (r *vmGroupRepository) List(ctx context.Context) ([]db.VMGroup, error) {
	rows, err := r.exec.QueryContext(ctx, `SELECT id, name, config_json, replicas, created_at, updated_at FROM vm_groups ORDER BY name ASC;`)
	if err != nil {
		return nil, fmt.Errorf("list vm groups: %w", err)
	}
	defer rows.Close()

	var result []db.VMGroup
	for rows.Next() {
		group, err := scanVMGroup(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vm groups: %w", err)
	}
	return result, nil
}


func (r *pluginArtifactRepository) Upsert(ctx context.Context, artifact db.PluginArtifact) error {
	if _, err := r.exec.ExecContext(ctx, `INSERT INTO plugin_artifacts (plugin_name, version, artifact_name, kind, source_url, checksum, format, local_path, size_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(plugin_name, version, artifact_name) DO UPDATE SET kind = excluded.kind, source_url = excluded.source_url, checksum = excluded.checksum, format = excluded.format, local_path = excluded.local_path, size_bytes = excluded.size_bytes, updated_at = CURRENT_TIMESTAMP;`,
		artifact.PluginName, artifact.Version, artifact.ArtifactName, artifact.Kind, artifact.SourceURL, artifact.Checksum, artifact.Format, artifact.LocalPath, artifact.SizeBytes); err != nil {
		return fmt.Errorf("upsert plugin artifact: %w", err)
	}
	return nil
}

func (r *pluginArtifactRepository) ListByPlugin(ctx context.Context, plugin string) ([]db.PluginArtifact, error) {
	rows, err := r.exec.QueryContext(ctx, `SELECT id, plugin_name, version, artifact_name, kind, source_url, checksum, format, local_path, size_bytes, created_at, updated_at FROM plugin_artifacts WHERE plugin_name = ? ORDER BY version DESC, artifact_name ASC;`, plugin)
	if err != nil {
		return nil, fmt.Errorf("list plugin artifacts: %w", err)
	}
	defer rows.Close()

	var result []db.PluginArtifact
	for rows.Next() {
		artifact, err := scanPluginArtifact(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugin artifacts: %w", err)
	}
	return result, nil
}


func (r *pluginArtifactRepository) ListByPluginVersion(ctx context.Context, plugin, version string) ([]db.PluginArtifact, error) {
	rows, err := r.exec.QueryContext(ctx, `SELECT id, plugin_name, version, artifact_name, kind, source_url, checksum, format, local_path, size_bytes, created_at, updated_at FROM plugin_artifacts WHERE plugin_name = ? AND version = ? ORDER BY artifact_name ASC;`, plugin, version)
	if err != nil {
		return nil, fmt.Errorf("list plugin artifacts by version: %w", err)
	}
	defer rows.Close()

	var result []db.PluginArtifact
	for rows.Next() {
		artifact, err := scanPluginArtifact(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugin artifacts by version: %w", err)
	}
	return result, nil
}

func (r *pluginArtifactRepository) Get(ctx context.Context, plugin, version, artifactName string) (*db.PluginArtifact, error) {
	row := r.exec.QueryRowContext(ctx, `SELECT id, plugin_name, version, artifact_name, kind, source_url, checksum, format, local_path, size_bytes, created_at, updated_at FROM plugin_artifacts WHERE plugin_name = ? AND version = ? AND artifact_name = ?;`, plugin, version, artifactName)
	artifact, err := scanPluginArtifact(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &artifact, nil
}

func (r *pluginArtifactRepository) DeleteByPluginVersion(ctx context.Context, plugin, version string) error {
	if _, err := r.exec.ExecContext(ctx, `DELETE FROM plugin_artifacts WHERE plugin_name = ? AND version = ?;`, plugin, version); err != nil {
		return fmt.Errorf("delete plugin artifacts by version: %w", err)
	}
	return nil
}

func (r *pluginArtifactRepository) DeleteByPlugin(ctx context.Context, plugin string) error {
	if _, err := r.exec.ExecContext(ctx, `DELETE FROM plugin_artifacts WHERE plugin_name = ?;`, plugin); err != nil {
		return fmt.Errorf("delete plugin artifacts: %w", err)
	}
	return nil
}

func (r *vmCloudInitRepository) Upsert(ctx context.Context, record db.VMCloudInit) error {
	if _, err := r.exec.ExecContext(ctx, `INSERT INTO vm_cloudinit (vm_id, user_data, meta_data, network_config, seed_path)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(vm_id) DO UPDATE SET user_data = excluded.user_data, meta_data = excluded.meta_data, network_config = excluded.network_config, seed_path = excluded.seed_path, updated_at = CURRENT_TIMESTAMP;`,
		record.VMID, record.UserData, record.MetaData, record.NetworkConfig, record.SeedPath); err != nil {
		return fmt.Errorf("upsert vm cloudinit: %w", err)
	}
	return nil
}

func (r *vmCloudInitRepository) Get(ctx context.Context, vmID int64) (*db.VMCloudInit, error) {
	row := r.exec.QueryRowContext(ctx, `SELECT vm_id, user_data, meta_data, network_config, seed_path, updated_at FROM vm_cloudinit WHERE vm_id = ?;`, vmID)
	record, err := scanVMCloudInit(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *vmCloudInitRepository) Delete(ctx context.Context, vmID int64) error {
	if _, err := r.exec.ExecContext(ctx, `DELETE FROM vm_cloudinit WHERE vm_id = ?;`, vmID); err != nil {
		return fmt.Errorf("delete vm cloudinit: %w", err)
	}
	return nil
}

func (r *vmConfigRepository) GetCurrent(ctx context.Context, vmID int64) (*db.VMConfig, error) {
	row := r.exec.QueryRowContext(ctx, `SELECT vm_id, version, config_json, updated_at FROM vm_configs WHERE vm_id = ?;`, vmID)
	cfg, err := scanVMConfig(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

func (r *vmConfigRepository) Upsert(ctx context.Context, vmID int64, payload []byte) (*db.VMConfig, error) {
	var currentVersion int
	row := r.exec.QueryRowContext(ctx, `SELECT version FROM vm_configs WHERE vm_id = ?;`, vmID)
	switch err := row.Scan(&currentVersion); err {
	case nil:
	case sql.ErrNoRows:
		currentVersion = 0
	default:
		return nil, fmt.Errorf("select vm config version: %w", err)
	}

	nextVersion := currentVersion + 1
	configText := string(payload)

	if _, err := r.exec.ExecContext(ctx, `INSERT INTO vm_configs (vm_id, config_json, version, updated_at)
	VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(vm_id) DO UPDATE SET config_json = excluded.config_json, version = excluded.version, updated_at = CURRENT_TIMESTAMP;`, vmID, configText, nextVersion); err != nil {
		return nil, fmt.Errorf("upsert vm config: %w", err)
	}

	if _, err := r.exec.ExecContext(ctx, `INSERT INTO vm_config_history (vm_id, version, config_json, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP);`, vmID, nextVersion, configText); err != nil {
		return nil, fmt.Errorf("insert vm config history: %w", err)
	}

	row = r.exec.QueryRowContext(ctx, `SELECT vm_id, version, config_json, updated_at FROM vm_configs WHERE vm_id = ?;`, vmID)
	cfg, err := scanVMConfig(row)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *vmConfigRepository) History(ctx context.Context, vmID int64, limit int) ([]db.VMConfigHistoryEntry, error) {
	baseQuery := `SELECT id, vm_id, version, config_json, updated_at FROM vm_config_history WHERE vm_id = ? ORDER BY version DESC`
	var (
		rows *sql.Rows
		err  error
	)
	if limit > 0 {
		rows, err = r.exec.QueryContext(ctx, baseQuery+" LIMIT ?", vmID, limit)
	} else {
		rows, err = r.exec.QueryContext(ctx, baseQuery, vmID)
	}
	if err != nil {
		return nil, fmt.Errorf("query vm config history: %w", err)
	}
	defer rows.Close()

	var entries []db.VMConfigHistoryEntry
	for rows.Next() {
		record, scanErr := scanVMConfigHistory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vm config history: %w", err)
	}
	return entries, nil
}

func scanVM(row rowScanner) (db.VM, error) {
	var (
		vm         db.VM
		status     string
		runtime    sql.NullString
		pid        sql.NullInt64
		cmdline    sql.NullString
		serial     sql.NullString
		groupID    sql.NullInt64
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
		&serial,
		&groupID,
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
		val := pid.Int64
		vm.PID = &val
	}
	if cmdline.Valid {
		vm.KernelCmdline = cmdline.String
	}
	if serial.Valid {
		vm.SerialSocket = serial.String
	}
	if groupID.Valid {
		gid := groupID.Int64
		vm.GroupID = &gid
	}

	created, err := parseTimestamp(createdRaw)
	if err != nil {
		return db.VM{}, fmt.Errorf("parse vm created: %w", err)
	}
	updated, err := parseTimestamp(updatedRaw)
	if err != nil {
		return db.VM{}, fmt.Errorf("parse vm updated: %w", err)
	}

	vm.CreatedAt = created
	vm.UpdatedAt = updated
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


func scanPluginArtifact(row rowScanner) (db.PluginArtifact, error) {
	var (
		artifact db.PluginArtifact
		created  any
		updated  any
	)

	if err := row.Scan(&artifact.ID, &artifact.PluginName, &artifact.Version, &artifact.ArtifactName, &artifact.Kind, &artifact.SourceURL, &artifact.Checksum, &artifact.Format, &artifact.LocalPath, &artifact.SizeBytes, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.PluginArtifact{}, err
		}
		return db.PluginArtifact{}, fmt.Errorf("scan plugin artifact: %w", err)
	}
	createdAt, err := parseTimestamp(created)
	if err != nil {
		return db.PluginArtifact{}, fmt.Errorf("parse artifact created_at: %w", err)
	}
	updatedAt, err := parseTimestamp(updated)
	if err != nil {
		return db.PluginArtifact{}, fmt.Errorf("parse artifact updated_at: %w", err)
	}
	artifact.CreatedAt = createdAt
	artifact.UpdatedAt = updatedAt
	return artifact, nil
}

func scanVMCloudInit(row rowScanner) (db.VMCloudInit, error) {
	var (
		record  db.VMCloudInit
		updated any
	)

	if err := row.Scan(&record.VMID, &record.UserData, &record.MetaData, &record.NetworkConfig, &record.SeedPath, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.VMCloudInit{}, err
		}
		return db.VMCloudInit{}, fmt.Errorf("scan vm cloudinit: %w", err)
	}
	if updatedTime, err := parseTimestamp(updated); err == nil {
		record.UpdatedAt = updatedTime
	}
	return record, nil
}

func scanVMGroup(row rowScanner) (db.VMGroup, error) {
	var (
		group      db.VMGroup
		configText string
		createdRaw any
		updatedRaw any
	)

	if err := row.Scan(&group.ID, &group.Name, &configText, &group.Replicas, &createdRaw, &updatedRaw); err != nil {
		return db.VMGroup{}, err
	}
	group.ConfigJSON = []byte(configText)
	created, err := parseTimestamp(createdRaw)
	if err != nil {
		return db.VMGroup{}, fmt.Errorf("parse vm group created: %w", err)
	}
	updated, err := parseTimestamp(updatedRaw)
	if err != nil {
		return db.VMGroup{}, fmt.Errorf("parse vm group updated: %w", err)
	}
	group.CreatedAt = created
	group.UpdatedAt = updated
	return group, nil
}

func scanVMConfig(row rowScanner) (db.VMConfig, error) {
	var (
		cfg     db.VMConfig
		payload []byte
		updated any
	)

	if err := row.Scan(&cfg.VMID, &cfg.Version, &payload, &updated); err != nil {
		if err == sql.ErrNoRows {
			return db.VMConfig{}, err
		}
		return db.VMConfig{}, fmt.Errorf("scan vm config: %w", err)
	}

	cfg.ConfigJSON = append([]byte(nil), payload...)
	ts, err := parseTimestamp(updated)
	if err != nil {
		return db.VMConfig{}, fmt.Errorf("parse vm config updated: %w", err)
	}
	cfg.UpdatedAt = ts
	return cfg, nil
}

func scanVMConfigHistory(row rowScanner) (db.VMConfigHistoryEntry, error) {
	var (
		record  db.VMConfigHistoryEntry
		payload []byte
		updated any
	)

	if err := row.Scan(&record.ID, &record.VMID, &record.Version, &payload, &updated); err != nil {
		return db.VMConfigHistoryEntry{}, fmt.Errorf("scan vm config history: %w", err)
	}
	record.ConfigJSON = append([]byte(nil), payload...)
	ts, err := parseTimestamp(updated)
	if err != nil {
		return db.VMConfigHistoryEntry{}, fmt.Errorf("parse vm config history updated: %w", err)
	}
	record.UpdatedAt = ts
	return record, nil
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

func parseTimestamp(value any) (time.Time, error) {
	switch t := value.(type) {
	case time.Time:
		return t, nil
	case string:
		for _, layout := range timestampLayouts {
			parsed, err := time.Parse(layout, t)
			if err == nil {
				return parsed, nil
			}
		}
	case []byte:
		for _, layout := range timestampLayouts {
			parsed, err := time.Parse(layout, string(t))
			if err == nil {
				return parsed, nil
			}
		}
	case int64:
		return time.Unix(t, 0), nil
	case float64:
		return time.Unix(int64(t), 0), nil
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp type %T", value)
}
