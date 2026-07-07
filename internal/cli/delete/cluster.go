package delete

import (
	"fmt"

	clictx "github.com/kedify/cli/internal/cli/context"
	getcmd "github.com/kedify/cli/internal/cli/get"
)

type DeleteClusterCmd struct {
	Name string `arg:"" optional:"" name:"name" help:"Cluster name or id."`
}

func (c *DeleteClusterCmd) Run(ctx *clictx.Context) error {
	token, err := clictx.ResolveToken(ctx)
	if err != nil {
		return err
	}

	clusterID, clusterName, err := c.resolveCluster(ctx, token)
	if err != nil {
		return err
	}

	if err := ctx.Client.DeleteCluster(ctx.APIURL, token, clusterID); err != nil {
		return err
	}

	if clusterName != "" {
		_, err = fmt.Fprintf(ctx.Stderr, "Deleted cluster %q (%s).\n", clusterName, clusterID)
	} else {
		_, err = fmt.Fprintf(ctx.Stderr, "Deleted cluster %s.\n", clusterID)
	}

	return err
}

func (c *DeleteClusterCmd) resolveCluster(ctx *clictx.Context, token string) (string, string, error) {
	if c.Name != "" && getcmd.IsUUID(c.Name) {
		cluster, err := ctx.Client.GetCluster(ctx.APIURL, token, c.Name)
		if err == nil {
			return c.Name, clusterString(cluster, "name"), nil
		}
		return c.Name, "", nil
	}

	clusters, err := ctx.Client.ListClusters(ctx.APIURL, token)
	if err != nil {
		return "", "", err
	}

	if c.Name != "" {
		cluster, err := getcmd.FindCluster(clusters, c.Name)
		if err != nil {
			return "", "", err
		}
		return clusterString(cluster, "id"), clusterString(cluster, "name"), nil
	}

	cluster, err := ctx.SelectCluster(ctx.Stdin, ctx.Stdout, ctx.Stderr, clusters)
	if err != nil {
		return "", "", err
	}

	return clusterString(cluster, "id"), clusterString(cluster, "name"), nil
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
