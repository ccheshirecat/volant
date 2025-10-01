package stub

import (
	"context"
	"os"

	"github.com/ccheshirecat/volant/internal/server/db"
	"github.com/ccheshirecat/volant/internal/server/orchestrator"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/vmconfig"
)

type Engine struct{}

func (Engine) Start(ctx context.Context) error { return nil }
func (Engine) Stop(ctx context.Context) error  { return nil }
func (Engine) CreateVM(ctx context.Context, req orchestrator.CreateVMRequest) (*db.VM, error) {
	return nil, nil
}
func (Engine) DestroyVM(ctx context.Context, name string) error { return nil }
func (Engine) ListVMs(ctx context.Context) ([]db.VM, error)     { return nil, nil }
func (Engine) GetVM(ctx context.Context, name string) (*db.VM, error) {
	return nil, nil
}
func (Engine) GetVMConfig(ctx context.Context, name string) (*vmconfig.Versioned, error) {
	return nil, nil
}
func (Engine) UpdateVMConfig(ctx context.Context, name string, patch vmconfig.Patch) (*vmconfig.Versioned, error) {
	return nil, nil
}
func (Engine) GetVMConfigHistory(ctx context.Context, name string, limit int) ([]vmconfig.HistoryEntry, error) {
	return nil, nil
}
func (Engine) StartVM(ctx context.Context, name string) (*db.VM, error) {
	return nil, nil
}
func (Engine) StopVM(ctx context.Context, name string) (*db.VM, error) {
	return nil, nil
}
func (Engine) RestartVM(ctx context.Context, name string) (*db.VM, error) {
	return nil, nil
}
func (Engine) Store() db.Store { return nil }

func NewStub(params orchestrator.Params) (orchestrator.Engine, error) {
	params.APIListenAddr = "127.0.0.1:7777"
	params.APIAdvertiseAddr = "127.0.0.1:7777"
	params.RuntimeDir = os.TempDir()
	return orchestrator.New(params)
}
