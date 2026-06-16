package cli

import (
	"io"

	"github.com/kedify/cli/internal/output"
)

func writeOutput(w io.Writer, value any, format string) error {
	return output.Write(w, value, format)
}
