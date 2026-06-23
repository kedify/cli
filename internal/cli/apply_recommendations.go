package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

var supportedRecommendationResources = []string{
	"cpu-requests",
	"cpu-limits",
	"memory-requests",
	"memory-limits",
}

type ApplyRecommendationsCmd struct {
	Target              string `arg:"" name:"target" help:"Target workload in kind/name form, for example deployment/my-app."`
	Namespace           string `name:"namespace" help:"Kubernetes namespace." required:""`
	Container           string `name:"container" help:"Container name." required:""`
	Resources           string `name:"resources" help:"Comma-separated resources to apply."`
	RecommendationsFile string `name:"recommendations-file" help:"Recommendations JSON or YAML file." required:""`
	ChartPath           string `name:"chart-path" help:"Path to the Helm chart." required:""`
	ValuesFile          string `name:"values-file" help:"Path to the Helm values file." required:""`
	Format              string `name:"format" help:"Output format." enum:"json,diff,override" default:"json"`
	OutputFile          string `name:"output-file" help:"Path to write override output."`
	MinConfidence       int    `name:"min-confidence" help:"Minimum inclusive confidence threshold." default:"60"`
	DryRun              bool   `name:"dry-run" help:"Compute output without writing files."`
}

type recommendationEntry struct {
	Kind        string         `json:"kind" yaml:"kind"`
	Name        string         `json:"name" yaml:"name"`
	Namespace   string         `json:"namespace" yaml:"namespace"`
	ResourceUID string         `json:"resourceUID" yaml:"resourceUID"`
	Status      string         `json:"status" yaml:"status"`
	Labels      map[string]any `json:"labels" yaml:"labels"`
	Raw         map[string]any `json:"-" yaml:"-"`
}

type applyRecommendationsResult struct {
	Result       string                      `json:"result"`
	Target       string                      `json:"target"`
	Namespace    string                      `json:"namespace"`
	Container    string                      `json:"container"`
	Format       string                      `json:"format"`
	DryRun       bool                        `json:"dryRun"`
	Matched      []applyRecommendationMatch  `json:"matched,omitempty"`
	Patched      []applyPatchedResource      `json:"patched,omitempty"`
	Reasons      []applyRecommendationReason `json:"reasons,omitempty"`
	ChangedFiles []string                    `json:"changedFiles,omitempty"`
	OutputFile   string                      `json:"outputFile,omitempty"`
}

type applyRecommendationMatch struct {
	Resource       string `json:"resource"`
	Status         string `json:"status"`
	Confidence     int    `json:"confidence"`
	OriginalValue  string `json:"originalValue"`
	SuggestedValue string `json:"suggestedValue"`
}

type applyPatchedResource struct {
	Resource       string `json:"resource"`
	Path           string `json:"path"`
	OriginalValue  string `json:"originalValue"`
	SuggestedValue string `json:"suggestedValue"`
}

type applyRecommendationReason struct {
	Code     string `json:"code"`
	Resource string `json:"resource,omitempty"`
	Message  string `json:"message"`
}

type renderedDeployment struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []struct {
					Name string `yaml:"name"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

type valuesCandidate struct {
	Path []string
	Node *yaml.Node
}

func (c *ApplyRecommendationsCmd) Run(ctx *context) error {
	if c.Format == "override" && !c.DryRun && strings.TrimSpace(c.OutputFile) == "" {
		return errors.New("--output-file is required when --format override is used without --dry-run")
	}

	kind, name, err := parseRecommendationTarget(c.Target)
	if err != nil {
		return err
	}

	result := applyRecommendationsResult{
		Result:    "failed",
		Target:    c.Target,
		Namespace: c.Namespace,
		Container: c.Container,
		Format:    c.Format,
		DryRun:    c.DryRun,
	}

	if kind != "deployment" {
		result.Reasons = append(result.Reasons, applyRecommendationReason{
			Code:    "unsupported",
			Message: fmt.Sprintf("workload kind %q is not supported in v1", kind),
		})
		return &commandResultError{exitCode: 1, payload: result}
	}

	recommendations, err := loadRecommendationsFile(c.RecommendationsFile)
	if err != nil {
		return err
	}

	requestedResources, err := parseRequestedResources(c.Resources)
	if err != nil {
		return err
	}

	matchingRecommendations := filterMatchingRecommendations(recommendations, "Deployment", name, c.Namespace, c.Container)
	indexedRecommendations, duplicateResources := indexRecommendationsByResource(matchingRecommendations)
	selectedResources := requestedResources
	if len(selectedResources) == 0 {
		selectedResources = availableRecommendationResources(indexedRecommendations)
	}

	for _, resource := range selectedResources {
		recommendation, ok := indexedRecommendations[resource]
		if ok {
			result.Matched = append(result.Matched, applyRecommendationMatch{
				Resource:       resource,
				Status:         recommendation.Status,
				Confidence:     recommendationConfidence(recommendation),
				OriginalValue:  recommendationLabelText(recommendation, "originalValue"),
				SuggestedValue: recommendationLabelText(recommendation, "suggestedValue"),
			})
		}
	}

	for _, resource := range selectedResources {
		if slices.Contains(duplicateResources, resource) {
			result.Reasons = append(result.Reasons, applyRecommendationReason{
				Code:     "ambiguous",
				Resource: resource,
				Message:  fmt.Sprintf("multiple recommendations matched resource %q", resource),
			})
			continue
		}

		recommendation, ok := indexedRecommendations[resource]
		if !ok {
			result.Reasons = append(result.Reasons, applyRecommendationReason{
				Code:     "not_found",
				Resource: resource,
				Message:  fmt.Sprintf("no waiting recommendation found for resource %q", resource),
			})
			continue
		}

		if recommendationConfidence(recommendation) < c.MinConfidence {
			result.Reasons = append(result.Reasons, applyRecommendationReason{
				Code:     "below_confidence_threshold",
				Resource: resource,
				Message:  fmt.Sprintf("recommendation confidence %d is below threshold %d", recommendationConfidence(recommendation), c.MinConfidence),
			})
		}
	}

	if len(selectedResources) == 0 {
		result.Reasons = append(result.Reasons, applyRecommendationReason{
			Code:    "not_found",
			Message: "no applicable recommendations found for the selected workload and container",
		})
		return &commandResultError{exitCode: 1, payload: result}
	}

	renderedManifest, err := renderHelmChart(c.ChartPath, c.ValuesFile)
	if err != nil {
		return err
	}

	if !renderedDeploymentExists(renderedManifest, name, c.Namespace, c.Container) {
		result.Reasons = append(result.Reasons, applyRecommendationReason{
			Code:    "not_found",
			Message: "rendered deployment or container was not found in the Helm chart output",
		})
	}

	valuesData, err := os.ReadFile(c.ValuesFile) // #nosec G304 -- CLI intentionally reads a user-selected Helm values file.
	if err != nil {
		return err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(valuesData, &root); err != nil {
		return fmt.Errorf("parse values file: %w", err)
	}

	candidates := findValuesCandidates(&root, name, c.Container)
	switch len(candidates) {
	case 0:
		result.Reasons = append(result.Reasons, applyRecommendationReason{
			Code:    "not_found",
			Message: "no explicit values mapping found for the selected workload and container",
		})
	case 1:
	default:
		result.Reasons = append(result.Reasons, applyRecommendationReason{
			Code:    "ambiguous",
			Message: "multiple explicit values mappings matched the selected workload and container",
		})
	}

	if len(result.Reasons) > 0 {
		return &commandResultError{exitCode: 1, payload: result}
	}

	candidate := candidates[0]
	overrideValues := map[string]any{}
	for _, resource := range selectedResources {
		recommendation := indexedRecommendations[resource]
		path, err := setRecommendationValue(candidate.Node, resource, recommendationLabelText(recommendation, "suggestedValue"))
		if err != nil {
			result.Reasons = append(result.Reasons, applyRecommendationReason{
				Code:     "unsupported",
				Resource: resource,
				Message:  err.Error(),
			})
			continue
		}

		appendOverrideValue(overrideValues, candidate.Path, resource, recommendationLabelText(recommendation, "suggestedValue"))
		result.Patched = append(result.Patched, applyPatchedResource{
			Resource:       resource,
			Path:           strings.Join(append(candidate.Path, path...), "."),
			OriginalValue:  recommendationLabelText(recommendation, "originalValue"),
			SuggestedValue: recommendationLabelText(recommendation, "suggestedValue"),
		})
	}

	if len(result.Reasons) > 0 {
		return &commandResultError{exitCode: 1, payload: result}
	}

	patchedValues, err := marshalYAMLNode(&root)
	if err != nil {
		return err
	}

	switch c.Format {
	case "override":
		overrideBytes, err := yaml.Marshal(overrideValues)
		if err != nil {
			return fmt.Errorf("marshal override output: %w", err)
		}

		if !bytes.HasSuffix(overrideBytes, []byte("\n")) {
			overrideBytes = append(overrideBytes, '\n')
		}

		result.Result = "generated"
		if c.DryRun {
			result.Result = "planned"
		}
		if !c.DryRun {
			if err := os.WriteFile(c.OutputFile, overrideBytes, 0o600); err != nil { // #nosec G304 -- CLI intentionally writes a user-selected override output file.
				return fmt.Errorf("write override output: %w", err)
			}
			result.ChangedFiles = append(result.ChangedFiles, c.OutputFile)
			result.OutputFile = c.OutputFile
		}
		_, err = ctx.stdout.Write(overrideBytes)
		return err
	case "diff":
		diffOutput, err := unifiedDiff(c.ValuesFile, valuesData, patchedValues)
		if err != nil {
			return err
		}
		result.Result = "patched"
		if c.DryRun {
			result.Result = "planned"
		}
		if !c.DryRun {
			if err := writePatchedValuesFile(c.ValuesFile, patchedValues); err != nil {
				return err
			}
			result.ChangedFiles = append(result.ChangedFiles, c.ValuesFile)
		}
		if diffOutput == "" {
			diffOutput = "\n"
		}
		_, err = io.WriteString(ctx.stdout, diffOutput)
		return err
	default:
		result.Result = "patched"
		if c.DryRun {
			result.Result = "planned"
		}
		if !c.DryRun {
			if err := writePatchedValuesFile(c.ValuesFile, patchedValues); err != nil {
				return err
			}
			result.ChangedFiles = append(result.ChangedFiles, c.ValuesFile)
		}
		return ctx.writeOutput(ctx.stdout, result, "json")
	}
}

func parseRecommendationTarget(target string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(target), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("target must use kind/name form, for example deployment/my-app")
	}

	return strings.ToLower(parts[0]), parts[1], nil
}

func parseRequestedResources(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	seen := make(map[string]struct{})
	resources := make([]string, 0, 4)
	for _, resource := range strings.Split(raw, ",") {
		resource = strings.TrimSpace(resource)
		if resource == "" {
			continue
		}
		if !slices.Contains(supportedRecommendationResources, resource) {
			return nil, fmt.Errorf("unsupported resource %q", resource)
		}
		if _, ok := seen[resource]; ok {
			continue
		}
		seen[resource] = struct{}{}
		resources = append(resources, resource)
	}

	return resources, nil
}

func loadRecommendationsFile(path string) ([]recommendationEntry, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- CLI intentionally reads a user-selected recommendations file.
	if err != nil {
		return nil, fmt.Errorf("read recommendations file: %w", err)
	}

	var list []map[string]any
	if err := yaml.Unmarshal(data, &list); err == nil {
		return recommendationEntriesFromMaps(list), nil
	}

	var payload struct {
		Items []map[string]any `json:"items" yaml:"items"`
	}
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse recommendations file: %w", err)
	}

	return recommendationEntriesFromMaps(payload.Items), nil
}

func recommendationEntriesFromMaps(items []map[string]any) []recommendationEntry {
	recommendations := make([]recommendationEntry, 0, len(items))
	for _, item := range items {
		entry := recommendationEntry{
			Kind:        textValue(item["kind"]),
			Name:        textValue(item["name"]),
			Namespace:   textValue(item["namespace"]),
			ResourceUID: textValue(item["resourceUID"]),
			Status:      textValue(item["status"]),
			Labels:      mapValue(item["labels"]),
			Raw:         item,
		}
		recommendations = append(recommendations, entry)
	}

	return recommendations
}

func filterMatchingRecommendations(recommendations []recommendationEntry, kind, name, namespace, container string) []recommendationEntry {
	filtered := make([]recommendationEntry, 0, len(recommendations))
	for _, recommendation := range recommendations {
		if recommendation.Kind != kind {
			continue
		}
		if recommendation.Name != name {
			continue
		}
		if recommendation.Namespace != namespace {
			continue
		}
		if recommendation.Status != "waiting" {
			continue
		}
		if recommendationLabelText(recommendation, "workloadContainer") != container {
			continue
		}
		filtered = append(filtered, recommendation)
	}
	return filtered
}

func indexRecommendationsByResource(recommendations []recommendationEntry) (map[string]recommendationEntry, []string) {
	indexed := make(map[string]recommendationEntry, len(recommendations))
	duplicates := make([]string, 0)
	seenDuplicates := make(map[string]struct{})

	for _, recommendation := range recommendations {
		resource := recommendationResourceTarget(recommendation.ResourceUID)
		if resource == "" {
			continue
		}
		if _, exists := indexed[resource]; exists {
			if _, recorded := seenDuplicates[resource]; !recorded {
				duplicates = append(duplicates, resource)
				seenDuplicates[resource] = struct{}{}
			}
			continue
		}
		indexed[resource] = recommendation
	}

	return indexed, duplicates
}

func availableRecommendationResources(indexed map[string]recommendationEntry) []string {
	resources := make([]string, 0, len(indexed))
	for _, resource := range supportedRecommendationResources {
		if _, ok := indexed[resource]; ok {
			resources = append(resources, resource)
		}
	}
	return resources
}

func recommendationResourceTarget(resourceUID string) string {
	parts := strings.Split(strings.TrimSpace(resourceUID), "/")
	if len(parts) == 0 {
		return ""
	}

	resource := parts[len(parts)-1]
	if slices.Contains(supportedRecommendationResources, resource) {
		return resource
	}

	return ""
}

func recommendationConfidence(recommendation recommendationEntry) int {
	value, ok := recommendation.Labels["confidence"]
	if !ok {
		return 0
	}

	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func recommendationLabelText(recommendation recommendationEntry, key string) string {
	return textValue(recommendation.Labels[key])
}

func renderHelmChart(chartPath, valuesFile string) ([]byte, error) {
	if _, err := exec.LookPath("helm"); err != nil {
		return nil, fmt.Errorf("render helm chart: helm not found in PATH")
	}
	cmd := exec.Command("helm", "template", "kedify-apply", chartPath, "--values", valuesFile) // #nosec G204 -- executable is fixed; args are explicit CLI inputs for local Helm rendering.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("render helm chart: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func renderedDeploymentExists(rendered []byte, name, namespace, container string) bool {
	decoder := yaml.NewDecoder(bytes.NewReader(rendered))
	for {
		var manifest renderedDeployment
		err := decoder.Decode(&manifest)
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			return false
		}
		if manifest.Kind == "" {
			continue
		}
		if manifest.Kind != "Deployment" {
			continue
		}
		if manifest.Metadata.Name != name || manifest.Metadata.Namespace != namespace {
			continue
		}
		for _, candidate := range manifest.Spec.Template.Spec.Containers {
			if candidate.Name == container {
				return true
			}
		}
		return false
	}
}

func findValuesCandidates(root *yaml.Node, name, container string) []valuesCandidate {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return findValuesCandidates(root.Content[0], name, container)
	}

	candidates := make([]valuesCandidate, 0)
	visitValuesCandidates(root, nil, name, container, &candidates)
	return candidates
}

func visitValuesCandidates(node *yaml.Node, path []string, name, container string, candidates *[]valuesCandidate) {
	if node == nil {
		return
	}
	if node.Kind != yaml.MappingNode {
		return
	}

	if mappingString(node, "name") == name && mappingString(node, "containerName") == container {
		if resourcesNode := mappingValue(node, "resources"); resourcesNode != nil && resourcesNode.Kind == yaml.MappingNode {
			candidatePath := append([]string(nil), path...)
			*candidates = append(*candidates, valuesCandidate{Path: candidatePath, Node: node})
		}
	}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		if value.Kind == yaml.MappingNode {
			visitValuesCandidates(value, append(path, key.Value), name, container, candidates)
		}
	}
}

func setRecommendationValue(candidate *yaml.Node, resource, suggestedValue string) ([]string, error) {
	path, err := valuesPathForResource(resource)
	if err != nil {
		return nil, err
	}

	resourcesNode := mappingValue(candidate, "resources")
	if resourcesNode == nil {
		return nil, errors.New("resources block is not explicitly present in values mapping")
	}

	current := resourcesNode
	for i, segment := range path {
		if current.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("path %q is not an explicit mapping", strings.Join(append([]string{"resources"}, path[:i]...), "."))
		}

		next := mappingValue(current, segment)
		if next == nil {
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			if i == len(path)-1 {
				next = scalarNode(suggestedValue)
			}
			appendMappingEntry(current, segment, next)
		} else if i == len(path)-1 {
			*next = *scalarNode(suggestedValue)
		}
		current = next
	}

	return append([]string{"resources"}, path...), nil
}

func valuesPathForResource(resource string) ([]string, error) {
	switch resource {
	case "cpu-requests":
		return []string{"requests", "cpu"}, nil
	case "cpu-limits":
		return []string{"limits", "cpu"}, nil
	case "memory-requests":
		return []string{"requests", "memory"}, nil
	case "memory-limits":
		return []string{"limits", "memory"}, nil
	default:
		return nil, fmt.Errorf("unsupported resource %q", resource)
	}
}

func appendOverrideValue(root map[string]any, candidatePath []string, resource, suggestedValue string) {
	current := root
	for _, segment := range candidatePath {
		next, ok := current[segment].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[segment] = next
		}
		current = next
	}

	resources, ok := current["resources"].(map[string]any)
	if !ok {
		resources = map[string]any{}
		current["resources"] = resources
	}

	switch resource {
	case "cpu-requests":
		overrideLeaf(resources, []string{"requests", "cpu"}, suggestedValue)
	case "cpu-limits":
		overrideLeaf(resources, []string{"limits", "cpu"}, suggestedValue)
	case "memory-requests":
		overrideLeaf(resources, []string{"requests", "memory"}, suggestedValue)
	case "memory-limits":
		overrideLeaf(resources, []string{"limits", "memory"}, suggestedValue)
	}
}

func overrideLeaf(root map[string]any, path []string, value string) {
	current := root
	for _, segment := range path[:len(path)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[segment] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}

func marshalYAMLNode(root *yaml.Node) ([]byte, error) {
	nodeToEncode := root
	if root != nil && root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		nodeToEncode = root.Content[0]
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(nodeToEncode); err != nil {
		_ = encoder.Close()
		return nil, fmt.Errorf("marshal values file: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}
	return buf.Bytes(), nil
}

func unifiedDiff(valuesFile string, original, patched []byte) (string, error) {
	if _, err := exec.LookPath("diff"); err != nil {
		return "", fmt.Errorf("generate unified diff: diff not found in PATH")
	}
	originalFile, err := writeTempDiffFile("original-values-", original)
	if err != nil {
		return "", err
	}
	defer removeTempDiffFile(originalFile)

	patchedFile, err := writeTempDiffFile("patched-values-", patched)
	if err != nil {
		return "", err
	}
	defer removeTempDiffFile(patchedFile)

	cmd := exec.Command("diff", "-u", "--label", filepath.Clean(valuesFile), "--label", filepath.Clean(valuesFile), originalFile, patchedFile) // #nosec G204 -- executable is fixed; args are controlled temp files and a user-selected values path label.
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return string(output), nil
	}

	return "", fmt.Errorf("generate unified diff: %w", err)
}

func removeTempDiffFile(path string) {
	_ = os.Remove(path)
}

func writeTempDiffFile(prefix string, data []byte) (string, error) {
	file, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", fmt.Errorf("create temp diff file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("write temp diff file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("close temp diff file: %w", err)
	}
	return file.Name(), nil
}

func writePatchedValuesFile(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat values file: %w", err)
	}
	if err := os.WriteFile(path, data, info.Mode()); err != nil { // #nosec G304,G306 -- CLI intentionally updates the user-selected values file while preserving its existing mode.
		return fmt.Errorf("write patched values file: %w", err)
	}
	return nil
}

func mappingString(node *yaml.Node, key string) string {
	value := mappingValue(node, key)
	if value == nil {
		return ""
	}
	return value.Value
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func appendMappingEntry(node *yaml.Node, key string, value *yaml.Node) {
	node.Content = append(node.Content, &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: key,
	}, value)
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: value,
	}
}

func mapValue(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	mapped, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return mapped
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
