// Package main provides the kedify CLI executable entrypoint.
package main

import (
	"os"

	"github.com/kedify/cli/internal/cli"
)

func main() {
	os.Exit(cli.Run())
}
