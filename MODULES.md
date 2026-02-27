# Writing an opentrace module

A module is a standalone Go binary. It receives input over stdin,
does its work, prints what it wants to the terminal, and returns
a result string over stdout. The core handles all the wiring.

---

## Quickstart

```bash
mkdir opentrace-my-module
cd opentrace-my-module
go mod init github.com/you/opentrace-my-module
go get github.com/its-ernest/opentrace/sdk
```

Create `main.go`:

```go
package main

import (
    "fmt"
    "github.com/its-ernest/opentrace/sdk"
)

type MyModule struct{}

func (m *MyModule) Name() string { return "my_module" }

func (m *MyModule) Run(input sdk.Input) (sdk.Output, error) {
    fmt.Println("  received:", input.Input)
    return sdk.Output{Result: input.Input}, nil
}

func main() { sdk.Run(&MyModule{}) }
```

```bash
go mod tidy
go build -o my_module .
```

That is a working module.

---

## SDK types

```go
// what the core sends to your module over stdin
type Input struct {
    Input  string         // the input string — literal or from prior module
    Config map[string]any // config block from the pipeline YAML
}

// what your module must return
type Output struct {
    Result string // JSON string passed to next module if referenced
}
```

---

## Reading input

`input.Input` is always a plain string. What it contains depends on
where it comes from in the pipeline.

**First module in a pipeline — literal value from YAML:**

```yaml
- name: ip_locator
  input: "8.8.8.8"
```

```go
func (m *IPLocator) Run(input sdk.Input) (sdk.Output, error) {
    ip := input.Input   // "8.8.8.8"
}
```

**Any module after — JSON string from prior module's output:**

```yaml
- name: asn_lookup
  input: "$ip_locator"   # receives ip_locator's result string
```

```go
func (m *ASNLookup) Run(input sdk.Input) (sdk.Output, error) {
    // input.Input is a JSON string from ip_locator
    // deserialize it into whatever you expect

    var geo struct {
        Latitude    float64 `json:"latitude"`
        Longitude   float64 `json:"longitude"`
        City        string  `json:"city"`
        CountryCode string  `json:"country_code"`
        Org         string  `json:"org"`
    }

    if err := json.Unmarshal([]byte(input.Input), &geo); err != nil {
        return sdk.Output{}, fmt.Errorf("invalid input: %w", err)
    }

    // use geo.Org, geo.City, geo.Latitude etc.
}
```

Your module defines its own struct matching what it expects.
There is no shared schema — modules agree on structure by convention.

---

## Reading config

`input.Config` is `map[string]any`. Two ways to read it.

**Direct key access — simple configs:**

```go
token, _ := input.Config["token"].(string)
depth, _ := input.Config["depth"].(int)
enabled, _ := input.Config["enabled"].(bool)
```

**Unmarshal into a struct — complex configs:**

```go
type config struct {
    Token          string   `json:"token"`
    OutputDir      string   `json:"output_dir"`
    MinOccurrences int      `json:"min_occurrences"`
    MaxContacts    int      `json:"max_contacts"`
    LeakPaths      []string `json:"leak_paths"`
}

var cfg config
configBytes, err := json.Marshal(input.Config)
if err != nil {
    return sdk.Output{}, fmt.Errorf("config marshal: %w", err)
}
if err := json.Unmarshal(configBytes, &cfg); err != nil {
    return sdk.Output{}, fmt.Errorf("invalid config: %w", err)
}
```

Marshal to bytes first, then unmarshal into your struct.
You cannot unmarshal `map[string]any` directly.

---

## Returning output

`sdk.Output.Result` is a string. If you want the next module to
receive structured data, marshal your result to JSON first.

```go
type result struct {
    Subject      string `json:"subject"`
    ContactsFile string `json:"contacts_file"`
    ContactCount int    `json:"contact_count"`
    Source       string `json:"source"`
}

raw, err := json.Marshal(result{
    Subject:      subject,
    ContactsFile: csvPath,
    ContactCount: count,
    Source:       "leak_cooccurrence_inference",
})
if err != nil {
    return sdk.Output{}, fmt.Errorf("marshal result: %w", err)
}

return sdk.Output{Result: string(raw)}, nil
```

If your module is the last in the pipeline and nothing reads its
output, you can return any string or an empty result:

```go
return sdk.Output{Result: ""}, nil
```

---

## Printing to terminal

Your module owns its display. Print whatever is useful to the operator.
Use `fmt.Printf` or `fmt.Println` freely — it goes directly to the
terminal. It does not affect the result string.

```go
fmt.Printf("  ip       : %s\n", ip)
fmt.Printf("  city     : %s, %s\n", geo.City, geo.CountryCode)
fmt.Printf("  coords   : %.6f, %.6f\n", geo.Latitude, geo.Longitude)
fmt.Printf("  confidence: %.2f\n", confidence)
```

The core reads only the last valid JSON line from stdout as the result.
Everything else is display.

---

## Full module example

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/its-ernest/opentrace/sdk"
)

type IPLocator struct{}

func (m *IPLocator) Name() string { return "ip_locator" }

func (m *IPLocator) Run(input sdk.Input) (sdk.Output, error) {
    ip := input.Input
    token, _ := input.Config["token"].(string)

    url := fmt.Sprintf("https://ipinfo.io/%s/json", ip)
    if token != "" {
        url += "?token=" + token
    }

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Get(url)
    if err != nil {
        return sdk.Output{}, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()

    var data struct {
        City    string `json:"city"`
        Region  string `json:"region"`
        Country string `json:"country"`
        Loc     string `json:"loc"`
        Org     string `json:"org"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return sdk.Output{}, fmt.Errorf("decode: %w", err)
    }

    // print to terminal — module owns its display
    fmt.Printf("  ip      : %s\n", ip)
    fmt.Printf("  city    : %s, %s, %s\n", data.City, data.Region, data.Country)
    fmt.Printf("  coords  : %s\n", data.Loc)
    fmt.Printf("  org     : %s\n", data.Org)

    // return structured result as JSON string
    type result struct {
        IP      string `json:"ip"`
        City    string `json:"city"`
        Region  string `json:"region"`
        Country string `json:"country"`
        Loc     string `json:"loc"`
        Org     string `json:"org"`
    }

    raw, err := json.Marshal(result{
        IP:      ip,
        City:    data.City,
        Region:  data.Region,
        Country: data.Country,
        Loc:     data.Loc,
        Org:     data.Org,
    })
    if err != nil {
        return sdk.Output{}, fmt.Errorf("marshal result: %w", err)
    }

    return sdk.Output{Result: string(raw)}, nil
}

func main() { sdk.Run(&IPLocator{}) }
```

---

## manifest.yaml

Every module must include a `manifest.yaml` at the root of the repo.

```yaml
name: ip_locator           # snake_case, matches what users type in install + pipeline
version: 0.1.0             # semver — bump on every release
description: Resolves an IP to country/city/coordinates via IPinfo.io
author: your_handle
official: false            # only set by opentrace maintainers
verified: false            # set after code review by opentrace maintainers
entity_types: [ip]         # what kind of input this module expects
                           # ip | email | username | domain | phone | text | url | coords
```

---

## Repository structure

```
opentrace-my-module/
├── main.go
├── go.mod
└── manifest.yaml
```

Name your repo `opentrace-<module_name>`.
The `opentrace-` prefix makes it discoverable and identifies it
as part of the ecosystem.

---

## Common mistakes

**Using `input.Value` instead of `input.Input`**

```go
// wrong
subject := input.Value

// correct
subject := input.Input
```

**Unmarshaling config directly**

```go
// wrong — Config is map[string]any, not []byte
json.Unmarshal(input.Config, &cfg)

// correct — marshal to bytes first
b, _ := json.Marshal(input.Config)
json.Unmarshal(b, &cfg)
```

**Returning a struct instead of a string**

```go
// wrong — Result is a string
return sdk.Output{Result: myStruct}, nil

// correct — marshal first
raw, _ := json.Marshal(myStruct)
return sdk.Output{Result: string(raw)}, nil
```

**Not printing anything**

Your module is the display layer. The core prints nothing.
If your module is silent, the operator sees a blank run.
Print what you found.

---

## Publishing

Once your module works:

1. Push to `github.com/you/opentrace-my-module`
2. Users can install immediately by repo:

```bash
opentrace install github.com/you/opentrace-my-module
```

3. To list it in the registry for name-based install,
   open a PR to [opentrace-modules](https://github.com/its-ernest/opentrace-modules)
   adding one entry to `modules/registry.json`:

```json
"my_module": {
    "repo": "github.com/you/opentrace-my-module",
    "version": "0.1.0",
    "author": "you",
    "description": "What your module does",
    "official": false,
    "verified": false
}
```

Once merged, users install by name:

```bash
opentrace install my_module
```
