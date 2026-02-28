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

Requires Go 1.22+.
```bash
go install github.com/its-ernest/opentrace/cmd/opentrace@latest
```

Or build from source:
```bash
git clone https://github.com/its-ernest/opentrace
cd opentrace
go mod tidy
make build
```

**If the command is not found after install:**

Go installs binaries to `$GOPATH/bin`. Add it to your PATH if it isn't already.
```bash
# find your GOPATH
go env GOPATH

# add to ~/.bashrc or ~/.zshrc
export PATH=$PATH:$(go env GOPATH)/bin

# reload
source ~/.bashrc   # or source ~/.zshrc
```

---

## Usage
```bash
# run a pipeline
opentrace run track.yaml

# install an official module by name
opentrace install ip_locator

# install a community module by repo
opentrace install github.com/alice/opentrace-face-osint

# uninstall a module
opentrace uninstall ip_locator

# list installed modules
opentrace modules
```

---

## Installing modules

**Official modules** are maintained in the opentrace-modules repository.
Install them by name:
```bash
opentrace install ip_locator
opentrace install social_patterns
```

**Community modules** live in their own repositories.
Install them by passing the full repo path:
```bash
opentrace install github.com/alice/opentrace-face-osint
opentrace install github.com/bob/opentrace-wifi-scanner
```

The installer detects which is which automatically.
You will be prompted before installing any unverified module.
```
  name        : face_osint
  version     : 0.1.0
  author      : alice
  description : Reverse face search across public platforms
  official    : false
  verified    : false

  ⚠  face_osint is unverified (community module). Install anyway? (y/n):
```

Installed modules are stored in `~/.opentrace/bin/`.
The local registry is at `~/.opentrace/registry.json`.

Browse official and listed community modules:
[github.com/its-ernest/opentrace-modules](https://github.com/its-ernest/opentrace-modules)

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

If `input` starts with `$`, it is treated as a reference to a previous module's output.
The reference must match a module name that has already run in the same pipeline.
```yaml
modules:
  - name: ip_locator
    input: "1.2.3.4"

  - name: geo_cluster
    input: "$ip_locator"      # receives whatever ip_locator returned
```

Modules run sequentially in declaration order.

---

## Writing a module

Every module is a standalone Go binary that imports the opentrace SDK,
implements one interface, and calls `sdk.Run()` in `main()`.
```go
package main

import "github.com/its-ernest/opentrace/sdk"

type MyModule struct{}

func (m *MyModule) Name() string { return "my_module" }

func (m *MyModule) Run(input sdk.Input) (sdk.Output, error) {
    // input.Input  — the string passed from the pipeline or prior module
    // input.Config — the config map from the pipeline YAML

    // print whatever you want to the terminal here

    return sdk.Output{Result: "value passed to next module"}, nil
}

func main() { sdk.Run(&MyModule{}) }
```

The SDK handles all stdin/stdout plumbing between the core and your module.
You never touch it directly.

**SDK types**
```go
type Input struct {
    Input  string         // the input string
    Config map[string]any // config block from the pipeline YAML
}

type Output struct {
    Result string // stored by core, passed as input to next module if referenced
}
```

**Your module repository structure**
```
opentrace-your-module/
├── main.go
├── go.mod
└── manifest.yaml
```
```yaml
# manifest.yaml
name: your_module
version: 0.1.0
description: What this module does
author: your_handle
official: false
verified: false
entity_types: [ip]   # ip | email | username | domain | phone | text | url
```

**Publishing**

Anyone can install your module directly from your repo without any approval:
```bash
opentrace install github.com/you/opentrace-your-module
```

To list it in the official registry so it is discoverable by name,
open a PR to [opentrace-modules](https://github.com/its-ernest/opentrace-modules)
adding one entry to `modules/registry.json`:
```json
"your_module": {
    "repo": "github.com/you/opentrace-your-module",
    "version": "0.1.0",
    "author": "you",
    "description": "What your module does",
    "official": false,
    "verified": false
}
```

That is the entire PR. One JSON block. Nobody else's code is touched.
Once listed, users can install by name:
```bash
opentrace install your_module
```

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
├── modules/registry.json        ← index of all official + listed community modules
└── modules/<name>/<version>/    ← official modules maintained here
    ├── main.go
    ├── go.mod
    └── manifest.yaml

community modules
└── each lives in its own repo    ← installed directly by repo path
    └── imports github.com/its-ernest/opentrace/sdk
```

The core never knows what a module does.
Modules never know they are being orchestrated.
The only contract between them is stdin and stdout.

---

## Uninstall opentrace
```bash
# remove the binary
rm $(which opentrace)

# remove all installed modules + registry
rm -rf ~/.opentrace
```

## License

Mozilla Public License Version 2.0