package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ccheshirecat/viper/internal/cli/client"
)

const (
	refreshInterval       = 5 * time.Second
	logBufferCapacity     = 200
	statusClearAfter      = 4 * time.Second
	defaultCreateCPUCores = 2
	defaultCreateMemoryMB = 2048
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

	rootCommands   = []string{"vms", "status", "help"}
	vmsSubcommands = []string{"create", "delete", "list", "get"}
)

func newSpinnerModel() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return sp
}

type vmListMsg struct {
	vms []client.VM
}

type vmEventMsg struct {
	event client.VMEvent
}

type vmLogMsg struct {
	source <-chan client.VMLogEvent
	event  client.VMLogEvent
}

type errMsg struct {
	err error
}

type eventsClosedMsg struct{}
type logClosedMsg struct {
	source <-chan client.VMLogEvent
}
type tickMsg struct{}
type systemStatusMsg struct {
	status *client.SystemStatus
}
type commandResultMsg struct {
	label    string
	lines    []string
	err      error
	duration time.Duration
}
type clearStatusMsg struct{}

type statusLevel int

const (
	statusLevelInfo statusLevel = iota
	statusLevelRunning
	statusLevelSuccess
	statusLevelError
)

type paneFocus int

const (
	focusVMList paneFocus = iota
	focusLogs
	focusInput
)

type commandAction func(context.Context, *client.Client) ([]string, error)

type commandPlan struct {
	label  string
	action commandAction
}

type model struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *client.Client

	// VM/state data
	vms []client.VM
	err error

	eventCh   chan client.VMEvent
	streamEOF bool

	logCh        chan client.VMLogEvent
	logCancel    context.CancelFunc
	logStreamEOF bool

	logs       []string
	selectedVM string

	// UI components
	vmList  list.Model
	logView viewport.Model
	input   textinput.Model
	spinner spinner.Model

	focused paneFocus
	width   int
	height  int

	// Command execution
	executing      bool
	commandHistory []string
	historyIndex   int
	statusMessage  string
	statusLevel    statusLevel
	statusStarted  time.Time
	pendingClear   bool
}

func (m *model) setStatus(level statusLevel, message string) tea.Cmd {
	if message == "" {
		return nil
	}
	if m.executing {
		return nil
	}
	m.statusMessage = message
	m.statusLevel = level
	return m.ensureStatusClear()
}

func (m *model) ensureStatusClear() tea.Cmd {
	if m.pendingClear {
		return nil
	}
	m.pendingClear = true
	return clearStatusLater()
}

func (m *model) clearStatus() {
	m.statusMessage = ""
	m.pendingClear = false
}

func newModel(ctx context.Context, cancel context.CancelFunc, api *client.Client) model {
	m := model{
		ctx:        ctx,
		cancel:     cancel,
		api:        api,
		eventCh:    make(chan client.VMEvent, 64),
		vmList:     list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
		logView:    viewport.New(0, 0),
		input:      textinput.New(),
		spinner:    newSpinnerModel(),
		focused:    focusVMList,
		logs:       []string{},
		selectedVM: "",
	}

	m.vmList.Title = "VMs"
	m.vmList.SetShowHelp(false)
	m.vmList.SetShowStatusBar(false)
	m.vmList.SetShowPagination(false)
	m.vmList.SetShowFilter(false)

	m.input.Placeholder = "Enter command (e.g., vms create my-vm)..."
	m.input.CharLimit = 512
	m.input.Width = 50
	m.input.Prompt = "» "

	m.logView.Width = 50
	m.logView.Height = 12
	m.logView.YPosition = 1
	m.logView.MouseWheelEnabled = true
	m.logView.SetContent("Select a VM to begin streaming logs.")

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
	var vmListUpdated, logViewUpdated, inputUpdated bool

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		listWidth := msg.Width / 2
		if listWidth < 40 {
			listWidth = msg.Width - 4
		}
		listHeight := msg.Height / 2
		if listHeight < 10 {
			listHeight = msg.Height - 6
		}

		m.vmList.SetWidth(listWidth)
		m.vmList.SetHeight(listHeight)
		m.logView.Width = listWidth
		m.logView.Height = listHeight
		m.input.Width = msg.Width

	case tea.KeyMsg:
		switch m.focused {
		case focusVMList:
			prevSelection := selectedVMName(m.vmList)

			if key.Matches(msg, tabKey) {
				m.focused = focusLogs
				break
			}

			var cmd tea.Cmd
			m.vmList, cmd = m.vmList.Update(msg)
			vmListUpdated = true
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

			if key.Matches(msg, enterKey) {
				m.focused = focusInput
			}

			currentSelection := selectedVMName(m.vmList)
			if currentSelection != "" && currentSelection != prevSelection {
				if logCmd := m.ensureLogStreamForSelection(); logCmd != nil {
					cmds = append(cmds, logCmd)
				}
			}

		case focusLogs:
			if key.Matches(msg, tabKey) {
				m.focused = focusInput
				break
			}

			switch {
			case key.Matches(msg, upKey):
				m.logView.LineUp(1)
			case key.Matches(msg, downKey):
				m.logView.LineDown(1)
			default:
				var cmd tea.Cmd
				m.logView, cmd = m.logView.Update(msg)
				logViewUpdated = true
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}

		case focusInput:
			switch {
			case key.Matches(msg, tabKey):
				if !m.applyAutocomplete() {
					if cmd := m.setStatus(statusLevelInfo, "No autocomplete suggestions"); cmd != nil {
						cmds = append(cmds, cmd)
					}
					m.focused = focusVMList
				}
			case msg.String() == "ctrl+w":
				m.input.SetValue("")
			case msg.String() == "up":
				if m.navigateHistory(-1) {
					inputUpdated = true
				} else if cmd := m.setStatus(statusLevelInfo, "Start of history"); cmd != nil {
					cmds = append(cmds, cmd)
				}
			case msg.String() == "down":
				if m.navigateHistory(1) {
					inputUpdated = true
				} else if cmd := m.setStatus(statusLevelInfo, "End of history"); cmd != nil {
					cmds = append(cmds, cmd)
				}
			case key.Matches(msg, enterKey):
				if execCmd := m.queueCommand(m.input.Value()); execCmd != nil {
					cmds = append(cmds, execCmd)
				}
				m.input.Reset()
				m.historyIndex = len(m.commandHistory)
				m.focused = focusVMList
				inputUpdated = true
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				inputUpdated = true
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

	case vmListMsg:
		m.vms = msg.vms
		m.err = nil

		items := make([]list.Item, len(m.vms))
		for i, vm := range m.vms {
			items[i] = vmItem{vm: vm}
		}
		m.vmList.SetItems(items)

		if len(m.vms) == 0 {
			m.selectedVM = ""
			m.resetLogStream()
			m.logView.SetContent("No VMs available.")
		} else {
			if m.selectedVM != "" {
				for idx, vm := range m.vms {
					if vm.Name == m.selectedVM {
						m.vmList.Select(idx)
						break
					}
				}
			}
			if m.selectedVM == "" {
				m.vmList.Select(0)
			}
			if logCmd := m.ensureLogStreamForSelection(); logCmd != nil {
				cmds = append(cmds, logCmd)
			}
		}

	case systemStatusMsg:
		vmCount := msg.status.VMCount
		cpu := fmt.Sprintf("%.1f%%", msg.status.CPU)
		mem := fmt.Sprintf("%.1f%%", msg.status.MEM)
		m.statusMessage = fmt.Sprintf("Cluster status refreshed | VMs: %d | CPU: %s | MEM: %s", vmCount, cpu, mem)
		m.statusLevel = statusLevelInfo

	case vmEventMsg:
		if msg.event.Type == client.VMEventTypeLog {
			if msg.event.Name == m.selectedVM {
				m.appendLogLine(msg.event.Timestamp, msg.event.Stream, valueOrFallback(msg.event.Line, msg.event.Message))
			}
		} else {
			m.appendLifecycleEvent(msg.event)
			cmds = append(cmds, fetchVMsCmd(m.api, m.ctx))
		}
		cmds = append(cmds, waitEventCmd(m.eventCh))

	case vmLogMsg:
		if msg.source == m.logCh {
			if msg.event.Name == m.selectedVM || m.selectedVM == "" {
				m.appendLogLine(msg.event.Timestamp, msg.event.Stream, msg.event.Line)
			}
			if m.logCh != nil {
				cmds = append(cmds, waitLogCmd(m.logCh))
			}
		}

	case logClosedMsg:
		if msg.source == m.logCh {
			m.logStreamEOF = true
			m.logCh = nil
			if m.logCancel != nil {
				m.logCancel()
				m.logCancel = nil
			}
		}

	case commandResultMsg:
		m.executing = false
		m.pendingClear = false
		m.spinner = newSpinnerModel()

		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("%s failed: %v", msg.label, msg.err)
			m.statusLevel = statusLevelError
		} else {
			m.statusMessage = fmt.Sprintf("%s succeeded in %s", msg.label, msg.duration.Round(10*time.Millisecond))
			m.statusLevel = statusLevelSuccess
		}

		for _, line := range msg.lines {
			m.appendLogLine(time.Now().UTC(), "cli", line)
		}

		if msg.err == nil {
			if cmd := m.ensureStatusClear(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case clearStatusMsg:
		if !m.executing {
			m.clearStatus()
		}

	case errMsg:
		m.err = msg.err

	case eventsClosedMsg:
		m.streamEOF = true

	case tickMsg:
		cmds = append(cmds,
			tickCmd(),
			fetchVMsCmd(m.api, m.ctx),
			fetchStatusCmd(m.api, m.ctx),
		)
	}

	var spinCmd tea.Cmd
	if m.executing {
		m.spinner, spinCmd = m.spinner.Update(msg)
		if spinCmd != nil {
			cmds = append(cmds, spinCmd)
		}
	}

	if !vmListUpdated {
		var cmd tea.Cmd
		m.vmList, cmd = m.vmList.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if !logViewUpdated {
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if !inputUpdated {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
	headerTitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Bold(true)
	statusStyle := lipgloss.NewStyle().Padding(0, 1)
	vmListStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)
	logViewportStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)
	inputStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)

	header := headerStyle.Render(headerTitleStyle.Render("VIPER v2.0 | God Mode Dashboard"))

	statusLine := ""
	if m.statusMessage != "" {
		style := statusColorStyle(m.statusLevel)
		if m.executing {
			statusLine = fmt.Sprintf("%s %s", m.spinner.View(), style.Render(m.statusMessage))
		} else {
			statusLine = style.Render(m.statusMessage)
		}
	} else if m.executing {
		statusLine = fmt.Sprintf("%s %s", m.spinner.View(), statusColorStyle(statusLevelRunning).Render("Executing command..."))
	}
	statusView := statusStyle.Render(statusLine)

	vmListView := vmListStyle.Width(m.width / 2).Height(m.height / 2).Render(m.vmList.View())

	logTitle := "Logs"
	if m.selectedVM != "" {
		logTitle = fmt.Sprintf("Logs (%s)", m.selectedVM)
	}
	logContent := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Render(logTitle),
		m.logView.View(),
	)
	logView := logViewportStyle.Width(m.width / 2).Height(m.height / 2).Render(logContent)

	inputView := inputStyle.Width(m.width).Render(m.input.View())

	help := helpStyle.Render("(tab: switch/autocomplete) (ctrl+w: clear input) (↑/↓: history) (q: quit)")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		statusView,
		lipgloss.JoinHorizontal(lipgloss.Left, vmListView, logView),
		inputView,
		help,
	)

	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
		content += "\n" + errStyle
	}
	if m.streamEOF {
		content += "\nEvent stream closed."
	}
	if m.logStreamEOF {
		content += "\nLog stream closed."
	}

	return content
}

func (m *model) queueCommand(input string) tea.Cmd {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return nil
	}
	if m.executing {
		m.statusMessage = "Another command is already running."
		m.statusLevel = statusLevelError
		return nil
	}

	m.appendToHistory(raw)

	parts := tokenize(raw)
	plan, err := m.planCommand(parts)
	if err != nil {
		m.statusMessage = err.Error()
		m.statusLevel = statusLevelError
		return nil
	}

	m.executing = true
	m.statusMessage = fmt.Sprintf("Executing %s ...", plan.label)
	m.statusLevel = statusLevelRunning
	m.statusStarted = time.Now()
	m.pendingClear = false
	m.spinner = newSpinnerModel()

	return tea.Batch(
		executeCommand(m.ctx, m.api, plan),
		m.spinner.Tick,
	)
}

func (m *model) planCommand(parts []string) (commandPlan, error) {
	if len(parts) == 0 {
		return commandPlan{}, fmt.Errorf("empty command")
	}

	switch parts[0] {
	case "help":
		return commandPlan{
			label: "help",
			action: func(ctx context.Context, api *client.Client) ([]string, error) {
				lines := []string{
					"Available commands:",
					"  help                          - Show this help message.",
					"  status                        - Refresh system status.",
					"  vms list                      - List all VMs.",
					"  vms get <name>                - Show VM details.",
					"  vms create <name> [flags...]  - Create a VM (flags: --cpu, --memory, --kernel-cmdline).",
					"  vms delete <name>             - Destroy a VM.",
				}
				return lines, nil
			},
		}, nil

	case "status":
		return commandPlan{
			label: "status",
			action: func(ctx context.Context, api *client.Client) ([]string, error) {
				status, err := api.GetSystemStatus(ctx)
				if err != nil {
					return nil, err
				}
				line := fmt.Sprintf("System status | VMs: %d | CPU: %.2f%% | MEM: %.2f%%", status.VMCount, status.CPU, status.MEM)
				return []string{line}, nil
			},
		}, nil

	case "vms", "vm":
		if len(parts) == 1 {
			return commandPlan{}, fmt.Errorf("vms command requires a subcommand (create, delete, list, get)")
		}
		sub := parts[1]

		switch sub {
		case "list":
			return commandPlan{
				label: "vms list",
				action: func(ctx context.Context, api *client.Client) ([]string, error) {
					vms, err := api.ListVMs(ctx)
					if err != nil {
						return nil, err
					}
					lines := []string{fmt.Sprintf("Found %d VM(s)", len(vms))}
					for _, vm := range vms {
						lines = append(lines, fmt.Sprintf("- %s [%s] %s", vm.Name, vm.Status, vm.IPAddress))
					}
					return lines, nil
				},
			}, nil

		case "get":
			if len(parts) < 3 {
				return commandPlan{}, fmt.Errorf("vms get requires a VM name")
			}
			name := parts[2]
			return commandPlan{
				label: fmt.Sprintf("vms get %s", name),
				action: func(ctx context.Context, api *client.Client) ([]string, error) {
					vm, err := api.GetVM(ctx, name)
					if err != nil {
						return nil, err
					}
					lines := []string{
						fmt.Sprintf("VM %s:", vm.Name),
						fmt.Sprintf("  Status: %s", vm.Status),
						fmt.Sprintf("  IP: %s", vm.IPAddress),
						fmt.Sprintf("  MAC: %s", vm.MACAddress),
						fmt.Sprintf("  CPU: %d", vm.CPUCores),
						fmt.Sprintf("  Memory: %d MB", vm.MemoryMB),
					}
					if vm.PID != nil {
						lines = append(lines, fmt.Sprintf("  PID: %d", *vm.PID))
					}
					if vm.KernelCmdline != "" {
						lines = append(lines, fmt.Sprintf("  Kernel Cmdline: %s", vm.KernelCmdline))
					}
					return lines, nil
				},
			}, nil

		case "create":
			if len(parts) < 3 {
				return commandPlan{}, fmt.Errorf("vms create requires a VM name")
			}
			name := parts[2]
			cpu := defaultCreateCPUCores
			mem := defaultCreateMemoryMB
			kernelCmdline := ""

			flags := parts[3:]
			for i := 0; i < len(flags); i++ {
				token := flags[i]
				switch {
				case strings.HasPrefix(token, "--cpu="):
					val := strings.TrimPrefix(token, "--cpu=")
					parsed, err := strconv.Atoi(val)
					if err != nil || parsed <= 0 {
						return commandPlan{}, fmt.Errorf("invalid --cpu value %q", val)
					}
					cpu = parsed
				case token == "--cpu" && i+1 < len(flags):
					i++
					val := flags[i]
					parsed, err := strconv.Atoi(val)
					if err != nil || parsed <= 0 {
						return commandPlan{}, fmt.Errorf("invalid --cpu value %q", val)
					}
					cpu = parsed
				case strings.HasPrefix(token, "--memory="):
					val := strings.TrimPrefix(token, "--memory=")
					parsed, err := strconv.Atoi(val)
					if err != nil || parsed <= 0 {
						return commandPlan{}, fmt.Errorf("invalid --memory value %q", val)
					}
					mem = parsed
				case token == "--memory" && i+1 < len(flags):
					i++
					val := flags[i]
					parsed, err := strconv.Atoi(val)
					if err != nil || parsed <= 0 {
						return commandPlan{}, fmt.Errorf("invalid --memory value %q", val)
					}
					mem = parsed
				case strings.HasPrefix(token, "--kernel-cmdline="):
					kernelCmdline = strings.TrimPrefix(token, "--kernel-cmdline=")
				case token == "--kernel-cmdline" && i+1 < len(flags):
					i++
					kernelCmdline = flags[i]
				default:
					return commandPlan{}, fmt.Errorf("unknown flag %q", token)
				}
			}

			return commandPlan{
				label: fmt.Sprintf("vms create %s", name),
				action: func(ctx context.Context, api *client.Client) ([]string, error) {
					req := client.CreateVMRequest{
						Name:          name,
						CPUCores:      cpu,
						MemoryMB:      mem,
						KernelCmdline: kernelCmdline,
					}
					if _, err := api.CreateVM(ctx, req); err != nil {
						return nil, err
					}
					return []string{
						fmt.Sprintf("VM %s creation requested (cpu=%d, memory=%dMB)", name, cpu, mem),
					}, nil
				},
			}, nil

		case "delete", "destroy":
			if len(parts) < 3 {
				return commandPlan{}, fmt.Errorf("vms %s requires a VM name", sub)
			}
			name := parts[2]
			return commandPlan{
				label: fmt.Sprintf("vms delete %s", name),
				action: func(ctx context.Context, api *client.Client) ([]string, error) {
					if err := api.DeleteVM(ctx, name); err != nil {
						return nil, err
					}
					return []string{fmt.Sprintf("VM %s deletion requested", name)}, nil
				},
			}, nil
		default:
			return commandPlan{}, fmt.Errorf("unknown vms subcommand %q", sub)
		}
	default:
		return commandPlan{}, fmt.Errorf("unknown command %q", parts[0])
	}
}

func (m *model) appendToHistory(cmd string) {
	if len(m.commandHistory) == 0 || m.commandHistory[len(m.commandHistory)-1] != cmd {
		m.commandHistory = append(m.commandHistory, cmd)
	}
	m.historyIndex = len(m.commandHistory)
	m.pendingClear = false
}

func (m *model) navigateHistory(delta int) bool {
	if len(m.commandHistory) == 0 {
		return false
	}
	newIndex := m.historyIndex + delta
	if newIndex < 0 {
		newIndex = 0
	}
	if newIndex > len(m.commandHistory) {
		newIndex = len(m.commandHistory)
	}
	if newIndex == m.historyIndex {
		return false
	}
	m.historyIndex = newIndex
	if newIndex == len(m.commandHistory) {
		m.input.SetValue("")
	} else {
		m.input.SetValue(m.commandHistory[newIndex])
		m.input.CursorEnd()
	}
	return true
}

func (m *model) applyAutocomplete() bool {
	newValue, ok := m.autocomplete(m.input.Value())
	if !ok {
		return false
	}
	m.input.SetValue(newValue)
	m.input.CursorEnd()
	return true
}

func (m *model) autocomplete(value string) (string, bool) {
	trimmed := strings.TrimLeft(value, " ")
	leadingSpaces := len(value) - len(trimmed)
	value = trimmed

	tokens := strings.Fields(value)
	hasTrailingSpace := strings.HasSuffix(value, " ")
	withLeading := func(result string) string {
		if leadingSpaces == 0 {
			return result
		}
		return strings.Repeat(" ", leadingSpaces) + result
	}

	if len(tokens) == 0 {
		return withLeading("vms "), true
	}

	switch len(tokens) {
	case 1:
		if !hasTrailingSpace {
			matches := prefixMatches(tokens[0], rootCommands)
			if len(matches) == 0 {
				return "", false
			}
			if len(matches) == 1 {
				return withLeading(matches[0] + " "), true
			}
			prefix := longestCommonPrefix(matches)
			if len(prefix) > len(tokens[0]) {
				return withLeading(prefix), true
			}
			return "", false
		}
		// trailing space after first token
		return m.autocomplete(value + " ")
	case 2:
		first := tokens[0]
		second := tokens[1]
		if !hasTrailingSpace {
			switch first {
			case "vms", "vm":
				matches := prefixMatches(second, vmsSubcommands)
				if len(matches) == 0 {
					return "", false
				}
				if len(matches) == 1 {
					return withLeading(fmt.Sprintf("%s %s ", first, matches[0])), true
				}
				prefix := longestCommonPrefix(matches)
				if len(prefix) > len(second) {
					return withLeading(fmt.Sprintf("%s %s", first, prefix)), true
				}
				return "", false
			default:
				return "", false
			}
		}
		// trailing space after second token
		switch second {
		case "delete", "destroy", "get":
			if len(m.vms) == 0 {
				return "", false
			}
			vmNames := make([]string, len(m.vms))
			for i, vm := range m.vms {
				vmNames[i] = vm.Name
			}
			sort.Strings(vmNames)
			return withLeading(fmt.Sprintf("%s %s %s", first, second, vmNames[0])), true
		default:
			return "", false
		}
	default:
		second := tokens[1]
		partial := tokens[len(tokens)-1]
		if !hasTrailingSpace && (second == "delete" || second == "destroy" || second == "get") {
			vmNames := make([]string, len(m.vms))
			for i, vm := range m.vms {
				vmNames[i] = vm.Name
			}
			sort.Strings(vmNames)
			matches := prefixMatches(partial, vmNames)
			if len(matches) == 0 {
				return "", false
			}
			path := strings.Join(tokens[:len(tokens)-1], " ")
			if len(matches) == 1 {
				return withLeading(fmt.Sprintf("%s %s", path, matches[0])), true
			}
			prefix := longestCommonPrefix(matches)
			if len(prefix) > len(partial) {
				return withLeading(fmt.Sprintf("%s %s", path, prefix)), true
			}
		}
		return "", false
	}
}

func (m *model) ensureLogStreamForSelection() tea.Cmd {
	name := selectedVMName(m.vmList)
	if name == "" {
		return nil
	}
	if name == m.selectedVM && m.logCh != nil {
		return nil
	}
	return m.startLogStream(name)
}

func (m *model) startLogStream(name string) tea.Cmd {
	m.resetLogStream()
	m.selectedVM = name
	m.logStreamEOF = false
	m.logs = nil
	m.logView.GotoTop()
	m.logView.SetContent(fmt.Sprintf("Connecting to log stream for %s...", name))

	ch := make(chan client.VMLogEvent, 128)
	m.logCh = ch

	logCtx, cancel := context.WithCancel(m.ctx)
	m.logCancel = cancel

	return tea.Batch(
		watchLogsCmd(m.api, logCtx, name, ch),
		waitLogCmd(ch),
	)
}

func (m *model) resetLogStream() {
	if m.logCancel != nil {
		m.logCancel()
		m.logCancel = nil
	}
	m.logCh = nil
	m.logStreamEOF = false
}

func (m *model) appendLifecycleEvent(ev client.VMEvent) {
	line := ev.Message
	if line == "" {
		line = fmt.Sprintf("event: %s", ev.Type)
	}
	m.appendLogLine(ev.Timestamp, string(ev.Status), line)
}

func (m *model) appendLogLine(ts time.Time, stream, line string) {
	if line == "" {
		return
	}
	if stream == "" {
		stream = "info"
	}

	formatted := formatLogLine(ts, stream, line)
	m.logs = append([]string{formatted}, m.logs...)
	if len(m.logs) > logBufferCapacity {
		m.logs = m.logs[:logBufferCapacity]
	}

	m.logView.SetContent(strings.Join(m.logs, "\n"))
}

func selectedVMName(list list.Model) string {
	item := list.SelectedItem()
	if vmItem, ok := item.(vmItem); ok {
		return vmItem.vm.Name
	}
	return ""
}

func statusColorStyle(level statusLevel) lipgloss.Style {
	switch level {
	case statusLevelSuccess:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	case statusLevelError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	case statusLevelRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("247"))
	}
}

func executeCommand(ctx context.Context, api *client.Client, plan commandPlan) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		execCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		lines, err := plan.action(execCtx, api)
		duration := time.Since(start)
		return commandResultMsg{
			label:    plan.label,
			lines:    lines,
			err:      err,
			duration: duration,
		}
	}
}

func clearStatusLater() tea.Cmd {
	return tea.Tick(statusClearAfter, func(time.Time) tea.Msg { return clearStatusMsg{} })
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
				case ch <- client.VMEvent{
					Type:      "ERROR",
					Name:      "",
					Message:   err.Error(),
					Timestamp: time.Now().UTC(),
				}:
				default:
				}
			}
			close(ch)
		}()
		return nil
	}
}

func watchLogsCmd(api *client.Client, ctx context.Context, name string, ch chan<- client.VMLogEvent) tea.Cmd {
	return func() tea.Msg {
		go func() {
			err := api.WatchVMLogs(ctx, name, func(ev client.VMLogEvent) {
				select {
				case ch <- ev:
				case <-ctx.Done():
				}
			})
			if err != nil && ctx.Err() == nil {
				select {
				case ch <- client.VMLogEvent{
					Name:      name,
					Stream:    client.VMLogStreamStderr,
					Line:      fmt.Sprintf("log stream error: %v", err),
					Timestamp: time.Now().UTC(),
				}:
				default:
				}
			}
			close(ch)
		}()
		return nil
	}
}

func waitEventCmd(ch <-chan client.VMEvent) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return eventsClosedMsg{}
		}
		return vmEventMsg{event: ev}
	}
}

func waitLogCmd(ch <-chan client.VMLogEvent) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return logClosedMsg{source: ch}
		}
		return vmLogMsg{source: ch, event: ev}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func tokenize(input string) []string {
	return strings.Fields(strings.TrimSpace(input))
}

func prefixMatches(prefix string, options []string) []string {
	if prefix == "" {
		return append([]string{}, options...)
	}
	matches := make([]string, 0, len(options))
	for _, opt := range options {
		if strings.HasPrefix(opt, prefix) {
			matches = append(matches, opt)
		}
	}
	return matches
}

func longestCommonPrefix(items []string) string {
	if len(items) == 0 {
		return ""
	}
	prefix := items[0]
	for _, item := range items[1:] {
		for !strings.HasPrefix(item, prefix) {
			if len(prefix) == 0 {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

func formatLogLine(ts time.Time, stream, line string) string {
	timestamp := ts.Format(time.RFC3339)
	if stream != "" {
		return fmt.Sprintf("%s [%s] %s", timestamp, strings.ToUpper(stream), line)
	}
	return fmt.Sprintf("%s %s", timestamp, line)
}

func valueOrFallback(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
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
