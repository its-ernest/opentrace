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
	outputs := make(map[string]string)

	for _, step := range p.Modules {
		input := step.Input

		if strings.HasPrefix(input, "$") {
			ref := strings.TrimPrefix(input, "$")
			val, ok := outputs[ref]
			if !ok {
				return fmt.Errorf(
					"module %q references output of %q but it hasn't run yet",
					step.Name,
					ref,
				)
			}
			input = val
		}

		result, err := runModule(
			ctx,
			filepath.Join(binDir, step.Name),
			sdk.Input{
				Input:  input,
				Config: step.Config,
			},
		)
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

	// 🔑 stdout = machine data only
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	// 🔑 stderr = logs / prints
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("exited with error: %w", err)
	}

	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) == 0 {
		return "", fmt.Errorf("module produced no output")
	}

	var out sdk.Output
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf(
			"module output is not valid JSON: %w\nraw output:\n%s",
			err,
			string(raw),
		)
	}

	return out.Result, nil
}