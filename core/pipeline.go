package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/its-ernest/opentrace/sdk"
	"gopkg.in/yaml.v3"
)

type Step struct {
	Name   string         `yaml:"name"`
	Input  string         `yaml:"input"`
	Config map[string]any `yaml:"config"`
}

type Pipeline struct {
	Modules []Step `yaml:"modules"`
}

func Load(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read pipeline %q: %w", path, err)
	}
	var p Pipeline
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), &p); err != nil {
		return nil, fmt.Errorf("invalid pipeline YAML: %w", err)
	}
	if len(p.Modules) == 0 {
		return nil, fmt.Errorf("pipeline has no modules")
	}
	return &p, nil
}

func Run(ctx context.Context, p *Pipeline, binDir string) error {
	// outputs holds each module's result string, keyed by module name
	outputs := make(map[string]string)

	for _, step := range p.Modules {
		// Resolve input — if starts with $ it's a reference to a prior output
		input := step.Input
		if strings.HasPrefix(input, "$") {
			ref := strings.TrimPrefix(input, "$")
			val, ok := outputs[ref]
			if !ok {
				return fmt.Errorf("module %q references output of %q but it hasn't run yet", step.Name, ref)
			}
			input = val
		}

		result, err := runModule(ctx, filepath.Join(binDir, step.Name), sdk.Input{
			Input:  input,
			Config: step.Config,
		})
		if err != nil {
			return fmt.Errorf("[%s] %w", step.Name, err)
		}

		outputs[step.Name] = result
	}

	return nil
}

func runModule(ctx context.Context, binPath string, in sdk.Input) (string, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = os.Stdout // module prints directly to terminal
	cmd.Stderr = os.Stderr

	// needs both passthrough and capture of stdout for piping.
	// using a custom writer that tees to stdout and a buffer.
	var buf bytes.Buffer
	tee := &teeWriter{w: os.Stdout, buf: &buf}
	cmd.Stdout = tee

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("exited with error: %w", err)
	}

	// Extract result field from the module's JSON output
	var out sdk.Output
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		// Module may print non-JSON lines before the final JSON — find last line
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if err2 := json.Unmarshal([]byte(lines[i]), &out); err2 == nil {
				return out.Result, nil
			}
		}
		return "", fmt.Errorf("could not parse output JSON: %w", err)
	}

	return out.Result, nil
}

// teeWriter writes to both a passthrough writer and an internal buffer.
type teeWriter struct {
	w   *os.File
	buf *bytes.Buffer
}

func (t *teeWriter) Write(p []byte) (int, error) {
	t.buf.Write(p)
	return t.w.Write(p)
}