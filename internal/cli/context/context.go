package context

import (
	"io"
	"strings"

	"github.com/kedify/cli/internal/service"
)

type Context struct {
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	APIURL        string
	Token         string
	Client        service.ClusterService
	Credentials   service.CredentialsStore
	ReadSecret    func(io.Reader, io.Writer, io.Writer) (string, error)
	SelectCluster func(io.Reader, io.Writer, io.Writer, []map[string]any) (map[string]any, error)
	WriteOutput   func(io.Writer, any, string) error
}

func ResolveToken(ctx *Context) (string, error) {
	if token := strings.TrimSpace(ctx.Token); token != "" {
		return token, nil
	}

	creds, err := ctx.Credentials.ReadCredentials()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(creds.Token), nil
}
