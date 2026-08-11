package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"

	"github.com/kedify/cli/internal/prometheus"
)

const (
	metricsPane = iota
	labelsPane
	valuesPane
)

const (
	browseStage = iota
	editStage
	previewStage
	horizonStage
	graphLoadingStage
	graphStage
	targetsStage
)

type metricHorizon struct {
	argument string
	label    string
	duration time.Duration
	step     time.Duration
}

var metricHorizons = [...]metricHorizon{
	{argument: "6h", label: "last 6 hours", duration: 6 * time.Hour, step: time.Minute},
	{argument: "1d", label: "last day", duration: 24 * time.Hour, step: 5 * time.Minute},
	{argument: "3d", label: "last 3 days", duration: 3 * 24 * time.Hour, step: 15 * time.Minute},
	{argument: "1w", label: "last week", duration: 7 * 24 * time.Hour, step: 30 * time.Minute},
	{argument: "30d", label: "last month", duration: 30 * 24 * time.Hour, step: 2 * time.Hour},
}

var (
	promQLIdentifierPattern = regexp.MustCompile(`[a-zA-Z_:][a-zA-Z0-9_:]*`)
	promQLLabelListPattern  = regexp.MustCompile(`(?i)\b(?:by|without|on|ignoring|group_left|group_right)\s*\([^)]*\)`)
)

var (
	metricsTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	metricsActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	metricsSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62"))
	metricsHintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	metricsErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	metricsQueryStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("229"))
)

type MetricsClient interface {
	LabelNames(context.Context, string) ([]string, error)
	LabelValues(context.Context, string, string) ([]string, error)
	ValidateQuery(context.Context, string) error
	RangeQuery(context.Context, string, time.Time, time.Time, time.Duration) ([]prometheus.Series, error)
}

type MetricsResult struct {
	Query                   string
	GenerateScaledObject    bool
	GenerateMetricPredictor bool
	CreateResources         bool
}

type MetricsExplorerOptions struct {
	Filter    string
	Query     string
	Visualize bool
	Horizon   string
}

type metricsModel struct {
	client MetricsClient

	metrics       []string
	filtered      []string
	metricCursor  int
	labels        []string
	labelCursor   int
	labelValues   []string
	valueCursor   int
	selected      map[string]string
	pane          int
	stage         int
	filter        string
	valueFilter   string
	searchMode    bool
	loading       bool
	err           string
	editor        lineEditor
	previewCursor int
	horizonCursor int
	autoVisualize bool
	autoGraph     bool
	graph         string
	targets       [3]bool
	targetCursor  int
	width         int
	height        int
	done          bool
	quit          bool
}

type labelsLoadedMsg struct {
	metric string
	labels []string
	err    error
}

type valuesLoadedMsg struct {
	label  string
	values []string
	err    error
}

type queryValidatedMsg struct {
	err error
}

type graphLoadedMsg struct {
	graph string
	err   error
}

type lineEditor struct {
	value  []rune
	cursor int
}

func RunMetricsExplorer(
	stdin io.Reader,
	stdout, stderr io.Writer,
	client MetricsClient,
	metrics []string,
	options MetricsExplorerOptions,
) (MetricsResult, error) {
	file, err := interactiveFile(stdin)
	if err != nil {
		return MetricsResult{}, err
	}
	if len(metrics) == 0 && strings.TrimSpace(options.Query) == "" {
		return MetricsResult{}, errors.New("prometheus returned no metrics")
	}

	model, err := newMetricsModelWithOptions(client, metrics, options)
	if err != nil {
		return MetricsResult{}, err
	}
	result, err := tea.NewProgram(
		model,
		tea.WithInput(file),
		tea.WithOutput(promptOutput(stdout, stderr)),
	).Run()
	if err != nil {
		return MetricsResult{}, fmt.Errorf("run metrics explorer: %w", err)
	}

	finalModel, ok := result.(metricsModel)
	if !ok {
		return MetricsResult{}, errors.New("unexpected metrics explorer state")
	}
	if finalModel.quit {
		return MetricsResult{}, errors.New("metrics exploration canceled")
	}
	if !finalModel.done {
		return MetricsResult{}, errors.New("metrics exploration did not finish")
	}

	return MetricsResult{
		Query:                   strings.TrimSpace(finalModel.editor.String()),
		GenerateScaledObject:    finalModel.targets[0],
		GenerateMetricPredictor: finalModel.targets[1],
		CreateResources:         finalModel.targets[2],
	}, nil
}

func newMetricsModel(client MetricsClient, metricNames []string) metricsModel {
	model, _ := newMetricsModelWithOptions(client, metricNames, MetricsExplorerOptions{})
	return model
}

func newMetricsModelWithOptions(
	client MetricsClient,
	metricNames []string,
	options MetricsExplorerOptions,
) (metricsModel, error) {
	metrics := append([]string(nil), metricNames...)
	sort.Strings(metrics)
	horizonCursor, err := metricHorizonIndex(options.Horizon)
	if err != nil {
		return metricsModel{}, err
	}
	model := metricsModel{
		client:        client,
		metrics:       metrics,
		selected:      make(map[string]string),
		filter:        options.Filter,
		horizonCursor: horizonCursor,
		autoVisualize: options.Visualize,
		autoGraph:     strings.TrimSpace(options.Horizon) != "",
		width:         100,
		height:        30,
	}
	model.filterMetricNames()
	model.loading = model.currentMetric() != ""
	model.resetEditor()
	if query := strings.TrimSpace(options.Query); query != "" {
		model.editor.Set(query)
		model.stage = editStage
		model.loading = true
	}
	return model, nil
}

func (m metricsModel) Init() tea.Cmd {
	if m.stage == editStage && m.loading {
		return m.validateQuery(strings.TrimSpace(m.editor.String()))
	}
	return m.loadLabels()
}

func (m metricsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case labelsLoadedMsg:
		if msg.metric != m.currentMetric() {
			return m, nil
		}
		if m.stage == browseStage {
			m.loading = false
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			m.labels = nil
			return m, nil
		}
		m.labels = msg.labels
		m.labelCursor = clampCursor(m.labelCursor, len(m.labels))
		m.err = ""
		return m, nil
	case valuesLoadedMsg:
		if m.stage != browseStage || m.pane != valuesPane || m.currentLabel() != msg.label {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			m.labelValues = nil
			return m, nil
		}
		m.labelValues = msg.values
		m.valueCursor = clampCursor(m.valueCursor, len(m.filteredValues()))
		m.err = ""
		return m, nil
	case queryValidatedMsg:
		if m.stage != editStage || !m.loading {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.previewCursor = 0
		if m.autoGraph {
			return m.startGraph()
		}
		if m.autoVisualize {
			m.stage = horizonStage
			return m, nil
		}
		m.stage = previewStage
		return m, nil
	case graphLoadedMsg:
		if m.stage != graphLoadingStage || !m.loading {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			m.stage = horizonStage
			return m, nil
		}
		m.graph = msg.graph
		m.err = ""
		m.stage = graphStage
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quit = true
			return m, tea.Quit
		}
		switch m.stage {
		case browseStage:
			return m.updateBrowse(msg)
		case editStage:
			return m.updateEdit(msg)
		case previewStage:
			return m.updatePreview(msg)
		case horizonStage:
			return m.updateHorizon(msg)
		case graphLoadingStage:
			if msg.Type == tea.KeyEsc {
				m.stage = horizonStage
				m.loading = false
			}
			return m, nil
		case graphStage:
			return m.updateGraph(msg)
		case targetsStage:
			return m.updateTargets(msg)
		}
	}
	return m, nil
}

func (m metricsModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlE {
		m.stage = editStage
		m.loading = false
		m.err = ""
		return m, nil
	}

	if m.searchMode {
		switch msg.Type {
		case tea.KeyEsc, tea.KeyEnter:
			m.searchMode = false
			return m, nil
		case tea.KeyBackspace:
			if m.pane == valuesPane {
				m.valueFilter = removeLastRune(m.valueFilter)
				m.valueCursor = 0
				return m, nil
			}
			m.filter = removeLastRune(m.filter)
			return m.applyMetricFilter()
		default:
			if msg.Type == tea.KeyRunes {
				if m.pane == valuesPane {
					m.valueFilter += string(msg.Runes)
					m.valueCursor = 0
					return m, nil
				}
				m.filter += string(msg.Runes)
				return m.applyMetricFilter()
			}
		}
		return m, nil
	}

	if m.pane == metricsPane {
		switch msg.Type {
		case tea.KeyRunes:
			m.filter += string(msg.Runes)
			return m.applyMetricFilter()
		case tea.KeyBackspace:
			if m.filter == "" {
				return m, nil
			}
			m.filter = removeLastRune(m.filter)
			return m.applyMetricFilter()
		}
	}

	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "/":
		if m.pane == valuesPane {
			m.searchMode = true
		}
	case "tab":
		if m.pane == metricsPane && len(m.labels) > 0 {
			m.pane = labelsPane
		} else {
			m.pane = metricsPane
		}
	case "left", "h", "esc":
		if m.pane == valuesPane {
			m.pane = labelsPane
			m.valueFilter = ""
			m.err = ""
		} else if m.pane == labelsPane {
			m.pane = metricsPane
		} else if m.filter != "" {
			m.filter = ""
			return m.applyMetricFilter()
		}
	case "right", "l", "enter":
		switch m.pane {
		case metricsPane:
			if len(m.labels) > 0 {
				m.pane = labelsPane
			}
		case labelsPane:
			if m.currentLabel() != "" {
				m.pane = valuesPane
				m.valueCursor = 0
				m.valueFilter = ""
				m.labelValues = nil
				m.loading = true
				m.err = ""
				return m, m.loadValues()
			}
		case valuesPane:
			values := m.filteredValues()
			if len(values) > 0 {
				m.selected[m.currentLabel()] = values[m.valueCursor]
				m.pane = labelsPane
				m.valueFilter = ""
				m.resetEditor()
			}
		}
	case "up", "k":
		switch m.pane {
		case metricsPane:
			if m.metricCursor > 0 {
				m.metricCursor--
				return m.metricChanged()
			}
		case labelsPane:
			if m.labelCursor > 0 {
				m.labelCursor--
			}
		case valuesPane:
			if m.valueCursor > 0 {
				m.valueCursor--
			}
		}
	case "down", "j":
		switch m.pane {
		case metricsPane:
			if m.metricCursor < len(m.filtered)-1 {
				m.metricCursor++
				return m.metricChanged()
			}
		case labelsPane:
			if m.labelCursor < len(m.labels)-1 {
				m.labelCursor++
			}
		case valuesPane:
			if m.valueCursor < len(m.filteredValues())-1 {
				m.valueCursor++
			}
		}
	case "pgup":
		if m.pane == metricsPane && m.metricCursor > 0 {
			m.metricCursor = max(0, m.metricCursor-m.metricPageSize())
			return m.metricChanged()
		}
	case "pgdown":
		if m.pane == metricsPane && m.metricCursor < len(m.filtered)-1 {
			m.metricCursor = min(len(m.filtered)-1, m.metricCursor+m.metricPageSize())
			return m.metricChanged()
		}
	case "backspace", "delete", "x":
		if m.pane == labelsPane {
			delete(m.selected, m.currentLabel())
			m.resetEditor()
		}
	}
	return m, nil
}

func (m metricsModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.loading {
		if msg.Type == tea.KeyEsc {
			m.loading = false
			m.stage = browseStage
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.stage = browseStage
		m.err = ""
		return m, nil
	case tea.KeyEnter:
		query := strings.TrimSpace(m.editor.String())
		if query == "" {
			m.err = "PromQL query cannot be empty."
			return m, nil
		}
		m.loading = true
		m.err = ""
		return m, m.validateQuery(query)
	default:
		m.editor.Update(msg)
		m.err = ""
		return m, nil
	}
}

func (m metricsModel) updatePreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+e":
		m.stage = editStage
		m.err = ""
	case "up", "down", "j", "k", "tab":
		if m.previewCursor == 0 {
			m.previewCursor = 1
		} else {
			m.previewCursor = 0
		}
	case "g":
		m.previewCursor = 0
		m.stage = horizonStage
	case "c":
		m.previewCursor = 1
		m.stage = targetsStage
	case "enter":
		if m.previewCursor == 0 {
			m.stage = horizonStage
			return m, nil
		}
		m.stage = targetsStage
	}
	return m, nil
}

func (m metricsModel) updateHorizon(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.stage = previewStage
		m.err = ""
	case "ctrl+e":
		m.stage = editStage
		m.err = ""
	case "up", "k":
		if m.horizonCursor > 0 {
			m.horizonCursor--
		}
	case "down", "j":
		if m.horizonCursor < len(metricHorizons)-1 {
			m.horizonCursor++
		}
	case "enter":
		return m.startGraph()
	}
	return m, nil
}

func (m metricsModel) updateGraph(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.stage = horizonStage
	case "ctrl+e":
		m.stage = editStage
	case "enter", "c":
		m.stage = targetsStage
	}
	return m, nil
}

func (m metricsModel) updateTargets(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.err = ""
	switch msg.String() {
	case "esc", "ctrl+e":
		m.stage = editStage
	case "up", "k":
		if m.targetCursor > 0 {
			m.targetCursor--
		}
	case "down", "j":
		if m.targetCursor < len(m.targets)-1 {
			m.targetCursor++
		}
	case " ":
		m.targets[m.targetCursor] = !m.targets[m.targetCursor]
	case "enter":
		if m.targets[2] && !m.targets[0] && !m.targets[1] {
			m.err = "Select at least one resource manifest before enabling creation."
			return m, nil
		}
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m metricsModel) startGraph() (tea.Model, tea.Cmd) {
	m.stage = graphLoadingStage
	m.loading = true
	m.err = ""
	query := strings.TrimSpace(m.editor.String())
	width := m.width
	horizon := m.currentHorizon()
	return m, func() tea.Msg {
		end := time.Now()
		start := end.Add(-horizon.duration)
		series, err := m.client.RangeQuery(context.Background(), query, start, end, horizon.step)
		if err != nil {
			return graphLoadedMsg{err: err}
		}
		graph, err := renderMetricGraph(series, width, query)
		return graphLoadedMsg{graph: graph, err: err}
	}
}

func (m metricsModel) currentHorizon() metricHorizon {
	return metricHorizons[clampCursor(m.horizonCursor, len(metricHorizons))]
}

func (m metricsModel) loadLabels() tea.Cmd {
	metric := m.currentMetric()
	if metric == "" {
		return nil
	}
	return func() tea.Msg {
		labels, err := m.client.LabelNames(context.Background(), metric)
		return labelsLoadedMsg{metric: metric, labels: labels, err: err}
	}
}

func (m metricsModel) loadValues() tea.Cmd {
	label := m.currentLabel()
	otherLabels := make(map[string]string, len(m.selected))
	for key, value := range m.selected {
		if key != label {
			otherLabels[key] = value
		}
	}
	selector := prometheus.BuildSelector(m.currentMetric(), otherLabels)
	return func() tea.Msg {
		values, err := m.client.LabelValues(context.Background(), label, selector)
		return valuesLoadedMsg{label: label, values: values, err: err}
	}
}

func (m metricsModel) validateQuery(query string) tea.Cmd {
	return func() tea.Msg {
		return queryValidatedMsg{err: m.client.ValidateQuery(context.Background(), query)}
	}
}

func (m metricsModel) metricChanged() (tea.Model, tea.Cmd) {
	m.labels = nil
	m.labelValues = nil
	m.labelCursor = 0
	m.valueCursor = 0
	m.selected = make(map[string]string)
	m.loading = m.currentMetric() != ""
	m.err = ""
	m.resetEditor()
	if !m.loading {
		return m, nil
	}
	return m, m.loadLabels()
}

func (m metricsModel) applyMetricFilter() (tea.Model, tea.Cmd) {
	m.filterMetricNames()
	m.metricCursor = 0
	return m.metricChanged()
}

func (m *metricsModel) filterMetricNames() {
	needle := strings.ToLower(m.filter)
	filtered := make([]string, 0, len(m.metrics))
	for _, metric := range m.metrics {
		if strings.Contains(strings.ToLower(metric), needle) {
			filtered = append(filtered, metric)
		}
	}
	m.filtered = filtered
}

func (m metricsModel) metricPageSize() int {
	return max(5, m.height-9)
}

func (m *metricsModel) resetEditor() {
	query := ""
	if metric := m.currentMetric(); metric != "" {
		query = "sum(" + prometheus.BuildSelector(metric, m.selected) + ")"
	}
	m.editor.Set(query)
}

func (m metricsModel) currentMetric() string {
	if m.metricCursor < 0 || m.metricCursor >= len(m.filtered) {
		return ""
	}
	return m.filtered[m.metricCursor]
}

func (m metricsModel) currentLabel() string {
	if m.labelCursor < 0 || m.labelCursor >= len(m.labels) {
		return ""
	}
	return m.labels[m.labelCursor]
}

func (m metricsModel) filteredValues() []string {
	if m.valueFilter == "" {
		return m.labelValues
	}
	needle := strings.ToLower(m.valueFilter)
	values := make([]string, 0, len(m.labelValues))
	for _, value := range m.labelValues {
		if strings.Contains(strings.ToLower(value), needle) {
			values = append(values, value)
		}
	}
	return values
}

func (m metricsModel) View() string {
	if m.done {
		return "\n"
	}
	switch m.stage {
	case browseStage:
		return m.browseView()
	case editStage:
		return m.editView()
	case previewStage:
		return m.previewView()
	case horizonStage:
		return m.horizonView()
	case graphLoadingStage:
		horizon := m.currentHorizon()
		return m.loadingView(fmt.Sprintf("Loading samples from the %s at a %s step…", horizon.label, metricStepLabel(horizon.step)))
	case graphStage:
		return m.graphView()
	case targetsStage:
		return m.targetsView()
	default:
		return ""
	}
}

func (m metricsModel) browseView() string {
	width := max(m.width, 60)
	leftWidth := max(28, width/2-2)
	rightWidth := max(28, width-leftWidth-3)
	listHeight := max(5, m.height-9)

	leftTitle := "Metrics"
	if m.pane == metricsPane {
		leftTitle = metricsActiveStyle.Render(leftTitle)
	}
	filter := m.filter
	if filter == "" {
		filter = "type to filter"
	} else {
		filter = "filter: " + filter + "█"
	}

	var left strings.Builder
	left.WriteString(leftTitle)
	left.WriteString("  ")
	left.WriteString(metricsHintStyle.Render(filter))
	left.WriteString("\n\n")
	left.WriteString(renderList(m.filtered, m.metricCursor, listHeight, leftWidth, m.pane == metricsPane))

	var right strings.Builder
	rightTitle := "Labels"
	if m.pane == labelsPane {
		rightTitle = metricsActiveStyle.Render(rightTitle)
	}
	if m.pane == valuesPane {
		rightTitle = metricsActiveStyle.Render("Values: " + m.currentLabel())
	}
	right.WriteString(rightTitle)
	right.WriteString("\n\n")
	if m.loading {
		right.WriteString(metricsHintStyle.Render("Loading…"))
	} else if m.pane == valuesPane {
		values := m.filteredValues()
		valueFilter := m.valueFilter
		if m.searchMode {
			valueFilter += "█"
		}
		if valueFilter != "" {
			right.WriteString(metricsHintStyle.Render("/ " + valueFilter))
			right.WriteString("\n")
		}
		right.WriteString(renderList(values, m.valueCursor, listHeight-1, rightWidth, true))
	} else if len(m.labels) == 0 {
		right.WriteString(metricsHintStyle.Render("No labels found."))
	} else {
		rows := make([]string, len(m.labels))
		for index, label := range m.labels {
			rows[index] = label
			if value, ok := m.selected[label]; ok {
				rows[index] = "✓ " + label + "=" + value
			}
		}
		right.WriteString(renderList(rows, m.labelCursor, listHeight, rightWidth, m.pane == labelsPane))
	}

	leftPanel := lipgloss.NewStyle().Width(leftWidth).MaxWidth(leftWidth).Render(left.String())
	rightPanel := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238")).
		PaddingLeft(1).
		Width(rightWidth).
		MaxWidth(rightWidth).
		Render(right.String())

	var output strings.Builder
	output.WriteString(metricsTitleStyle.Render("Kedify Metrics Explorer"))
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("Build a selector, then edit and validate the final PromQL expression."))
	output.WriteString("\n\n")
	output.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel))
	output.WriteString("\n\n")
	output.WriteString(metricsQueryStyle.Render(m.editor.String()))
	output.WriteString("\n")
	if m.err != "" {
		output.WriteString(metricsErrorStyle.Render(m.err))
		output.WriteString("\n")
	}
	output.WriteString(metricsHintStyle.Render("↑/↓/PgUp/PgDn move • type to filter • Backspace edit • Esc clear • →/Enter drill down • Ctrl+E edit query"))
	output.WriteString("\n")
	return output.String()
}

func (m metricsModel) editView() string {
	query := m.editor.View()
	var output strings.Builder
	output.WriteString(metricsTitleStyle.Render("Edit PromQL"))
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("The query is checked with Prometheus before you continue."))
	output.WriteString("\n\n")
	output.WriteString(metricsQueryStyle.Render(query))
	output.WriteString("\n\n")
	if m.loading {
		output.WriteString(metricsHintStyle.Render("Validating query…"))
		output.WriteString("\n")
	}
	if m.err != "" {
		output.WriteString(metricsErrorStyle.Render(m.err))
		output.WriteString("\n")
	}
	output.WriteString(metricsHintStyle.Render("Enter validate • ←/→ move cursor • Esc return to metrics • Ctrl+C cancel"))
	output.WriteString("\n")
	return output.String()
}

func (m metricsModel) previewView() string {
	options := []string{"Visualize metric", "Continue to resource YAML"}
	var output strings.Builder
	output.WriteString(metricsTitleStyle.Render("PromQL is valid"))
	output.WriteString("\n\n")
	output.WriteString(metricsQueryStyle.Render(m.editor.String()))
	output.WriteString("\n\n")
	for index, option := range options {
		prefix := "  "
		style := lipgloss.NewStyle()
		if index == m.previewCursor {
			prefix = "› "
			style = metricsSelectedStyle
		}
		output.WriteString(style.Render(prefix + option))
		output.WriteString("\n")
	}
	if m.err != "" {
		output.WriteString("\n")
		output.WriteString(metricsErrorStyle.Render(m.err))
		output.WriteString("\n")
	}
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("↑/↓ choose • Enter continue • Ctrl+E edit"))
	output.WriteString("\n")
	return output.String()
}

func (m metricsModel) horizonView() string {
	var output strings.Builder
	output.WriteString(metricsTitleStyle.Render("Select a time horizon"))
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("The query step is adjusted to keep the terminal graph responsive."))
	output.WriteString("\n\n")
	for index, horizon := range metricHorizons {
		line := fmt.Sprintf("%s (%s step)", horizon.label, metricStepLabel(horizon.step))
		if index == m.horizonCursor {
			output.WriteString(metricsSelectedStyle.Render("› " + line))
		} else {
			output.WriteString("  " + line)
		}
		output.WriteString("\n")
	}
	if m.err != "" {
		output.WriteString("\n")
		output.WriteString(metricsErrorStyle.Render(m.err))
		output.WriteString("\n")
	}
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("↑/↓ choose • Enter visualize • Esc return • Ctrl+E edit query"))
	output.WriteString("\n")
	return output.String()
}

func (m metricsModel) loadingView(message string) string {
	return metricsTitleStyle.Render("Kedify Metrics Explorer") + "\n\n" +
		metricsHintStyle.Render(message) + "\n" +
		metricsHintStyle.Render("Esc to return.") + "\n"
}

func (m metricsModel) graphView() string {
	var output strings.Builder
	output.WriteString(metricsTitleStyle.Render("Metric preview — " + m.currentHorizon().label))
	output.WriteString("\n\n")
	output.WriteString(m.graph)
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("Enter continue • Ctrl+E edit query • Esc return"))
	output.WriteString("\n")
	return output.String()
}

func (m metricsModel) targetsView() string {
	options := []string{
		"generate ScaledObject YAML",
		"generate MetricPredictor YAML",
		"create selected resources in the cluster",
	}
	var output strings.Builder
	output.WriteString(metricsTitleStyle.Render("Generate Kubernetes resources"))
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("YAML is printed to stdout. Cluster creation is opt-in and disabled by default."))
	output.WriteString("\n\n")
	for index, option := range options {
		check := "[ ]"
		if m.targets[index] {
			check = "[x]"
		}
		line := check + " " + option
		if index == m.targetCursor {
			line = "› " + line
			output.WriteString(metricsSelectedStyle.Render(line))
		} else {
			output.WriteString("  " + line)
		}
		output.WriteString("\n")
	}
	if m.err != "" {
		output.WriteString("\n")
		output.WriteString(metricsErrorStyle.Render(m.err))
		output.WriteString("\n")
	}
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("↑/↓ move • Space toggle • Enter finish • Ctrl+E edit query"))
	output.WriteString("\n")
	return output.String()
}

func (e *lineEditor) Set(value string) {
	e.value = []rune(value)
	e.cursor = len(e.value)
}

func (e *lineEditor) String() string {
	return string(e.value)
}

func (e *lineEditor) Update(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyLeft:
		if e.cursor > 0 {
			e.cursor--
		}
	case tea.KeyRight:
		if e.cursor < len(e.value) {
			e.cursor++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		e.cursor = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		e.cursor = len(e.value)
	case tea.KeyBackspace:
		if e.cursor > 0 {
			e.value = append(e.value[:e.cursor-1], e.value[e.cursor:]...)
			e.cursor--
		}
	case tea.KeyDelete:
		if e.cursor < len(e.value) {
			e.value = append(e.value[:e.cursor], e.value[e.cursor+1:]...)
		}
	default:
		if msg.Type == tea.KeyRunes {
			e.value = insertRunes(e.value, e.cursor, msg.Runes)
			e.cursor += len(msg.Runes)
		}
	}
}

func (e *lineEditor) View() string {
	before := string(e.value[:e.cursor])
	after := string(e.value[e.cursor:])
	cursor := " "
	if after != "" {
		r, size := utf8.DecodeRuneInString(after)
		cursor = string(r)
		after = after[size:]
	}
	return before + promptCursorStyle.Render(cursor) + after
}

func renderList(items []string, cursor, height, width int, active bool) string {
	if len(items) == 0 {
		return metricsHintStyle.Render("No matches.")
	}
	height = max(height, 1)
	start := max(0, cursor-height/2)
	if start+height > len(items) {
		start = max(0, len(items)-height)
	}
	end := min(len(items), start+height)

	var output strings.Builder
	for index := start; index < end; index++ {
		prefix := "  "
		style := lipgloss.NewStyle()
		if index == cursor {
			prefix = "› "
			if active {
				style = metricsSelectedStyle
			} else {
				style = metricsActiveStyle
			}
		}
		line := truncateText(prefix+items[index], width-1)
		output.WriteString(style.Render(line))
		if index < end-1 {
			output.WriteString("\n")
		}
	}
	return output.String()
}

func renderMetricGraph(series []prometheus.Series, terminalWidth int, query string) (string, error) {
	if len(series) == 0 {
		return "", errors.New("the query returned no samples for the selected time horizon")
	}

	const (
		maxSeries   = 4
		bytesPerMiB = 1024 * 1024
	)
	queryUsesMemoryMetric := queryContainsMemoryMetric(query)
	var output strings.Builder
	limit := min(len(series), maxSeries)
	for index := 0; index < limit; index++ {
		memoryValues := queryUsesMemoryMetric ||
			strings.Contains(strings.ToLower(series[index].Labels["__name__"]), "memory")
		values := make([]float64, 0, len(series[index].Points))
		for _, point := range series[index].Points {
			if !math.IsNaN(point.Value) && !math.IsInf(point.Value, 0) {
				value := point.Value
				if memoryValues {
					value /= bytesPerMiB
				}
				values = append(values, value)
			}
		}
		if len(values) == 0 {
			continue
		}
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		caption := seriesCaption(series[index].Labels)
		if memoryValues {
			caption += " (MiB)"
		}
		output.WriteString(caption)
		output.WriteString("\n")
		graphWidth := min(max(terminalWidth-14, 30), 120)
		output.WriteString(asciigraph.Plot(
			values,
			asciigraph.Height(10),
			asciigraph.Width(graphWidth),
			asciigraph.SeriesColors(asciigraph.Cyan),
		))
		output.WriteString("\n")
	}
	if output.Len() == 0 {
		return "", errors.New("the query returned no numeric samples for the selected time horizon")
	}
	if len(series) > maxSeries {
		output.WriteString(metricsHintStyle.Render(fmt.Sprintf("Showing %d of %d series.", maxSeries, len(series))))
		output.WriteString("\n")
	}
	return output.String(), nil
}

func queryContainsMemoryMetric(query string) bool {
	query = stripPromQLStringLiterals(query)
	query = promQLLabelListPattern.ReplaceAllStringFunc(query, func(value string) string {
		return strings.Repeat(" ", len(value))
	})
	for _, match := range promQLIdentifierPattern.FindAllStringIndex(query, -1) {
		identifier := strings.ToLower(query[match[0]:match[1]])
		if !strings.Contains(identifier, "memory") {
			continue
		}

		remainder := strings.TrimLeft(query[match[1]:], " \t\r\n")
		if strings.HasPrefix(remainder, "(") ||
			strings.HasPrefix(remainder, "=") ||
			strings.HasPrefix(remainder, "!=") ||
			strings.HasPrefix(remainder, "=~") ||
			strings.HasPrefix(remainder, "!~") {
			continue
		}
		return true
	}
	return false
}

func stripPromQLStringLiterals(query string) string {
	runes := []rune(query)
	var quote rune
	escaped := false
	for index, current := range runes {
		if quote == 0 {
			if current == '"' || current == '\'' || current == '`' {
				quote = current
				runes[index] = ' '
			}
			continue
		}

		runes[index] = ' '
		if escaped {
			escaped = false
			continue
		}
		if current == '\\' && quote != '`' {
			escaped = true
			continue
		}
		if current == quote {
			quote = 0
		}
	}
	return string(runes)
}

func seriesCaption(labels map[string]string) string {
	name := labels["__name__"]
	keys := make([]string, 0, len(labels))
	for key := range labels {
		if key != "__name__" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	if len(parts) == 0 {
		if name == "" {
			return "<series>"
		}
		return name
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

func truncateText(value string, width int) string {
	if width < 2 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
}

func removeLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func clampCursor(cursor, length int) int {
	if length == 0 {
		return 0
	}
	return min(max(cursor, 0), length-1)
}

func metricStepLabel(step time.Duration) string {
	if step%time.Hour == 0 {
		return fmt.Sprintf("%dh", step/time.Hour)
	}
	if step%time.Minute == 0 {
		return fmt.Sprintf("%dm", step/time.Minute)
	}
	return step.String()
}

func metricHorizonIndex(argument string) (int, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		return 0, nil
	}
	for index, horizon := range metricHorizons {
		if horizon.argument == argument {
			return index, nil
		}
	}
	return 0, fmt.Errorf("unsupported metric horizon %q", argument)
}
