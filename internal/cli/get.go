package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kedify/cli/internal/config"
	"github.com/kedify/cli/internal/tui"
)

func runGetCluster(ctx *context, args []string) error {
	flags := flag.NewFlagSet("cluster", flag.ContinueOnError)
	flags.SetOutput(ctx.stderr)

	outputFormat := flags.String("output", "json", "Output format.")
	flags.StringVar(outputFormat, "o", "json", "Output format.")
	flags.Usage = func() {
		writeGetClusterHelp(ctx.stdout)
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if flags.NArg() > 1 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	creds, err := config.ReadCredentials()
	if err != nil {
		return err
	}

	clusters, err := ctx.client.ListClusters(ctx.apiURL, creds.Token)
	if err != nil {
		return err
	}

	var cluster map[string]any
	if flags.NArg() == 1 {
		cluster, err = findCluster(clusters, flags.Arg(0))
		if err != nil {
			return err
		}
	} else {
		cluster, err = tui.SelectClusterOrFail(ctx.stdin, ctx.stdout, clusters)
		if err != nil {
			return err
		}
	}

	return writeOutput(ctx.stdout, cluster, *outputFormat)
}

func findCluster(clusters []map[string]any, query string) (map[string]any, error) {
	for _, cluster := range clusters {
		if clusterString(cluster, "name") == query || clusterString(cluster, "id") == query {
			return cluster, nil
		}
	}

	return nil, fmt.Errorf("cluster %q not found", query)
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

func writeGetClusterHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: kedify get cluster [name] [flags]

Get a cluster by name or id.
If no name is provided, an interactive picker is shown.

Flags:
  -h, --help             Show help.
  -o, --output string    Output format (json|yaml) (default "json")
`)
}
