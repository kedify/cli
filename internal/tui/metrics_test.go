package tui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/guptarohit/asciigraph"

	"github.com/kedify/cli/internal/prometheus"
)

type fakeMetricsClient struct {
	labels       []string
	values       []string
	validateErr  error
	rangeSeries  []prometheus.Series
	rangeErr     error
	lastSelector string
	lastLabel    string
	lastQuery    string
	rangeStart   time.Time
	rangeEnd     time.Time
	rangeStep    time.Duration
}

func (f *fakeMetricsClient) LabelNames(_ context.Context, selector string) ([]string, error) {
	f.lastSelector = selector
	return f.labels, nil
}

func (f *fakeMetricsClient) LabelValues(_ context.Context, label, selector string) ([]string, error) {
	f.lastLabel = label
	f.lastSelector = selector
	return f.values, nil
}

func (f *fakeMetricsClient) ValidateQuery(_ context.Context, query string) error {
	f.lastQuery = query
	return f.validateErr
}

func (f *fakeMetricsClient) RangeQuery(
	_ context.Context,
	query string,
	start, end time.Time,
	step time.Duration,
) ([]prometheus.Series, error) {
	f.lastQuery = query
	f.rangeStart = start
	f.rangeEnd = end
	f.rangeStep = step
	return f.rangeSeries, f.rangeErr
}

func TestMetricsModelDrillsIntoLabelsAndBuildsQuery(t *testing.T) {
	client := &fakeMetricsClient{
		labels: []string{"method", "zone"},
		values: []string{"GET", "POST"},
	}
	model := newMetricsModel(client, []string{"up", "http_requests_total"})

	initMsg := model.Init()()
	loaded := initMsg.(labelsLoadedMsg)
	if loaded.metric != "http_requests_total" {
		t.Fatalf("initial metric = %q", loaded.metric)
	}
	updated, _ := model.Update(initMsg)
	model = updated.(metricsModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(metricsModel)
	if model.pane != labelsPane {
		t.Fatalf("pane = %d, want labelsPane", model.pane)
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(metricsModel)
	if model.pane != valuesPane || cmd == nil {
		t.Fatalf("value drill-down pane = %d, cmd = %v", model.pane, cmd)
	}
	valuesMsg := cmd()
	updated, _ = model.Update(valuesMsg)
	model = updated.(metricsModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(metricsModel)
	if model.selected["method"] != "GET" {
		t.Fatalf("selected labels = %#v", model.selected)
	}
	if got := model.editor.String(); got != `sum(http_requests_total{method="GET"})` {
		t.Fatalf("query = %q", got)
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(metricsModel)
	_ = cmd()
	if client.lastSelector != "http_requests_total" {
		t.Fatalf("selector for changing method = %q, want current label omitted", client.lastSelector)
	}
}

func TestMetricsModelValidatesThenChoosesTargets(t *testing.T) {
	client := &fakeMetricsClient{}
	model := newMetricsModel(client, []string{"up"})
	model.stage = editStage
	model.loading = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(metricsModel)
	if cmd == nil || !model.loading {
		t.Fatalf("validation cmd = %v, loading = %v", cmd, model.loading)
	}
	queryWhileValidating := model.editor.String()
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = updated.(metricsModel)
	if model.editor.String() != queryWhileValidating {
		t.Fatal("query changed while validation was in flight")
	}
	updated, _ = model.Update(cmd())
	model = updated.(metricsModel)
	if model.stage != previewStage {
		t.Fatalf("stage = %d, want previewStage", model.stage)
	}

	model.previewCursor = 1
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(metricsModel)
	if model.stage != targetsStage {
		t.Fatalf("stage = %d, want targetsStage", model.stage)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(metricsModel)
	if !model.targets[0] {
		t.Fatal("ScaledObject target was not selected")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(metricsModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(metricsModel)
	if !model.targets[1] {
		t.Fatal("MetricPredictor target was not selected")
	}
	if model.targets[2] {
		t.Fatal("resource creation must be disabled by default")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(metricsModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(metricsModel)
	if !model.targets[2] {
		t.Fatal("cluster creation target was not selected")
	}
}

func TestMetricsModelRequiresManifestForClusterCreation(t *testing.T) {
	model := newMetricsModel(&fakeMetricsClient{}, []string{"up"})
	model.stage = targetsStage
	model.loading = false
	model.targetCursor = 2

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(metricsModel)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(metricsModel)
	if cmd != nil || model.done {
		t.Fatal("model finished with creation selected but no resource manifest")
	}
	if !strings.Contains(model.err, "Select at least one") {
		t.Fatalf("error = %q", model.err)
	}
}

func TestMetricsModelKeepsValidationErrorsInEditor(t *testing.T) {
	client := &fakeMetricsClient{validateErr: errors.New("parse error")}
	model := newMetricsModel(client, []string{"up"})
	model.stage = editStage
	model.loading = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(metricsModel)
	updated, _ = model.Update(cmd())
	model = updated.(metricsModel)
	if model.stage != editStage || model.err != "parse error" {
		t.Fatalf("stage = %d, error = %q", model.stage, model.err)
	}
}

func TestMetricsModelFiltersByTypedSubstringAndUsesCtrlEForEditing(t *testing.T) {
	model := newMetricsModel(&fakeMetricsClient{}, []string{
		"errors_total",
		"http_requests_total",
		"node_errors_total",
		"up",
	})
	model.loading = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	model = updated.(metricsModel)
	if model.filter != "E" {
		t.Fatalf("filter = %q, want E", model.filter)
	}
	want := []string{"errors_total", "http_requests_total", "node_errors_total"}
	if !reflect.DeepEqual(model.filtered, want) {
		t.Fatalf("filtered metrics = %#v, want %#v", model.filtered, want)
	}
	if model.stage != browseStage {
		t.Fatalf("typing E changed stage to %d", model.stage)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model = updated.(metricsModel)
	if model.stage != editStage {
		t.Fatalf("Ctrl+E stage = %d, want editStage", model.stage)
	}
}

func TestMetricsModelSeedsAnEditableFilter(t *testing.T) {
	model, err := newMetricsModelWithOptions(
		&fakeMetricsClient{},
		[]string{"cpu_usage", "memory", "memory_usage"},
		MetricsExplorerOptions{Filter: "memory_"},
	)
	if err != nil {
		t.Fatalf("newMetricsModelWithOptions() error = %v", err)
	}
	if model.filter != "memory_" {
		t.Fatalf("filter = %q, want memory_", model.filter)
	}
	if want := []string{"memory_usage"}; !reflect.DeepEqual(model.filtered, want) {
		t.Fatalf("filtered metrics = %#v, want %#v", model.filtered, want)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(metricsModel)
	if model.filter != "memory" {
		t.Fatalf("filter after Backspace = %q, want memory", model.filter)
	}
	if want := []string{"memory", "memory_usage"}; !reflect.DeepEqual(model.filtered, want) {
		t.Fatalf("filtered metrics after Backspace = %#v, want %#v", model.filtered, want)
	}
}

func TestMetricsModelStartsAtRequestedQueryStage(t *testing.T) {
	tests := []struct {
		name      string
		options   MetricsExplorerOptions
		wantStage int
		wantGraph bool
	}{
		{
			name:      "query asks whether to visualize",
			options:   MetricsExplorerOptions{Query: "sum(foobar)"},
			wantStage: previewStage,
		},
		{
			name:      "visualize asks for horizon",
			options:   MetricsExplorerOptions{Query: "sum(foobar)", Visualize: true},
			wantStage: horizonStage,
		},
		{
			name: "horizon loads graph",
			options: MetricsExplorerOptions{
				Query:     "sum(foobar)",
				Visualize: true,
				Horizon:   "3d",
			},
			wantStage: graphLoadingStage,
			wantGraph: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeMetricsClient{rangeSeries: []prometheus.Series{{
				Points: []prometheus.Point{{Value: 1}},
			}}}
			model, err := newMetricsModelWithOptions(client, nil, test.options)
			if err != nil {
				t.Fatalf("newMetricsModelWithOptions() error = %v", err)
			}
			if model.stage != editStage || !model.loading {
				t.Fatalf("initial stage = %d, loading = %v", model.stage, model.loading)
			}

			validateCmd := model.Init()
			if validateCmd == nil {
				t.Fatal("initial query validation command is nil")
			}
			updated, graphCmd := model.Update(validateCmd())
			model = updated.(metricsModel)
			if model.stage != test.wantStage {
				t.Fatalf("stage after validation = %d, want %d", model.stage, test.wantStage)
			}
			if client.lastQuery != "sum(foobar)" {
				t.Fatalf("validated query = %q, want sum(foobar)", client.lastQuery)
			}
			if (graphCmd != nil) != test.wantGraph {
				t.Fatalf("graph command present = %v, want %v", graphCmd != nil, test.wantGraph)
			}
			if !test.wantGraph {
				return
			}

			updated, _ = model.Update(graphCmd())
			model = updated.(metricsModel)
			if model.stage != graphStage {
				t.Fatalf("stage after graph load = %d, want graphStage", model.stage)
			}
			if got := client.rangeEnd.Sub(client.rangeStart); got != 3*24*time.Hour {
				t.Fatalf("query duration = %s, want 72h", got)
			}
			if client.rangeStep != 15*time.Minute {
				t.Fatalf("query step = %s, want 15m", client.rangeStep)
			}
		})
	}
}

func TestMetricsModelRejectsUnsupportedHorizon(t *testing.T) {
	_, err := newMetricsModelWithOptions(
		&fakeMetricsClient{},
		nil,
		MetricsExplorerOptions{Query: "up", Visualize: true, Horizon: "2d"},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported metric horizon") {
		t.Fatalf("newMetricsModelWithOptions() error = %v", err)
	}
}

func TestMetricsModelPageUpAndPageDownMoveAWholePage(t *testing.T) {
	metrics := make([]string, 40)
	for index := range metrics {
		metrics[index] = fmt.Sprintf("metric_%02d", index)
	}
	model := newMetricsModel(&fakeMetricsClient{}, metrics)
	model.height = 20
	model.loading = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(metricsModel)
	if model.metricCursor != 11 {
		t.Fatalf("Page Down cursor = %d, want 11", model.metricCursor)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(metricsModel)
	if model.metricCursor != 22 {
		t.Fatalf("second Page Down cursor = %d, want 22", model.metricCursor)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(metricsModel)
	if model.metricCursor != 11 {
		t.Fatalf("Page Up cursor = %d, want 11", model.metricCursor)
	}
}

func TestMetricsModelQueriesSelectedTimeHorizon(t *testing.T) {
	tests := []struct {
		name     string
		index    int
		duration time.Duration
		step     time.Duration
	}{
		{name: "last 6 hours", index: 0, duration: 6 * time.Hour, step: time.Minute},
		{name: "last day", index: 1, duration: 24 * time.Hour, step: 5 * time.Minute},
		{name: "last 3 days", index: 2, duration: 3 * 24 * time.Hour, step: 15 * time.Minute},
		{name: "last week", index: 3, duration: 7 * 24 * time.Hour, step: 30 * time.Minute},
		{name: "last month", index: 4, duration: 30 * 24 * time.Hour, step: 2 * time.Hour},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeMetricsClient{rangeSeries: []prometheus.Series{{
				Points: []prometheus.Point{{Value: 1}},
			}}}
			model := newMetricsModel(client, []string{"up"})
			model.stage = horizonStage
			model.horizonCursor = test.index

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(metricsModel)
			if cmd == nil || model.stage != graphLoadingStage {
				t.Fatalf("graph command = %v, stage = %d", cmd, model.stage)
			}
			updated, _ = model.Update(cmd())
			model = updated.(metricsModel)

			if got := client.rangeEnd.Sub(client.rangeStart); got != test.duration {
				t.Fatalf("query duration = %s, want %s", got, test.duration)
			}
			if client.rangeStep != test.step {
				t.Fatalf("query step = %s, want %s", client.rangeStep, test.step)
			}
			if model.stage != graphStage {
				t.Fatalf("stage = %d, want graphStage", model.stage)
			}
			if !strings.Contains(model.graphView(), test.name) {
				t.Fatalf("graph title does not contain %q:\n%s", test.name, model.graphView())
			}
		})
	}
}

func TestMetricsModelSelectsTimeHorizonBeforeLoadingGraph(t *testing.T) {
	model := newMetricsModel(&fakeMetricsClient{}, []string{"up"})
	model.stage = previewStage

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(metricsModel)
	if cmd != nil || model.stage != horizonStage {
		t.Fatalf("graph choice command = %v, stage = %d", cmd, model.stage)
	}

	for index, horizon := range metricHorizons {
		if !strings.Contains(model.horizonView(), horizon.label) {
			t.Fatalf("horizon %d (%q) is missing:\n%s", index, horizon.label, model.horizonView())
		}
	}
}

func TestRenderMetricGraph(t *testing.T) {
	graph, err := renderMetricGraph([]prometheus.Series{{
		Labels: map[string]string{"__name__": "up", "job": "prometheus"},
		Points: []prometheus.Point{
			{Value: 1},
			{Value: 2},
			{Value: 1.5},
		},
	}}, 80, "up")
	if err != nil {
		t.Fatalf("renderMetricGraph() error = %v", err)
	}
	if !strings.Contains(graph, "up{job=prometheus}") {
		t.Fatalf("graph caption missing:\n%s", graph)
	}
	if !strings.Contains(graph, asciigraph.Cyan.String()) {
		t.Fatalf("graph does not use cyan for the series:\n%q", graph)
	}
}

func TestRenderMetricGraphConvertsMemoryBytesToMiB(t *testing.T) {
	const mib = 1024 * 1024
	graph, err := renderMetricGraph([]prometheus.Series{{
		Labels: map[string]string{"job": "kubelet"},
		Points: []prometheus.Point{
			{Value: mib},
			{Value: 2 * mib},
			{Value: 1.5 * mib},
		},
	}}, 80, "sum(container_memory_usage_bytes)")
	if err != nil {
		t.Fatalf("renderMetricGraph() error = %v", err)
	}
	if !strings.Contains(graph, "(MiB)") {
		t.Fatalf("memory unit is missing:\n%s", graph)
	}
	if !strings.Contains(graph, "1.00") {
		t.Fatalf("converted MiB value is missing:\n%s", graph)
	}
	if strings.Contains(graph, "2097152") {
		t.Fatalf("graph still contains unconverted byte values:\n%s", graph)
	}
}

func TestQueryContainsMemoryMetricIgnoresLabelNamesAndValues(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{query: "sum(container_memory_usage_bytes)", want: true},
		{query: "node_Memory_MemAvailable_bytes", want: true},
		{query: `up{job="memory"}`, want: false},
		{query: `up{memory_type="working_set"}`, want: false},
		{query: `sum by (memory_pool) (up)`, want: false},
	}
	for _, test := range tests {
		if got := queryContainsMemoryMetric(test.query); got != test.want {
			t.Fatalf("queryContainsMemoryMetric(%q) = %v, want %v", test.query, got, test.want)
		}
	}
}

func TestTextEditorSupportsCursorInsertion(t *testing.T) {
	var editor lineEditor
	editor.Set("ac")
	editor.Update(tea.KeyMsg{Type: tea.KeyLeft})
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if got := editor.String(); got != "abc" {
		t.Fatalf("editor = %q, want abc", got)
	}
}
