package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func Write(w io.Writer, value any, format string) error {
	var (
		data []byte
		err  error
	)

	switch format {
	case "text":
		data, err = renderText(value)
	case "json":
		data, err = json.MarshalIndent(value, "", "  ")
	case "yaml":
		data, err = yaml.Marshal(value)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
	if err != nil {
		return fmt.Errorf("encode %s output: %w", format, err)
	}

	if !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}

	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func renderText(value any) ([]byte, error) {
	switch v := value.(type) {
	case []any:
		// An empty list is ambiguous (could be clusters, recommendations, or something else),
		// so fall back to YAML to avoid misleading "No clusters found" output.
		if len(v) == 0 {
			return yaml.Marshal(v)
		}
		if clusters, ok := asClusterList(v); ok {
			if looksLikeClusterList(clusters) {
				return renderClusterListText(clusters), nil
			}
			if looksLikeRecommendationList(clusters) {
				return renderRecommendationListText(clusters), nil
			}
		}
		return yaml.Marshal(v)
	case []map[string]any:
		if looksLikeClusterList(v) {
			return renderClusterListText(v), nil
		}
		if looksLikeRecommendationList(v) {
			return renderRecommendationListText(v), nil
		}
		return yaml.Marshal(v)
	case map[string]any:
		if !looksLikeCluster(v) {
			return yaml.Marshal(v)
		}
		return renderClusterText(v), nil
	default:
		return nil, fmt.Errorf("text output is not supported for %T", value)
	}
}

func asClusterList(items []any) ([]map[string]any, bool) {
	clusters := make([]map[string]any, 0, len(items))
	for _, item := range items {
		cluster, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		clusters = append(clusters, cluster)
	}

	return clusters, true
}

func renderClusterListText(clusters []map[string]any) []byte {
	if len(clusters) == 0 {
		return []byte("No clusters found.\n")
	}

	rows := make([][]string, 0, len(clusters)+1)
	rows = append(rows, []string{"NAME", "ID", "AGENT VERSION", "KEDA VERSION", "AGENT STATUS", "KEDA STATUS", "AGE"})

	for _, cluster := range clusters {
		rows = append(rows, []string{
			fallbackClusterValue(cluster, "name", "<unnamed cluster>"),
			clusterTextValue(cluster, "id"),
			clusterAgentVersion(cluster),
			clusterKEDAVersion(cluster),
			clusterTextValue(cluster, "agentStatus"),
			clusterTextValue(cluster, "kedaStatus"),
			clusterAge(cluster),
		})
	}

	return []byte(renderTextTable(rows))
}

func renderClusterText(cluster map[string]any) []byte {
	rows := [][]string{
		{"NAME", "ID", "AGENT VERSION", "KEDA VERSION", "AGENT STATUS", "KEDA STATUS", "AGE"},
		{
			fallbackClusterValue(cluster, "name", "<unnamed cluster>"),
			clusterTextValue(cluster, "id"),
			clusterAgentVersion(cluster),
			clusterKEDAVersion(cluster),
			clusterTextValue(cluster, "agentStatus"),
			clusterTextValue(cluster, "kedaStatus"),
			clusterAge(cluster),
		},
	}

	return []byte(renderTextTable(rows))
}

func renderRecommendationListText(recommendations []map[string]any) []byte {
	if len(recommendations) == 0 {
		return []byte("No recommendations found.\n")
	}

	grouped := groupRecommendations(recommendations)
	rows := make([][]string, 0, len(grouped)+1)
	rows = append(rows, []string{"KIND", "NAME", "NAMESPACE", "CPU REQUESTS", "CPU LIMITS", "MEMORY REQUESTS", "MEMORY LIMITS"})

	for _, recommendation := range grouped {
		rows = append(rows, []string{
			recommendation.kind,
			recommendation.name,
			recommendation.namespace,
			recommendation.cpuRequests,
			recommendation.cpuLimits,
			recommendation.memoryRequests,
			recommendation.memoryLimits,
		})
	}

	return []byte(renderTextTable(rows))
}

func clusterTextValue(cluster map[string]any, key string) string {
	value, ok := cluster[key]
	if !ok {
		return ""
	}

	return textValue(value)
}

func fallbackClusterValue(cluster map[string]any, key, fallback string) string {
	if value := clusterTextValue(cluster, key); value != "" {
		return value
	}
	return fallback
}

func renderTextTable(rows [][]string) string {
	if len(rows) == 0 {
		return "\n"
	}

	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		padded := make([]string, len(row))
		for i, cell := range row {
			padded[i] = padRight(cell, widths[i])
		}
		lines = append(lines, strings.TrimRight(strings.Join(padded, "  "), " "))
	}

	return strings.Join(lines, "\n") + "\n"
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func clusterAge(cluster map[string]any) string {
	createdAt := clusterTextValue(cluster, "createdAt")
	if createdAt == "" {
		return ""
	}

	created, err := time.Parse("2006-01-02", createdAt)
	if err != nil {
		return createdAt
	}

	return humanAge(time.Since(created))
}

func clusterAgentVersion(cluster map[string]any) string {
	agent, ok := cluster["agent"].(map[string]any)
	if !ok {
		return ""
	}
	version, _ := agent["version"].(string)
	return version
}

func clusterKEDAVersion(cluster map[string]any) string {
	agent, ok := cluster["agent"].(map[string]any)
	if !ok {
		return ""
	}

	kedaConfigs, ok := agent["kedaConfigs"].([]any)
	if !ok || len(kedaConfigs) == 0 {
		return ""
	}

	firstConfig, ok := kedaConfigs[0].(map[string]any)
	if !ok {
		return ""
	}

	version, _ := firstConfig["kedaVersion"].(string)
	return version
}

func humanAge(d time.Duration) string {
	if d < time.Minute {
		return "0m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d < 30*24*time.Hour {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if d < 365*24*time.Hour {
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
	return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
}

func textValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func looksLikeClusterList(items []map[string]any) bool {
	if len(items) == 0 {
		return true
	}

	for _, item := range items {
		if !looksLikeCluster(item) {
			return false
		}
	}

	return true
}

func looksLikeRecommendationList(items []map[string]any) bool {
	if len(items) == 0 {
		return true
	}

	for _, item := range items {
		if !looksLikeRecommendation(item) {
			return false
		}
	}

	return true
}

func looksLikeCluster(value map[string]any) bool {
	if _, ok := value["agentStatus"]; ok {
		return true
	}
	if _, ok := value["kedaStatus"]; ok {
		return true
	}
	if _, ok := value["createdAt"]; ok {
		return true
	}

	agent, ok := value["agent"].(map[string]any)
	if !ok {
		return false
	}

	if _, ok := agent["version"]; ok {
		return true
	}
	if _, ok := agent["kedaConfigs"]; ok {
		return true
	}

	return false
}

func looksLikeRecommendation(value map[string]any) bool {
	_, hasName := value["name"]
	_, hasNamespace := value["namespace"]
	_, hasResourceUID := value["resourceUID"]
	labels, _ := value["labels"].(map[string]any)
	_, hasOriginalValue := labels["originalValue"]
	_, hasSuggestedValue := labels["suggestedValue"]

	return hasResourceUID || hasOriginalValue || hasSuggestedValue || hasName || hasNamespace
}

func recommendationLabelText(recommendation map[string]any, key string) string {
	labels, ok := recommendation["labels"].(map[string]any)
	if !ok {
		return ""
	}

	return textValue(labels[key])
}

type recommendationRow struct {
	kind           string
	name           string
	namespace      string
	cpuRequests    string
	cpuLimits      string
	memoryRequests string
	memoryLimits   string
}

func groupRecommendations(recommendations []map[string]any) []recommendationRow {
	rowsByKey := make(map[string]*recommendationRow, len(recommendations))
	order := make([]string, 0, len(recommendations))

	for _, recommendation := range recommendations {
		key := recommendationGroupKey(recommendation)
		row, ok := rowsByKey[key]
		if !ok {
			row = &recommendationRow{
				kind:      textValue(recommendation["kind"]),
				name:      textValue(recommendation["name"]),
				namespace: textValue(recommendation["namespace"]),
			}
			rowsByKey[key] = row
			order = append(order, key)
		}

		switch recommendationResourceTarget(recommendation) {
		case "cpu-requests":
			row.cpuRequests = recommendationValueChange(recommendation)
		case "cpu-limits":
			row.cpuLimits = recommendationValueChange(recommendation)
		case "memory-requests":
			row.memoryRequests = recommendationValueChange(recommendation)
		case "memory-limits":
			row.memoryLimits = recommendationValueChange(recommendation)
		}
	}

	rows := make([]recommendationRow, 0, len(order))
	for _, key := range order {
		rows = append(rows, *rowsByKey[key])
	}

	return rows
}

func recommendationGroupKey(recommendation map[string]any) string {
	return strings.Join([]string{
		textValue(recommendation["kind"]),
		textValue(recommendation["name"]),
		textValue(recommendation["namespace"]),
	}, "\x00")
}

func recommendationResourceTarget(recommendation map[string]any) string {
	resourceUID := textValue(recommendation["resourceUID"])
	if resourceUID == "" {
		return ""
	}

	parts := strings.Split(resourceUID, "/")
	if len(parts) == 0 {
		return ""
	}

	last := parts[len(parts)-1]
	if slices.Contains([]string{"cpu-requests", "cpu-limits", "memory-requests", "memory-limits"}, last) {
		return last
	}

	return ""
}

func recommendationValueChange(recommendation map[string]any) string {
	originalValue := recommendationLabelText(recommendation, "originalValue")
	suggestedValue := recommendationLabelText(recommendation, "suggestedValue")

	switch {
	case originalValue == "" && suggestedValue == "":
		return ""
	case originalValue == "":
		return suggestedValue
	case suggestedValue == "":
		return originalValue
	default:
		return originalValue + " -> " + suggestedValue
	}
}
