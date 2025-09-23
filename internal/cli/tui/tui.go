package tui

import (
    "context"
    "fmt"
    "os"
    "time"

    tea "github.com/charmbracelet/bubbletea"

    "github.com/viperhq/viper/internal/cli/client"
)

const refreshInterval = 5 * time.Second

type vmListMsg struct {
    vms []client.VM
}

type vmEventMsg struct {
    event client.VMEvent
}

type errMsg struct {
    err error
}

type eventsClosedMsg struct{}

type tickMsg struct{}

// Run launches the Bubble Tea TUI.
func Run() error {
    base := os.Getenv("VIPER_API_BASE")
    api, err := client.New(base)
    if err != nil {
        return err
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    m := newModel(ctx, cancel, api)
    p := tea.NewProgram(m, tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        cancel()
        return err
    }
    return nil
}

type model struct {
    ctx       context.Context
    cancel    context.CancelFunc
    api       *client.Client
    vms       []client.VM
    logs      []string
    err       error
    eventCh   chan client.VMEvent
    streamErr error
    streamEOF bool
}

func newModel(ctx context.Context, cancel context.CancelFunc, api *client.Client) model {
    return model{
        ctx:     ctx,
        cancel:  cancel,
        api:     api,
        eventCh: make(chan client.VMEvent, 16),
    }
}

func (m model) Init() tea.Cmd {
    return tea.Batch(
        fetchVMsCmd(m.api, m.ctx),
        watchEventsCmd(m.api, m.ctx, m.eventCh),
        waitEventCmd(m.eventCh),
        tickCmd(),
    )
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c":
            m.cancel()
            return m, tea.Quit
        }
    case vmListMsg:
        m.vms = msg.vms
        m.err = nil
        return m, nil
    case vmEventMsg:
        ts := msg.event.Timestamp.Format(time.RFC3339)
        line := fmt.Sprintf("%s %-12s %s", ts, msg.event.Type, msg.event.Message)
        m.logs = append([]string{line}, m.logs...)
        if len(m.logs) > 100 {
            m.logs = m.logs[:100]
        }
        // refresh list after event
        return m, tea.Batch(fetchVMsCmd(m.api, m.ctx), waitEventCmd(m.eventCh))
    case errMsg:
        m.err = msg.err
        return m, nil
    case eventsClosedMsg:
        m.streamEOF = true
        return m, nil
    case tickMsg:
        return m, tea.Batch(tickCmd(), fetchVMsCmd(m.api, m.ctx))
    }
    return m, nil
}

func (m model) View() string {
    header := "VIPER :: MicroVM Dashboard (q to quit)\n"
    var body string
    body += "\nVMs:\n"
    if len(m.vms) == 0 {
        body += "  (no VMs)\n"
    } else {
        body += fmt.Sprintf("  %-18s %-10s %-16s %-20s %-4s %-4s\n", "NAME", "STATUS", "IP", "MAC", "CPU", "MEM")
        for _, vm := range m.vms {
            body += fmt.Sprintf("  %-18s %-10s %-16s %-20s %-4d %-4d\n", vm.Name, vm.Status, vm.IPAddress, vm.MACAddress, vm.CPUCores, vm.MemoryMB)
        }
    }

    body += "\nEvents:\n"
    if len(m.logs) == 0 {
        body += "  (waiting for events)\n"
    } else {
        for i, line := range m.logs {
            if i >= 10 {
                break
            }
            body += "  " + line + "\n"
        }
    }

    if m.err != nil {
        body += fmt.Sprintf("\nError: %v\n", m.err)
    }
    if m.streamEOF {
        body += "\nEvent stream closed.\n"
    }

    return header + body
}

func fetchVMsCmd(api *client.Client, parent context.Context) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(parent, 5*time.Second)
        defer cancel()
        vms, err := api.ListVMs(ctx)
        if err != nil {
            return errMsg{err: err}
        }
        return vmListMsg{vms: vms}
    }
}

func watchEventsCmd(api *client.Client, ctx context.Context, ch chan<- client.VMEvent) tea.Cmd {
    return func() tea.Msg {
        go func() {
            err := api.WatchVMEvents(ctx, func(ev client.VMEvent) {
                select {
                case ch <- ev:
                case <-ctx.Done():
                }
            })
            if err != nil && ctx.Err() == nil {
                select {
                case ch <- client.VMEvent{Type: "ERROR", Message: err.Error(), Timestamp: time.Now().UTC()}:
                default:
                }
            }
            close(ch)
        }()
        return nil
    }
}

func waitEventCmd(ch <-chan client.VMEvent) tea.Cmd {
    return func() tea.Msg {
        ev, ok := <-ch
        if !ok {
            return eventsClosedMsg{}
        }
        return vmEventMsg{event: ev}
    }
}

func tickCmd() tea.Cmd {
    return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}
