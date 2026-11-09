// Command check-contracts checks the vendored herdr-core schema and hiik's
// state schema: valid JSON, a known license, every method the client uses,
// and the definitions the store expects.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var requiredMethods = []string{
	"hello",
	"terminal.list",
	"terminal.run",
	"terminal.kill",
	"terminal.label",
	"terminal.attach",
	"terminal.read",
	"events.subscribe",
	"machine.list",
	"machine.exec",
	"file.read",
	"file.write",
	"file.edit",
}

func main() {
	root := flag.String("root", "..", "repository root")
	flag.Parse()

	var problems []string
	schemaPath := filepath.Join(*root, "contracts", "hc.openrpc.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		fatal(err)
	}
	var schema struct {
		Info struct {
			Title   string `json:"title"`
			Version string `json:"version"`
			License struct {
				Name string `json:"name"`
			} `json:"license"`
		} `json:"info"`
		Methods []struct {
			Name string `json:"name"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		fatal(fmt.Errorf("%s: %w", schemaPath, err))
	}
	if schema.Info.License.Name != "Apache-2.0" {
		problems = append(problems, "core schema: license is not Apache-2.0")
	}
	have := make(map[string]bool, len(schema.Methods))
	for _, m := range schema.Methods {
		have[m.Name] = true
	}
	for _, name := range missing(have, requiredMethods) {
		problems = append(problems, "core schema: missing method "+name)
	}

	statePath := filepath.Join(*root, "contracts", "state.schema.json")
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		fatal(err)
	}
	var state struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(stateData, &state); err != nil {
		fatal(fmt.Errorf("%s: %w", statePath, err))
	}
	defs := make(map[string]bool, len(state.Defs))
	for name := range state.Defs {
		defs[name] = true
	}
	for _, name := range missing(defs, []string{"space", "tab", "node", "ui"}) {
		problems = append(problems, "state schema: missing $defs/"+name)
	}

	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, p)
		}
		os.Exit(1)
	}
	fmt.Printf("contracts ok: %s %s, %d methods\n", schema.Info.Title, schema.Info.Version, len(schema.Methods))
}

// missing returns the entries of want that have no true entry in have,
// keeping want's order so reports are stable.
func missing(have map[string]bool, want []string) []string {
	var out []string
	for _, name := range want {
		if !have[name] {
			out = append(out, name)
		}
	}
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
