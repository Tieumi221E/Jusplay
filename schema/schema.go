// Package schema embeds the language-neutral JSON Schema contracts and
// validates documents against them.
package schema

import (
	"bytes"
	"embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed *.schema.json
var files embed.FS

// Names of the embedded schemas.
const (
	Capture  = "capture"
	Report   = "report"
	Manifest = "manifest"
)

var (
	once     sync.Once
	compiled map[string]*jsonschema.Schema
	initErr  error
)

func load() {
	c := jsonschema.NewCompiler()
	urls := map[string]string{}
	for _, name := range []string{Capture, Report, Manifest} {
		b, err := files.ReadFile(name + ".schema.json")
		if err != nil {
			initErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			initErr = fmt.Errorf("%s schema: %w", name, err)
			return
		}
		id, _ := doc.(map[string]any)["$id"].(string)
		if id == "" {
			initErr = fmt.Errorf("%s schema has no $id", name)
			return
		}
		if err := c.AddResource(id, doc); err != nil {
			initErr = err
			return
		}
		urls[name] = id
	}
	compiled = map[string]*jsonschema.Schema{}
	for name, id := range urls {
		s, err := c.Compile(id)
		if err != nil {
			initErr = fmt.Errorf("%s schema: %w", name, err)
			return
		}
		compiled[name] = s
	}
}

// Validate checks raw JSON bytes against the named schema.
func Validate(name string, data []byte) error {
	once.Do(load)
	if initErr != nil {
		return initErr
	}
	s, ok := compiled[name]
	if !ok {
		return fmt.Errorf("unknown schema %q", name)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s: invalid JSON: %w", name, err)
	}
	if err := s.Validate(inst); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
