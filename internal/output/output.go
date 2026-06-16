package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

	_, err = w.Write(data)
	return err
}

func renderText(value any) ([]byte, error) {
	switch v := value.(type) {
	case []map[string]any:
		return renderClusterListText(v), nil
	case map[string]any:
		return renderClusterText(v), nil
	default:
		return nil, fmt.Errorf("text output is not supported for %T", value)
	}
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

func clusterTextValue(cluster map[string]any, key string) string {
	value, ok := cluster[key]
	if !ok {
		return ""
	}

	text, ok := value.(string)
	if !ok {
		return ""
	}

	return text
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
