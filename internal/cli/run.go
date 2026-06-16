package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kedify/cli/internal/api"
	"github.com/kedify/cli/internal/config"
)

const defaultAPIURL = "https://api.dev.kedify.io/v1"

var errHelpShown = errors.New("help shown")

type context struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	apiURL string
	client *api.Client
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx := &context{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		client: api.NewClient(),
	}

	if err := run(ctx, args); err != nil {
		if errors.Is(err, errHelpShown) {
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "kedify: error: %v\n", err)
		return 1
	}

	return 0
}

func run(ctx *context, args []string) error {
	if len(args) == 0 {
		writeRootHelp(ctx.stdout)
		return nil
	}

	apiURL, remaining, err := parseGlobalFlags(args, ctx.stdout)
	if err != nil {
		return err
	}
	ctx.apiURL = apiURL

	if len(remaining) == 0 {
		writeRootHelp(ctx.stdout)
		return nil
	}

	switch remaining[0] {
	case "help", "--help", "-h":
		writeRootHelp(ctx.stdout)
		return nil
	case "login":
		return runLogin(ctx, remaining[1:])
	case "list":
		return runList(ctx, remaining[1:])
	default:
		return fmt.Errorf("unknown command %q", remaining[0])
	}
}

func parseGlobalFlags(args []string, stdout io.Writer) (string, []string, error) {
	apiURL := defaultAPIURL

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			writeRootHelp(stdout)
			return "", nil, errHelpShown
		case arg == "--apiurl":
			if i+1 >= len(args) {
				return "", nil, errors.New("missing value for --apiurl")
			}
			apiURL = args[i+1]
			i++
		case strings.HasPrefix(arg, "--apiurl="):
			apiURL = strings.TrimPrefix(arg, "--apiurl=")
		case strings.HasPrefix(arg, "-"):
			return "", nil, fmt.Errorf("unknown flag %q", arg)
		default:
			if envURL := config.APIURLFromEnv(); envURL != "" && apiURL == defaultAPIURL {
				apiURL = envURL
			}
			return apiURL, args[i:], nil
		}
	}

	if envURL := config.APIURLFromEnv(); envURL != "" && apiURL == defaultAPIURL {
		apiURL = envURL
	}

	return apiURL, nil, nil
}

func runList(ctx *context, args []string) error {
	if len(args) == 0 {
		writeListHelp(ctx.stdout)
		return nil
	}

	switch args[0] {
	case "clusters":
		return runListClusters(ctx, args[1:])
	case "help", "--help", "-h":
		writeListHelp(ctx.stdout)
		return nil
	default:
		return fmt.Errorf("unknown list subcommand %q", args[0])
	}
}

func runListClusters(ctx *context, args []string) error {
	flags := flag.NewFlagSet("clusters", flag.ContinueOnError)
	flags.SetOutput(ctx.stderr)

	outputFormat := flags.String("output", "json", "Output format.")
	flags.StringVar(outputFormat, "o", "json", "Output format.")
	flags.Usage = func() {
		writeListClustersHelp(ctx.stdout)
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if flags.NArg() > 0 {
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

	return writeOutput(ctx.stdout, clusters, *outputFormat)
}

func writeRootHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: kedify <command> [flags]

Kedify command line interface.

Flags:
  -h, --help             Show help.
      --apiurl string    Base URL for the Kedify API (default "https://api.dev.kedify.io/v1")

Commands:
  login                  Read an auth token from stdin and store it locally.
  list clusters          List clusters.
`)
}

func writeListHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: kedify list <command>

Commands:
  clusters               List clusters.
`)
}

func writeListClustersHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: kedify list clusters [flags]

Flags:
  -h, --help             Show help.
  -o, --output string    Output format (json|yaml) (default "json")
`)
}
