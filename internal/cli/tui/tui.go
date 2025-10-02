//go:build ignore

// Package tui was the Bubble Tea interactive interface. It is disabled and slated for deletion.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

	"github.com/ccheshirecat/volant/internal/cli/client"
	"github.com/ccheshirecat/volant/internal/cli/standard"
)

var (
	debugWriter io.Writer
)

func init() {
	if path := os.Getenv("VOLANT_TUI_DEBUG"); path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			_ = os.MkdirAll(filepath.Dir(abs), 0o755)
			f, err := os.OpenFile(abs, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				debugWriter = f
			}
		}
	}
}

func debugLog(format string, args ...interface{}) {
	if debugWriter == nil {
		return
	}
	fmt.Fprintf(debugWriter, "%s "+format+"\n", append([]interface{}{time.Now().Format(time.RFC3339Nano)}, args...)...)
}

const (
	refreshInterval          = 5 * time.Second
	logBufferCapacity        = 200
	aguiBufferCapacity       = logBufferCapacity
	statusClearAfter         = 4 * time.Second
	defaultCreateCPUCores    = 2
	defaultCreateMemoryMB    = 2048
	layoutMinHorizontalWidth = 110
	layoutOuterMargin        = 2
	layoutPaneGap            = 2
	layoutMinPaneWidth       = 34
	layoutMinPaneHeight      = 12
	layoutPaneChromeWidth    = 4
	layoutPaneChromeHeight   = 2
	headerReservedHeight     = 1
	statusReservedHeight     = 1
	helpReservedHeight       = 1
	inputReservedHeight      = 3
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
	aguiToggleKey = key.NewBinding(
		key.WithKeys("ctrl+a"),
	)

	rootCommands    = []string{"vms", "status", "help", "mcp"}
	vmsSubcommands  = []string{"list", "get", "create", "delete", "navigate", "screenshot", "scrape", "exec", "graphql"}
	scrapeArgHints  = []string{"css=", "attr=", "value="}
	graphqlArgHints = []string{"endpoint=", "query=", "variables="}
)

var defaultMCPSuggestions = []string{"volar.vms.list", "volar.vms.create", "volar.system.get_capabilities"}

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
type aguiEventMsg struct {
	payload string
}
type aguiClosedMsg struct{}
type aguiErrorMsg struct {
	err error
}

type aguiStreamEvent struct {
	payload string
	err     error
	closed  bool
}
type commandResultMsg struct {
	label    string
	lines    []string
	err      error
	duration time.Duration
}
type clearStatusMsg struct{}

type mcpRequest struct {
	label string
	body  client.MCPRequest
}

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
	focusAGUI
)

type layoutMode int

const (
	layoutHorizontal layoutMode = iota
	layoutVertical
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

	logCh            chan client.VMLogEvent
	logCancel        context.CancelFunc
	logStreamEOF     bool
	aguiCh           chan aguiStreamEvent
	aguiCancel       context.CancelFunc
	aguiStreamEOF    bool
	aguiActive       bool
	aguiBuffer       []string
	aguiError        error
	showAGUI         bool
	aguiLogs         []string
	aguiStreamCancel context.CancelFunc

	logs       []string
	selectedVM string

	// UI components
	vmList   list.Model
	logView  viewport.Model
	aguiView viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	focused paneFocus
	layout  layoutMode
	width   int
	height  int

	// Command execution
	executing        bool
	commandHistory   []string
	historyIndex     int
	statusMessage    string
	statusLevel      statusLevel
	statusStarted    time.Time
	pendingClear     bool
	statusPersistent bool
	mcpSuggest       []string
	pendingMCP       *mcpRequest
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
	m.statusPersistent = false
}

func (m *model) setFocus(target paneFocus) {
	if target == m.focused {
		return
	}
	if target == focusInput {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
	m.focused = target
}

func (m *model) advanceFocus() {
	switch m.focused {
	case focusVMList:
		m.setFocus(focusLogs)
	case focusLogs:
		m.setFocus(focusInput)
	case focusInput:
		m.setFocus(focusAGUI)
	case focusAGUI:
		m.setFocus(focusVMList)
	default:
		m.setFocus(focusVMList)
	}
}

func (m *model) updateStatus(level statusLevel, message string, persistent bool) tea.Cmd {
	if message == "" {
		return nil
	}
	if m.executing && !persistent {
		return nil
	}
	m.statusMessage = message
	m.statusLevel = level
	m.statusPersistent = persistent
	if persistent {
		m.pendingClear = false
		return nil
	}
	return m.ensureStatusClear()
}

func (m *model) setStatus(level statusLevel, message string) tea.Cmd {
	return m.updateStatus(level, message, false)
}

func (m *model) setPersistentStatus(level statusLevel, message string) {
	m.updateStatus(level, message, true)
}

func (m *model) applyResponsiveLayout(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}

	usableWidth := maxInt(width-(layoutOuterMargin*2), layoutMinPaneWidth)
	topSections := headerReservedHeight + statusReservedHeight
	bottomSections := helpReservedHeight + inputReservedHeight
	usableHeight := maxInt(height-(topSections+bottomSections), layoutMinPaneHeight)

	// Choose layout orientation.
	if usableWidth >= layoutMinHorizontalWidth {
		m.layout = layoutHorizontal
	} else {
		m.layout = layoutVertical
	}

	var paneWidth, paneHeight int
	switch m.layout {
	case layoutHorizontal:
		paneWidth = (usableWidth - layoutPaneGap) / 2
		if paneWidth < layoutMinPaneWidth {
			paneWidth = layoutMinPaneWidth
		}
		paneHeight = usableHeight - layoutPaneChromeHeight
	default:
		paneWidth = usableWidth
		paneHeight = (usableHeight - layoutPaneGap) / 2
	}

	if paneHeight < layoutMinPaneHeight {
		paneHeight = layoutMinPaneHeight
	}

	paneContentWidth := paneWidth - layoutPaneChromeWidth
	if paneContentWidth < layoutMinPaneWidth {
		paneContentWidth = layoutMinPaneWidth
	}

	m.vmList.SetWidth(paneContentWidth)
	m.vmList.SetHeight(paneHeight)
	m.logView.Width = paneContentWidth
	m.logView.Height = paneHeight
	m.aguiView.Width = paneContentWidth
	m.aguiView.Height = paneHeight

	inputWidth := usableWidth
	if inputWidth < layoutMinPaneWidth {
		inputWidth = layoutMinPaneWidth
	}
	m.input.Width = inputWidth
}

func newModel(ctx context.Context, cancel context.CancelFunc, api *client.Client) model {
	sp := newSpinnerModel()

	m := model{
		ctx:        ctx,
		cancel:     cancel,
		api:        api,
		eventCh:    make(chan client.VMEvent, 64),
		vmList:     list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
		logView:    viewport.New(0, 0),
		aguiView:   viewport.New(0, 0),
		input:      textinput.New(),
		spinner:    sp,
		focused:    focusInput,
		layout:     layoutVertical,
		logs:       []string{},
		selectedVM: "",
		aguiBuffer: []string{},
	}

	m.aguiView.MouseWheelEnabled = true
	m.aguiView.SetContent("Activate AG-UI pane (Ctrl+A) to stream events.")

	m.input.Placeholder = "Enter command (e.g., vms create my-vm)..."
	m.input.CharLimit = 512
	m.input.Width = layoutMinPaneWidth
	m.input.Prompt = "» "

	m.logView.Width = layoutMinPaneWidth
	m.logView.Height = layoutMinPaneHeight
	m.logView.YPosition = 1
	m.logView.MouseWheelEnabled = true
	m.logView.SetContent("Select a VM to begin streaming logs.")

	m.mcpSuggest = append([]string{}, defaultMCPSuggestions...)

	m.input.Focus()
	m.vmList.SetHeight(layoutMinPaneHeight)
	m.logView.Height = layoutMinPaneHeight
	m.logView.Width = layoutMinPaneWidth
	m.input.Width = layoutMinPaneWidth
	m.aguiView.Width = layoutMinPaneWidth
	m.aguiView.Height = layoutMinPaneHeight

	debugLog("model initialized")

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

		m.applyResponsiveLayout(msg.Width, msg.Height)

	case tea.KeyMsg:
		if key.Matches(msg, aguiToggleKey) {
			cmd := m.toggleAGUI()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			break
		}
		switch m.focused {
		case focusVMList:
			prevSelection := selectedVMName(m.vmList)

			if key.Matches(msg, tabKey) {
				debugLog("focus cycle VM -> next")
				m.advanceFocus()
				break
			}

			var cmd tea.Cmd
			m.vmList, cmd = m.vmList.Update(msg)
			vmListUpdated = true
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

			if key.Matches(msg, enterKey) {
				m.setFocus(focusInput)
			}

			currentSelection := selectedVMName(m.vmList)
			if currentSelection != "" && currentSelection != prevSelection {
				if logCmd := m.ensureLogStreamForSelection(); logCmd != nil {
					cmds = append(cmds, logCmd)
				}
			}

		case focusLogs:
			if key.Matches(msg, tabKey) {
				debugLog("focus cycle Logs -> next")
				m.advanceFocus()
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
				value := m.input.Value()
				trimmed := strings.TrimSpace(value)
				trailingSpace := strings.HasSuffix(value, " ")
				if trimmed != "" && !trailingSpace {
					if m.applyAutocomplete() {
						debugLog("autocomplete applied: %q", m.input.Value())
						break
					}
				}
				debugLog("focus cycle Input -> next")
				m.advanceFocus()
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
		m.setFocus(focusInput)

		if msg.err != nil {
			if cmd := m.setStatus(statusLevelError, fmt.Sprintf("%s failed: %v", msg.label, msg.err)); cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else {
			if msg.label == "help" {
				m.setPersistentStatus(statusLevelInfo, "Help displayed below. Scroll the log pane to read.")
			} else {
				if cmd := m.setStatus(statusLevelSuccess, fmt.Sprintf("%s succeeded in %s", msg.label, msg.duration.Round(10*time.Millisecond))); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

		for _, line := range msg.lines {
			m.appendLogLine(time.Now().UTC(), "cli", line)
		}

		if msg.err == nil && msg.label != "help" {
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
		m.setPersistentStatus(statusLevelError, "Event stream closed. Reconnecting...")
		if cmd := m.restartEventStream(); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tickMsg:
		cmds = append(cmds,
			tickCmd(),
			fetchVMsCmd(m.api, m.ctx),
			fetchStatusCmd(m.api, m.ctx),
		)

	case aguiEventMsg:
		m.appendAGUILine(msg.payload)
		if m.aguiCh != nil {
			cmds = append(cmds, waitAGUIEvent(m.aguiCh))
		}

	case aguiClosedMsg:
		m.aguiStreamEOF = true
		if m.aguiStreamCancel != nil {
			m.aguiStreamCancel()
			m.aguiStreamCancel = nil
		}
		m.aguiCh = nil
		if cmd := m.setStatus(statusLevelInfo, "AG-UI stream closed"); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case aguiErrorMsg:
		m.aguiError = msg.err
		if cmd := m.setStatus(statusLevelError, fmt.Sprintf("AG-UI stream error: %v", msg.err)); cmd != nil {
			cmds = append(cmds, cmd)
		}
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

	usableWidth := m.width - 8
	var paneWidth int
	var paneHeight int
	inputWidth := usableWidth
	if m.layout == layoutHorizontal {
		paneWidth = usableWidth / 2
		if paneWidth < 32 {
			paneWidth = 32
		}
		paneHeight = m.height - 8
		if paneHeight < 12 {
			paneHeight = 12
		}
		inputWidth = m.width - 6
		if inputWidth < 30 {
			inputWidth = m.width - 4
		}
	} else {
		paneWidth = usableWidth
		if paneWidth < 32 {
			paneWidth = usableWidth
		}
		paneHeight = (m.height - 12) / 2
		if paneHeight < 10 {
			paneHeight = 10
		}
		inputWidth = m.width - 6
		if inputWidth < 30 {
			inputWidth = m.width - 4
		}
	}
	if inputWidth < 10 {
		inputWidth = 10
	}

	paneStyle := lipgloss.NewStyle().Width(paneWidth).Height(paneHeight)

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
	headerTitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Bold(true)
	statusStyle := lipgloss.NewStyle().Padding(0, 1)
	inputStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)

	header := headerStyle.Render(headerTitleStyle.Render("VOLAR | CONTROL PLANE"))

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

	vmListView := paneStyle.Render(m.vmList.View())

	logTitle := "Logs"
	logContentBody := m.logView.View()
	if m.showAGUI {
		logTitle = "AG-UI Events"
		logContentBody = m.aguiView.View()
	}
	if m.selectedVM != "" && !m.showAGUI {
		logTitle = fmt.Sprintf("Logs (%s)", m.selectedVM)
	}
	logContent := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Render(logTitle),
		logContentBody,
	)
	logView := paneStyle.Render(logContent)

	inputView := inputStyle.Width(inputWidth).Render(m.input.View())

	help := helpStyle.Render("(tab: switch/autocomplete) (ctrl+w: clear input) (↑/↓: history) (q: quit)")

	var panes string
	if m.layout == layoutHorizontal {
		panes = lipgloss.JoinHorizontal(lipgloss.Top, vmListView, logView)
	} else {
		panes = lipgloss.JoinVertical(lipgloss.Left, vmListView, logView)
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		statusView,
		panes,
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
		if cmd := m.setStatus(statusLevelError, "Another command is already running."); cmd != nil {
			debugLog("command ignored while executing: %s", raw)
			return cmd
		}
		return nil
	}

	m.appendToHistory(raw)
	debugLog("queue command: %s", raw)

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

	m.setFocus(focusInput)

	return tea.Batch(
		executeCommand(m.ctx, m.api, plan),
		m.spinner.Tick,
	)
}

func (m *model) restartEventStream() tea.Cmd {
	if m.ctx.Err() != nil {
		return nil
	}
	m.eventCh = make(chan client.VMEvent, 64)
	m.streamEOF = false
	return tea.Batch(
		watchEventsCmd(m.api, m.ctx, m.eventCh),
		waitEventCmd(m.eventCh),
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

	case "mcp":
		if len(parts) < 2 {
			return commandPlan{}, fmt.Errorf("mcp command requires a method (e.g., mcp volar.vms.list)")
		}
		commandName := parts[1]
		var params map[string]any
		if len(parts) > 2 {
			joined := strings.Join(parts[2:], " ")
			if err := json.Unmarshal([]byte(joined), &params); err != nil {
				return commandPlan{}, fmt.Errorf("invalid MCP params JSON: %w", err)
			}
		}

		return commandPlan{
			label: fmt.Sprintf("mcp %s", commandName),
			action: func(ctx context.Context, api *client.Client) ([]string, error) {
				req := client.MCPRequest{Command: commandName, Params: params}
				resp, err := api.MCP(ctx, req)
				if err != nil {
					return nil, err
				}
				payload, marshalErr := json.MarshalIndent(resp, "", "  ")
				if marshalErr != nil {
					return nil, marshalErr
				}
				return []string{string(payload)}, nil
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

		case "navigate":
			if len(parts) < 4 {
				return commandPlan{}, fmt.Errorf("vms navigate requires a VM name and URL")
			}
			name := parts[2]
			url := parts[3]
			return commandPlan{
				label: fmt.Sprintf("vms navigate %s", name),
				action: func(ctx context.Context, api *client.Client) ([]string, error) {
					payload := client.NavigateActionRequest{URL: url}
					if err := api.NavigateVM(ctx, name, payload); err != nil {
						return nil, err
					}
					return []string{fmt.Sprintf("Navigation requested for %s", name)}, nil
				},
			}, nil

		case "screenshot":
			if len(parts) < 3 {
				return commandPlan{}, fmt.Errorf("vms screenshot requires a VM name")
			}
			name := parts[2]
			flags := parts[3:]
			var output string
			payload := client.ScreenshotActionRequest{}
			for i := 0; i < len(flags); i++ {
				token := flags[i]
				switch {
				case strings.HasPrefix(token, "--output="):
					output = strings.TrimPrefix(token, "--output=")
				case token == "--output" && i+1 < len(flags):
					i++
					output = flags[i]
				case token == "--full-page":
					payload.FullPage = true
				case strings.HasPrefix(token, "--format="):
					payload.Format = strings.TrimPrefix(token, "--format=")
				}
			}
			if payload.Format == "" {
				payload.Format = "png"
			}
			return commandPlan{
				label: fmt.Sprintf("vms screenshot %s", name),
				action: func(ctx context.Context, api *client.Client) ([]string, error) {
					resp, err := api.ScreenshotVM(ctx, name, payload)
					if err != nil {
						return nil, err
					}
					data, decodeErr := standard.DecodeBase64(resp.Data)
					if decodeErr != nil {
						return nil, decodeErr
					}
					filePath := output
					if strings.TrimSpace(filePath) == "" {
						suffix := resp.Format
						if suffix == "" {
							suffix = payload.Format
						}
						if suffix == "" {
							suffix = "png"
						}
						filePath = fmt.Sprintf("%s_%d.%s", name, time.Now().Unix(), suffix)
					}
					if err := os.WriteFile(filePath, data, 0o644); err != nil {
						return nil, err
					}
					return []string{fmt.Sprintf("Screenshot saved to %s (%d bytes)", filePath, len(data))}, nil
				},
			}, nil

		case "scrape":
			if len(parts) < 3 {
				return commandPlan{}, fmt.Errorf("vms scrape requires a VM name")
			}
			name := parts[2]
			if len(parts) < 4 {
				return commandPlan{}, fmt.Errorf("vms scrape requires selector argument (e.g. css=.btn)")
			}
			selector := ""
			attribute := ""
			attrValue := ""
			for _, opt := range parts[3:] {
				if kv := strings.SplitN(opt, "=", 2); len(kv) == 2 {
					switch kv[0] {
					case "css":
						selector = kv[1]
					case "attr":
						attribute = kv[1]
					case "value":
						attrValue = kv[1]
					}
				}
			}
			if strings.TrimSpace(selector) == "" {
				return commandPlan{}, fmt.Errorf("selector is required for scrape (use css=.selector)")
			}
			return commandPlan{
				label: fmt.Sprintf("vms scrape %s", name),
				action: func(ctx context.Context, api *client.Client) ([]string, error) {
					payload := client.ScrapeActionRequest{Selector: selector, Attribute: attribute}
					resp, err := api.ScrapeVM(ctx, name, payload)
					if err != nil {
						return nil, err
					}
					output := map[string]any{"exists": resp.Exists, "value": resp.Value}
					if attrValue != "" && resp.Exists {
						output["matches"] = (fmt.Sprint(resp.Value) == attrValue)
					}
					data, err := json.MarshalIndent(output, "", "  ")
					if err != nil {
						return nil, err
					}
					return []string{string(data)}, nil
				},
			}, nil

		case "exec":
			if len(parts) < 3 {
				return commandPlan{}, fmt.Errorf("vms exec requires a VM name")
			}
			name := parts[2]
			if len(parts) < 4 {
				return commandPlan{}, fmt.Errorf("vms exec requires a JavaScript expression")
			}
			expression := strings.Join(parts[3:], " ")
			return commandPlan{
				label: fmt.Sprintf("vms exec %s", name),
				action: func(ctx context.Context, api *client.Client) ([]string, error) {
					payload := client.EvaluateActionRequest{Expression: expression, AwaitPromise: true}
					resp, err := api.EvaluateVM(ctx, name, payload)
					if err != nil {
						return nil, err
					}
					data, err := json.MarshalIndent(resp, "", "  ")
					if err != nil {
						return nil, err
					}
					return []string{string(data)}, nil
				},
			}, nil

		case "graphql":
			if len(parts) < 3 {
				return commandPlan{}, fmt.Errorf("vms graphql requires a VM name")
			}
			name := parts[2]
			endpoint := ""
			query := ""
			variablesJSON := ""
			for _, opt := range parts[3:] {
				if kv := strings.SplitN(opt, "=", 2); len(kv) == 2 {
					switch kv[0] {
					case "endpoint":
						endpoint = kv[1]
					case "query":
						query = kv[1]
					case "variables":
						variablesJSON = kv[1]
					}
				}
			}
			if endpoint == "" {
				return commandPlan{}, fmt.Errorf("endpoint is required (endpoint=https://...)")
			}
			if query == "" {
				return commandPlan{}, fmt.Errorf("query is required (query='{...}')")
			}
			return commandPlan{
				label: fmt.Sprintf("vms graphql %s", name),
				action: func(ctx context.Context, api *client.Client) ([]string, error) {
					payload := client.GraphQLActionRequest{Endpoint: endpoint, Query: query}
					if variablesJSON != "" {
						var vars map[string]any
						if err := json.Unmarshal([]byte(variablesJSON), &vars); err != nil {
							return nil, fmt.Errorf("decode variables JSON: %w", err)
						}
						payload.Variables = vars
					}
					resp, err := api.GraphQLVM(ctx, name, payload)
					if err != nil {
						return nil, err
					}
					data, err := json.MarshalIndent(resp, "", "  ")
					if err != nil {
						return nil, err
					}
					return []string{string(data)}, nil
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
	m.statusPersistent = false
	debugLog("history appended: %s", cmd)
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
		case "delete", "destroy", "get", "navigate", "screenshot", "scrape", "exec", "graphql":
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
		first := tokens[0]
		second := tokens[1]
		partial := tokens[len(tokens)-1]

		if first == "vms" || first == "vm" {
			switch second {
			case "delete", "destroy", "get", "navigate", "screenshot", "scrape", "exec", "graphql":
				// Suggest VM names for third arg
				if len(tokens) == 3 && !hasTrailingSpace {
					vmNames := make([]string, len(m.vms))
					for i, vm := range m.vms {
						vmNames[i] = vm.Name
					}
					sort.Strings(vmNames)
					matches := prefixMatches(partial, vmNames)
					if len(matches) == 0 {
						return "", false
					}
					prefix := longestCommonPrefix(matches)
					if len(matches) == 1 {
						return withLeading(fmt.Sprintf("%s %s %s", first, second, matches[0])), true
					}
					if len(prefix) > len(partial) {
						return withLeading(strings.Join(append(tokens[:len(tokens)-1], prefix), " ")), true
					}
				}

				// Specialized hints after VM name
				if len(tokens) >= 3 {
					switch second {
					case "screenshot":
						hints := []string{"--output=", "--full-page", fmt.Sprintf("--format=%s", partial)}
						return applyOptionAutocomplete(tokens, hints, withLeading)
					case "scrape":
						return applyOptionAutocomplete(tokens, scrapeArgHints, withLeading)
					case "graphql":
						return applyOptionAutocomplete(tokens, graphqlArgHints, withLeading)
					}
				}
			}
		}
		return "", false
	}
}

func applyOptionAutocomplete(tokens []string, hints []string, withLeading func(string) string) (string, bool) {
	current := tokens[len(tokens)-1]
	matches := prefixMatches(current, hints)
	if len(matches) == 0 {
		return "", false
	}
	if len(matches) == 1 {
		return withLeading(strings.Join(append(tokens[:len(tokens)-1], matches[0]), " ")), true
	}
	prefix := longestCommonPrefix(matches)
	if len(prefix) > len(current) {
		return withLeading(strings.Join(append(tokens[:len(tokens)-1], prefix), " ")), true
	}
	return "", false
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

func toggleAGUIKeyBinding() key.Binding {
	return aguiToggleKey
}

func (m *model) toggleAGUI() tea.Cmd {
	m.showAGUI = !m.showAGUI
	if m.showAGUI {
		m.aguiStreamEOF = false
		m.aguiError = nil
		m.aguiLogs = []string{"Connecting to AG-UI event stream..."}
		m.aguiView.SetContent(strings.Join(m.aguiLogs, "\n"))

		ctx, cancel := context.WithCancel(m.ctx)
		m.aguiStreamCancel = cancel
		m.aguiCh = make(chan aguiStreamEvent, 64)

		return tea.Batch(startAGUIStream(m.api, ctx, m.aguiCh), waitAGUIEvent(m.aguiCh))
	}

	if m.aguiStreamCancel != nil {
		m.aguiStreamCancel()
		m.aguiStreamCancel = nil
	}
	m.aguiCh = nil
	m.aguiView.SetContent("AG-UI stream disconnected. Press Ctrl+A to reconnect.")
	return nil
}

func startAGUIStream(api *client.Client, ctx context.Context, ch chan<- aguiStreamEvent) tea.Cmd {
	return func() tea.Msg {
		go func() {
			err := api.WatchAGUI(ctx, func(payload string) {
				select {
				case ch <- aguiStreamEvent{payload: payload}:
				case <-ctx.Done():
				}
			})
			if err != nil && ctx.Err() == nil {
				ch <- aguiStreamEvent{err: err}
			}
			close(ch)
		}()
		return nil
	}
}

func waitAGUIEvent(ch <-chan aguiStreamEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return aguiClosedMsg{}
		}
		if ev.err != nil {
			return aguiErrorMsg{err: ev.err}
		}
		return aguiEventMsg{payload: ev.payload}
	}
}

func (m *model) appendAGUILine(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	m.aguiLogs = append(m.aguiLogs, line)
	if len(m.aguiLogs) > aguiBufferCapacity {
		m.aguiLogs = m.aguiLogs[len(m.aguiLogs)-aguiBufferCapacity:]
	}
	m.aguiView.SetContent(strings.Join(m.aguiLogs, "\n"))
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Run launches the Bubble Tea TUI.
func Run() error {
	base := os.Getenv("VOLANT_API_BASE")
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
