package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/z-shell/zi-setup/internal/contract"
	"github.com/z-shell/zi-setup/internal/presentation"
	"github.com/z-shell/zi-setup/internal/workflow"
)

type Options struct {
	Theme   string
	NoColor bool
	ASCII   bool
}

type Model struct {
	ctx            context.Context
	cancel         context.CancelFunc
	session        *workflow.Session
	options        Options
	styles         styles
	viewport       viewport.Model
	width          int
	height         int
	cursor         int
	tab            int
	busy           bool
	busyLabel      string
	confirm        bool
	showDetails    bool
	err            error
	applicationErr error
}

type discoveryDone struct {
	session *workflow.Session
	err     error
}
type planDone struct {
	session *workflow.Session
	err     error
}
type applyDone struct {
	session *workflow.Session
	err     error
}

func New(ctx context.Context, session *workflow.Session, options Options) *Model {
	child, cancel := context.WithCancel(ctx)
	view := viewport.New(viewport.WithWidth(76), viewport.WithHeight(17))
	view.SoftWrap = true
	view.FillHeight = true
	return &Model{
		ctx:      child,
		cancel:   cancel,
		session:  session,
		options:  options,
		styles:   makeStyles(options),
		viewport: view,
		width:    80,
		height:   24,
	}
}

func Run(ctx context.Context, session *workflow.Session, options Options) error {
	model := New(ctx, session, options)
	program := tea.NewProgram(model, tea.WithContext(ctx))
	final, err := program.Run()
	model.cancel()
	if err != nil {
		return err
	}
	if result, ok := final.(*Model); ok && result.applicationErr != nil {
		return result.applicationErr
	}
	if result, ok := final.(*Model); ok && result.err != nil {
		return result.err
	}
	return nil
}

func (m *Model) Init() tea.Cmd {
	m.busy = true
	m.busyLabel = "Inspecting setup inputs"
	return m.discoverCmd()
}

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var command tea.Cmd
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case tea.KeyPressMsg:
		command = m.handleKey(msg.String())
	case discoveryDone:
		m.publishSession(msg.session)
		m.busy = false
		m.busyLabel = ""
		m.err = msg.err
		m.cursor = m.firstSelectable()
	case planDone:
		m.publishSession(msg.session)
		m.busy = false
		m.busyLabel = ""
		m.err = msg.err
		m.confirm = false
		m.tab = 0
		m.viewport.GotoTop()
	case applyDone:
		m.publishSession(msg.session)
		m.busy = false
		m.busyLabel = ""
		m.err = msg.err
		m.applicationErr = msg.err
		m.confirm = false
		m.viewport.GotoTop()
	}
	m.refresh()
	if !m.busy {
		var viewportCommand tea.Cmd
		m.viewport, viewportCommand = m.viewport.Update(message)
		if command == nil {
			command = viewportCommand
		} else if viewportCommand != nil {
			command = tea.Batch(command, viewportCommand)
		}
	}
	return m, command
}

func (m *Model) View() tea.View {
	content := m.header() + "\n" + m.viewport.View() + "\n" + m.footer()
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Zi Setup"
	return view
}

func (m *Model) discoverCmd() tea.Cmd {
	next := m.session.Clone()
	return func() tea.Msg {
		return discoveryDone{session: next, err: next.DiscoverEnvironment(m.ctx)}
	}
}

func (m *Model) planCmd(profile string) tea.Cmd {
	next := m.session.Clone()
	return func() tea.Msg {
		if err := next.SelectProfile(profile); err != nil {
			return planDone{session: next, err: err}
		}
		return planDone{session: next, err: next.BuildPlan(m.ctx)}
	}
}

func (m *Model) applyCmd() tea.Cmd {
	next := m.session.Clone()
	return func() tea.Msg {
		err := next.ApplyReviewedPlan(m.ctx)
		if err == nil {
			err = next.VerifyReopen(m.ctx)
		}
		return applyDone{session: next, err: err}
	}
}

func (m *Model) publishSession(next *workflow.Session) {
	if next != nil {
		*m.session = *next
	}
}

func (m *Model) handleKey(key string) tea.Cmd {
	if key == "ctrl+c" {
		if m.busy && m.session.Stage == workflow.StageApply {
			return nil
		}
		m.cancel()
		return tea.Quit
	}
	if key == "q" && !m.busy {
		m.cancel()
		return tea.Quit
	}
	if m.busy {
		return nil
	}
	if key == "d" {
		m.showDetails = !m.showDetails
		m.viewport.GotoTop()
		return nil
	}
	switch m.session.Stage {
	case workflow.StageChoose:
		switch key {
		case "up", "k", "shift+tab":
			m.moveChoice(-1)
		case "down", "j", "tab":
			m.moveChoice(1)
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.session.Describe.Profiles) {
				choice := m.session.Describe.Profiles[m.cursor]
				if choice.Selectable {
					m.busy = true
					m.busyLabel = "Creating exact plan"
					m.err = nil
					return m.planCmd(choice.ID)
				}
			}
		}
	case workflow.StageReview:
		if m.confirm {
			switch key {
			case "y", "enter":
				m.busy = true
				m.busyLabel = "Applying checkout, then files"
				m.err = nil
				return m.applyCmd()
			case "n", "esc":
				m.confirm = false
			}
			return nil
		}
		switch key {
		case "left", "h", "shift+tab":
			m.tab = (m.tab + 3) % 4
			m.viewport.GotoTop()
		case "right", "l", "tab":
			m.tab = (m.tab + 1) % 4
			m.viewport.GotoTop()
		case "a":
			m.confirm = true
			m.viewport.GotoTop()
		case "esc", "backspace":
			m.session.BackToChoose()
			m.err = nil
			m.viewport.GotoTop()
		}
	case workflow.StageResult:
		if key == "enter" {
			return tea.Quit
		}
	}
	return nil
}

func (m *Model) firstSelectable() int {
	for i, profile := range m.session.Describe.Profiles {
		if profile.Selectable {
			return i
		}
	}
	return 0
}

func (m *Model) moveChoice(delta int) {
	profiles := m.session.Describe.Profiles
	if len(profiles) == 0 {
		return
	}
	for attempts := 0; attempts < len(profiles); attempts++ {
		m.cursor = (m.cursor + delta + len(profiles)) % len(profiles)
		if profiles[m.cursor].Selectable {
			return
		}
	}
}

func (m *Model) resize() {
	width := m.width - 4
	if width < 20 {
		width = 20
	}
	height := m.height - 6
	if height < 6 {
		height = 6
	}
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(height)
}

func (m *Model) refresh() {
	m.viewport.SetContent(m.body())
}

func (m *Model) header() string {
	mark := "zi>"
	if !m.options.ASCII {
		mark = "zi›"
	}
	steps := []string{"Discover", "Choose", "Review", "Apply", "Result"}
	active := map[workflow.Stage]int{
		workflow.StageDiscover: 0,
		workflow.StageChoose:   1,
		workflow.StageReview:   2,
		workflow.StageApply:    3,
		workflow.StageResult:   4,
	}[m.session.Stage]
	for i := range steps {
		if i == active {
			steps[i] = m.styles.active.Render(steps[i])
		} else {
			steps[i] = m.styles.muted.Render(steps[i])
		}
	}
	return m.styles.wordmark.Render(mark+" setup") + "   " + strings.Join(steps, "  ")
}

func (m *Model) footer() string {
	if m.busy {
		if m.session.Stage == workflow.StageApply {
			return m.styles.active.Render(m.busyLabel) + "  operation is not interruptible"
		}
		return m.styles.active.Render(m.busyLabel) + "  ctrl+c cancel"
	}
	switch m.session.Stage {
	case workflow.StageChoose:
		return "↑/↓ choose  enter review  d details  q quit"
	case workflow.StageReview:
		if m.confirm {
			return "y/enter apply exact plan  n/esc cancel"
		}
		return "tab/←/→ preview  ↑/↓ scroll  a apply  esc back  d details  q quit"
	case workflow.StageResult:
		return "↑/↓ scroll  d details  enter/q close"
	default:
		return "ctrl+c cancel"
	}
}

func (m *Model) body() string {
	if m.busy {
		return "\n" + m.styles.active.Render(m.busyLabel) + "\n\nThe current operation has no fabricated percentage."
	}
	if m.showDetails {
		return m.detailsBody()
	}
	var body string
	switch m.session.Stage {
	case workflow.StageChoose:
		body = m.chooseBody()
	case workflow.StageReview:
		body = m.reviewBody()
	case workflow.StageResult:
		body = m.resultBody()
	default:
		body = presentation.DescribeText(m.session.Describe)
	}
	if m.err != nil {
		body = m.styles.error.Render("Error: "+presentation.SafeText(m.err.Error())) + "\n\n" + body
	}
	return body
}

func (m *Model) chooseBody() string {
	var out strings.Builder
	out.WriteString(m.styles.heading.Render("Choose a starting point") + "\n")
	out.WriteString("The engine offered these profiles for the discovered environment.\n\n")
	for i, profile := range m.session.Describe.Profiles {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		line := fmt.Sprintf("%s%s\n    %s", marker, presentation.SafeText(profile.Title), presentation.ProfileSummary(profile.ID))
		if !profile.Selectable {
			line += "\n    Unavailable: " + presentation.SafeText(profile.Reason)
			line = m.styles.muted.Render(line)
		} else if i == m.cursor {
			line = m.styles.selected.Render(line)
		}
		out.WriteString(line + "\n\n")
	}
	out.WriteString(m.styles.heading.Render("Discovered inputs") + "\n")
	for _, fact := range m.session.Describe.Facts {
		fmt.Fprintf(&out, "%-18s %s  %s/%s\n", fact.ID, presentation.SafeText(fact.Value), fact.Source, fact.Confidence)
	}
	return out.String()
}

func (m *Model) reviewBody() string {
	if m.confirm {
		return m.styles.warning.Render("Apply reviewed plan?") + "\n\n" +
			"Plan SHA-256\n" + m.styles.active.Render(m.session.Plan.ID) + "\n\n" +
			"The checkout and files phases will run separately. Each phase receives this exact hash through --expect."
	}
	tabs := []string{"Experience", "Generated Zsh", "File Diff", "Load Plan"}
	for i := range tabs {
		if i == m.tab {
			tabs[i] = m.styles.active.Render("[" + tabs[i] + "]")
		}
	}
	var preview string
	switch m.tab {
	case 0:
		preview = presentation.ExperiencePreview(m.session.Plan.Meta.Profile)
	case 1:
		preview = presentation.GeneratedText(m.session.Plan)
	case 2:
		preview = presentation.DiffText(m.session.Plan)
	case 3:
		preview = presentation.PlanText(m.session.Plan)
	}
	header := strings.Join(tabs, "  ") + "\n\n"
	if m.width >= 110 {
		leftWidth := m.width / 3
		left := lipgloss.NewStyle().Width(leftWidth).PaddingRight(2).Render(presentation.PlanText(m.session.Plan))
		right := lipgloss.NewStyle().Width(m.width - leftWidth - 6).Render(preview)
		return header + lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return header + presentation.PlanText(m.session.Plan) + "\n" + preview
}

func (m *Model) resultBody() string {
	var checkout, files *contract.Result
	if m.session.Checkout != nil {
		checkout = &m.session.Checkout.Result
	}
	if m.session.Files != nil {
		files = &m.session.Files.Result
	}
	result := presentation.ResultText(checkout, files, m.session.VerificationPlan)
	if m.session.Plan.Meta.Profile == "annex" && m.session.Files != nil && m.session.Files.Result.Status == "succeeded" {
		result += "\nAnnex recipes remain deferred until the first future shell start.\n"
	}
	return result
}

func (m *Model) detailsBody() string {
	var out strings.Builder
	out.WriteString(m.styles.heading.Render("Sanitized engine details") + "\n\n")
	appendOutput := func(label, stdout, stderr string) {
		if stdout == "" && stderr == "" {
			return
		}
		out.WriteString(m.styles.active.Render(label) + "\n")
		if stdout != "" {
			out.WriteString(presentation.SafeText(stdout) + "\n")
		}
		if stderr != "" {
			out.WriteString(presentation.SafeText(stderr) + "\n")
		}
	}
	appendOutput("Discovery", m.session.DiscoveryOutput.Stdout, m.session.DiscoveryOutput.Stderr)
	appendOutput("Plan", m.session.PlanOutput.Stdout, m.session.PlanOutput.Stderr)
	if m.session.Checkout != nil {
		appendOutput("Checkout", m.session.Checkout.Output.Stdout, m.session.Checkout.Output.Stderr)
	}
	if m.session.Files != nil {
		appendOutput("Files", m.session.Files.Output.Stdout, m.session.Files.Output.Stderr)
	}
	if out.Len() == 0 {
		return "No engine details are available."
	}
	return out.String()
}

type styles struct {
	wordmark lipgloss.Style
	heading  lipgloss.Style
	active   lipgloss.Style
	selected lipgloss.Style
	muted    lipgloss.Style
	warning  lipgloss.Style
	error    lipgloss.Style
}

func makeStyles(options Options) styles {
	base := styles{
		wordmark: lipgloss.NewStyle().Bold(true),
		heading:  lipgloss.NewStyle().Bold(true),
		active:   lipgloss.NewStyle().Bold(true),
		selected: lipgloss.NewStyle().Bold(true),
		muted:    lipgloss.NewStyle().Faint(true),
		warning:  lipgloss.NewStyle().Bold(true),
		error:    lipgloss.NewStyle().Bold(true),
	}
	if options.NoColor || options.Theme == "mono" {
		return base
	}
	accent, warning, failure := "#58C7F3", "#FFC857", "#FF6B6B"
	if options.Theme == "light" {
		accent, warning, failure = "#005A9C", "#8A5100", "#B00020"
	}
	base.wordmark = base.wordmark.Foreground(lipgloss.Color(accent))
	base.heading = base.heading.Foreground(lipgloss.Color(accent))
	base.active = base.active.Foreground(lipgloss.Color(accent))
	base.selected = base.selected.Foreground(lipgloss.Color(accent))
	base.warning = base.warning.Foreground(lipgloss.Color(warning))
	base.error = base.error.Foreground(lipgloss.Color(failure))
	return base
}
