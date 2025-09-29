package stub

import (
	"context"
	"os"

	"github.com/ccheshirecat/volant/internal/server/db"
	"github.com/ccheshirecat/volant/internal/server/orchestrator"
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
func (Engine) Store() db.Store { return nil }

func NewStub(params orchestrator.Params) (orchestrator.Engine, error) {
	params.APIListenAddr = "127.0.0.1:7777"
	params.APIAdvertiseAddr = "127.0.0.1:7777"
	params.RuntimeDir = os.TempDir()
	return orchestrator.New(params)
}
