package app

import (
	"context"
	"fmt"
)

// Run starts the agent control loop. The chromedp integration will land in
// future commits; for now it returns a not implemented error so callers fail
// fast.
func Run(ctx context.Context) error {
	return fmt.Errorf("viper-agent not yet implemented")
}
