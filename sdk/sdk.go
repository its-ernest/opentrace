package sdk

import (
	"encoding/json"
	"fmt"
	"os"
)

// Input is what the core sends to a module over stdin.
type Input struct {
	Input  string         `json:"input"`
	Config map[string]any `json:"config"`
}

// Output is what every module must return over stdout.
type Output struct {
	Result string `json:"result"` // passed as input to next module if referenced
}

// Module is the interface every module implements.
type Module interface {
	Name() string
	Run(input Input) (Output, error)
}

// Run is called in every module's main().
// Handles all stdin/stdout plumbing — module dev never touches this.
func Run(m Module) {
	var in Input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] bad input: %v\n", m.Name(), err)
		os.Exit(1)
	}

	out, err := m.Run(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] error: %v\n", m.Name(), err)
		os.Exit(1)
	}

	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] encode output: %v\n", m.Name(), err)
		os.Exit(1)
	}
}