package cli

import (
	"fmt"

	"github.com/google/uuid"
)

type GetClusterCmd struct {
	Name   string `arg:"" optional:"" name:"name" help:"Cluster name or id."`
	Output string `name:"output" short:"o" help:"Output format." enum:"text,json,yaml" default:"text"`
}

func (c *GetClusterCmd) Run(ctx *context) error {
	token, err := resolveToken(ctx)
	if err != nil {
		return err
	}

	clusters, err := ctx.client.ListClusters(ctx.apiURL, token)
	if err != nil {
		return err
	}

	var cluster map[string]any
	if c.Name != "" {
		if isUUID(c.Name) {
			cluster, err = ctx.client.GetCluster(ctx.apiURL, token, c.Name)
			if err != nil {
				return err
			}
		} else {
			cluster, err = findCluster(clusters, c.Name)
			if err != nil {
				return err
			}
		}
	} else {
		selectedCluster, err := ctx.selectCluster(ctx.stdin, ctx.stdout, ctx.stderr, clusters)
		if err != nil {
			return err
		}
		cluster = selectedCluster
		if id := clusterString(selectedCluster, "id"); isUUID(id) {
			cluster, err = ctx.client.GetCluster(ctx.apiURL, token, id)
			if err != nil {
				return err
			}
		}
	}

	return ctx.writeOutput(ctx.stdout, cluster, c.Output)
}

func findCluster(clusters []map[string]any, query string) (map[string]any, error) {
	for _, cluster := range clusters {
		if clusterString(cluster, "name") == query || clusterString(cluster, "id") == query {
			return cluster, nil
		}
	}

	return nil, fmt.Errorf("cluster %q not found", query)
}

func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func clusterString(cluster map[string]any, key string) string {
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
