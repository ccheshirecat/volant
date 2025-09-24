package stub

import (
	"context"
	"fmt"

	"github.com/ccheshirecat/viper/internal/server/db"
	"github.com/ccheshirecat/viper/internal/server/orchestrator"
)

// Engine is a placeholder implementation used until the real orchestrator lands.
type Engine struct{}

var _ orchestrator.Engine = (*Engine)(nil)

func New() *Engine { return &Engine{} }

func (e *Engine) Start(context.Context) error {
	return fmt.Errorf("orchestrator engine not implemented")
}

func (e *Engine) Stop(context.Context) error { return nil }

func (e *Engine) CreateVM(context.Context, orchestrator.CreateVMRequest) (*db.VM, error) {
	return nil, fmt.Errorf("orchestrator engine not implemented")
}

func (e *Engine) DestroyVM(context.Context, string) error {
	return fmt.Errorf("orchestrator engine not implemented")
}

func (e *Engine) ListVMs(context.Context) ([]db.VM, error) {
	return nil, fmt.Errorf("orchestrator engine not implemented")
}

func (e *Engine) GetVM(context.Context, string) (*db.VM, error) {
	return nil, fmt.Errorf("orchestrator engine not implemented")
}
