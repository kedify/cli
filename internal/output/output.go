package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

func Write(w io.Writer, value any, format string) error {
	var (
		data []byte
		err  error
	)

	switch format {
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
