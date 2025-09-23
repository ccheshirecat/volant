package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/viperhq/viper/internal/cli/client"
)

const (
	refreshInterval = 5 * time.Second
)

var (
	quitKeys = key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
	)
	tabKey = key.NewBinding(
		key.WithKeys("tab"),
	)
	enterKey = key.NewBinding(
		key.WithKeys("enter"),
	)
	upKey = key.NewBinding(
		key.WithKeys("up"),
	)
	downKey = key.NewBinding(
		key.WithKeys("down"),
	)
)

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
type systemStatusMsg struct {
	status *client.SystemStatus
}

type model struct {
	ctx        context.Context
	cancel     context.CancelFunc
	api        *client.Client
	vms        []client.VM
	err        error
	eventCh    chan client.VMEvent
	streamErr  error
	streamEOF  bool
	logs       []string
	vmList     list.Model
	logView    viewport.Model
	input      textinput.Model
	header     string
	focused    paneFocus
	width      int
	height     int
}

type paneFocus int

const (
	focusVMList paneFocus = iota
	focusLogs
	focusInput
)

func newModel(ctx context.Context, cancel context.CancelFunc, api *client.Client) model {
	m := model{
		ctx:       ctx,
		cancel:    cancel,
		api:       api,
		eventCh:   make(chan client.VMEvent, 16),
		logs:      []string{},
		focused:   focusVMList,
		vmList:    list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
		logView:   viewport.New(0, 0),
		input:     textinput.New(),
	}

	m.vmList.Title = "VMs"
	m.vmList.SetShowHelp(false)
	m.vmList.SetShowStatusBar(false)
	m.vmList.SetShowPagination(false)
	m.vmList.SetShowFilter(false)

	m.input.Placeholder = "Enter command (e.g., vms create my-vm)..."
	m.input.CharLimit = 156
	m.input.Width = 50
	m.input.Prompt = "» "

	m.logView.Width = 50
	m.logView.Height = 10
	m.logView.SetContent("Waiting for events...")
	m.logView.MouseWheelEnabled = true
	m.logView.YPosition = 1
	m.logView.YOffset = 0

	m.header = "VIPER v2.0 | Initializing..."

	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchVMsCmd(m.api, m.ctx),
		fetchStatusCmd(m.api, m.ctx),
		watchEventsCmd(m.api, m.ctx, m.eventCh),
		waitEventCmd(m.eventCh),
		tickCmd(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vmList.SetWidth(msg.Width / 2)
		m.vmList.SetHeight(msg.Height / 2)
		m.logView.Width = msg.Width / 2
		m.logView.Height = msg.Height / 2
		m.input.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		if key.Matches(msg, quitKeys) {
			m.cancel()
			return m, tea.Quit
		}

		switch m.focused {
		case focusVMList:
			if key.Matches(msg, tabKey) {
				m.focused = focusLogs
				return m, nil
			}
			if key.Matches(msg, enterKey) {
				// Select VM for logs or something; for now, switch to input
				m.focused = focusInput
				return m, nil
			}
			var cmd tea.Cmd
			m.vmList, cmd = m.vmList.Update(msg)
			cmds = append(cmds, cmd)
		case focusLogs:
			if key.Matches(msg, tabKey) {
				m.focused = focusInput
				return m, nil
			}
			if key.Matches(msg, upKey) || key.Matches(msg, downKey) {
				m.logView.SetYOffset(m.logView.YOffset + 1)
				return m, nil
			}
			var cmd tea.Cmd
			m.logView, cmd = m.logView.Update(msg)
			cmds = append(cmds, cmd)
		case focusInput:
			if key.Matches(msg, tabKey) {
				m.focused = focusVMList
				return m, nil
			}
			if key.Matches(msg, enterKey) {
				cmd := m.executeCommand(m.input.Value())
				m.input.Reset()
				m.focused = focusVMList
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}

	case vmListMsg:
		m.vms = msg.vms
		m.err = nil
		items := make([]list.Item, len(m.vms))
		for i, vm := range m.vms {
			items[i] = vmItem{vm: vm}
		}
		m.vmList.SetItems(items)
		return m, nil

	case systemStatusMsg:
		vmCount := msg.status.VMCount
		cpu := fmt.Sprintf("%.1f%%", msg.status.CPU)
		mem := fmt.Sprintf("%.1f%%", msg.status.MEM)
		m.header = fmt.Sprintf("VIPER v2.0 | VMs: %d | CPU: %s | MEM: %s", vmCount, cpu, mem)
		return m, nil

	case vmEventMsg:
		ts := msg.event.Timestamp.Format(time.RFC3339)
		line := fmt.Sprintf("%s %-12s %s", ts, msg.event.Type, msg.event.Message)
		m.logs = append([]string{line}, m.logs...)
		if len(m.logs) > 100 {
			m.logs = m.logs[:100]
		}
		content := strings.Join(m.logs, "\n")
		m.logView.SetContent(content)
		// Refresh VM list on event
		cmds = append(cmds, fetchVMsCmd(m.api, m.ctx))
		return m, tea.Batch(cmds...)

	case errMsg:
		m.err = msg.err
		return m, nil

	case eventsClosedMsg:
		m.streamEOF = true
		return m, nil

	case tickMsg:
		cmds = append(cmds, tickCmd(), fetchVMsCmd(m.api, m.ctx), fetchStatusCmd(m.api, m.ctx))
		return m, tea.Batch(cmds...)
	}

	// Update sub-models
	var cmd tea.Cmd
	m.vmList, cmd = m.vmList.Update(msg)
	cmds = append(cmds, cmd)

	m.logView, cmd = m.logView.Update(msg)
	cmds = append(cmds, cmd)

	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Define styles locally
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
	headerTitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Bold(true)
	vmListStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)
	logViewportStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)
	inputStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)

	header := headerStyle.Render(headerTitleStyle.Render(m.header))
	help := helpStyle.Render("(q to quit) (tab to switch panes)")

	vmListView := vmListStyle.Width(m.width / 2).Height(m.height / 2).Render(m.vmList.View())
	logView := logViewportStyle.Width(m.width / 2).Height(m.height / 2).Render(m.logView.View())
	inputView := inputStyle.Width(m.width).Render(m.input.View())

	content := lipgloss.JoinVertical(lipgloss.Left, header, lipgloss.JoinHorizontal(lipgloss.Left, vmListView, logView), inputView, help)

	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
		content += "\n" + errStyle
	}
	if m.streamEOF {
		content += "\nEvent stream closed."
	}

	return content
}

func (m model) executeCommand(input string) tea.Cmd {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}
	cmd := parts[0]
	switch cmd {
	case "vms", "vm":
		if len(parts) >= 3 && parts[1] == "create" {
			name := parts[2]
			payload := client.CreateVMRequest{
				Name:     name,
				CPUCores: 2,
				MemoryMB: 2048,
			}
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
				defer cancel()
				vm, err := m.api.CreateVM(ctx, payload)
				if err != nil {
					return errMsg{err: err}
				}
				return vmListMsg{vms: []client.VM{*vm}}
			}
		}
	}
	return nil
}

type vmItem struct {
	vm client.VM
}

func (i vmItem) FilterValue() string { return i.vm.Name }

func (i vmItem) Title() string {
	return fmt.Sprintf("%s (%s)", i.vm.Name, i.vm.Status)
}

func (i vmItem) Description() string {
	return fmt.Sprintf("IP: %s | CPU: %d | MEM: %dMB", i.vm.IPAddress, i.vm.CPUCores, i.vm.MemoryMB)
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

func fetchStatusCmd(api *client.Client, parent context.Context) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		status, err := api.GetSystemStatus(ctx)
		if err != nil {
			return errMsg{err: err}
		}
		return systemStatusMsg{status: status}
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
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		cancel()
		return err
	}
	return nil
}