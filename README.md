# opentrace

Modular OSINT pipeline runner.

You define a pipeline. opentrace runs it. Modules do the work.
```bash
opentrace run track.yaml
```

---

## How it works

A pipeline is a YAML file. Each step is a module. A module receives an input string and a config block, does its work, prints what it wants to the terminal, and returns a result string. That result can be passed as input to the next module.
```yaml
modules:
  - name: ip_locator
    input: "8.8.8.8"
    config:
      token: "${IPINFO_TOKEN}"

  - name: social_patterns
    input: "$ip_locator"     # receives ip_locator's output
    config:
      depth: 2
```

The core does not interpret results. It does not cluster, score, or display anything. Modules own their logic, their output, and their inference.

---

## Install
```bash
go install github.com/its-ernest/opentrace/cmd/opentrace@latest
```

Or build from source:
```bash
git clone https://github.com/its-ernest/opentrace
cd opentrace
make build
```

Requires Go 1.22+.

---

## Usage
```bash
# Run a pipeline
opentrace run track.yaml

# Install a module
opentrace install ip_locator

# Uninstall a module
opentrace uninstall ip_locator

# List installed modules
opentrace modules
```

---

## Pipeline reference
```yaml
modules:
  - name: <module_name>       # must be installed
    input: "<string>"         # literal string, or $module_name to reference a prior output
    config:                   # passed to the module as-is
      key: value
      token: "${ENV_VAR}"     # env vars are expanded automatically
```

**Input referencing**

If `input` starts with `$`, it is treated as a reference to a previous module's output. The reference must match a module name that has already run in the same pipeline.
```yaml
modules:
  - name: ip_locator
    input: "1.2.3.4"

  - name: geo_cluster
    input: "$ip_locator"      # receives whatever ip_locator returned
```

Modules run sequentially in declaration order.

---

## Modules

Modules are standalone executables. They live in a separate repository and are installed on demand.
```bash
opentrace install ip_locator
opentrace install social_patterns
```

Installed modules are stored in `~/.opentrace/bin/`. The registry is at `~/.opentrace/registry.json`.

Browse available modules: [github.com/its-ernest/opentrace-modules](https://github.com/its-ernest/opentrace-modules)

---

## Writing a module

Every module is a Go program that imports the opentrace SDK, implements one interface, and calls `sdk.Run()` in `main()`.
```go
package main

import "github.com/its-ernest/opentrace/sdk"

type MyModule struct{}

func (m *MyModule) Name() string { return "my_module" }

func (m *MyModule) Run(input sdk.Input) (sdk.Output, error) {
    // input.Input  — the string passed from the pipeline or prior module
    // input.Config — the config map from the pipeline YAML

    // print whatever you want here

    return sdk.Output{Result: "value passed to next module"}, nil
}

func main() { sdk.Run(&MyModule{}) }
```

The SDK handles all stdin/stdout plumbing. You never touch it.

**SDK types**
```go
type Input struct {
    Input  string         // the input string
    Config map[string]any // config from pipeline YAML
}

type Output struct {
    Result string // returned to core, passed to next module if referenced
}
```

**Module repository structure**
```
opentrace-modules/
└── my_module/
    ├── main.go
    ├── go.mod
    └── manifest.yaml
```
```yaml
# manifest.yaml
name: my_module
version: 0.1.0
description: What this module does
author: your_handle
```

Submit a PR to [opentrace-modules](https://github.com/its-ernest/opentrace-modules) to publish your module.

---

## Architecture
```
opentrace (core)
├── reads pipeline YAML
├── resolves module binaries from ~/.opentrace/bin/
├── runs each module as a subprocess
│   ├── sends Input as JSON over stdin
│   └── reads Output as JSON from stdout
└── passes output.Result to the next module if referenced

opentrace-modules (separate repo)
└── each module is an independent Go binary
    └── imports github.com/its-ernest/opentrace/sdk
```

The core never knows what a module does. Modules never know they are being orchestrated.

---

## License

MIT