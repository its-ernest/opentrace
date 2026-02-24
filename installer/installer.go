package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const modulesRepo = "https://github.com/its-ernest/opentrace-modules"

type Registry map[string]string

func home() string { h, _ := os.UserHomeDir(); return h }
func BinDir() string      { return filepath.Join(home(), ".opentrace", "bin") }
func registryPath() string { return filepath.Join(home(), ".opentrace", "registry.json") }

func LoadRegistry() Registry {
	r := Registry{}
	data, err := os.ReadFile(registryPath())
	if err != nil {
		return r
	}
	_ = json.Unmarshal(data, &r)
	return r
}

func saveRegistry(r Registry) error {
	_ = os.MkdirAll(filepath.Dir(registryPath()), 0o755)
	data, _ := json.MarshalIndent(r, "", "  ")
	return os.WriteFile(registryPath(), data, 0o644)
}

func Install(name string) error {
	if err := os.MkdirAll(BinDir(), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	tmp, err := os.MkdirTemp("", "opentrace-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	fmt.Printf("  fetching %s...\n", name)

	clone := exec.Command("git", "clone", "--depth=1", "--filter=blob:none",
		"--sparse", modulesRepo, tmp)
	if out, err := clone.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %s: %w", string(out), err)
	}

	sparse := exec.Command("git", "-C", tmp, "sparse-checkout", "set", name)
	if out, err := sparse.CombinedOutput(); err != nil {
		return fmt.Errorf("sparse-checkout: %s: %w", string(out), err)
	}

	srcDir := filepath.Join(tmp, name)
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return fmt.Errorf("module %q not found in opentrace-modules", name)
	}

	binName := name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(BinDir(), binName)

	fmt.Printf("  building %s...\n", name)
	if out, err := exec.Command("go", "build", "-o", binPath, srcDir).CombinedOutput(); err != nil {
		return fmt.Errorf("build: %s: %w", string(out), err)
	}

	reg := LoadRegistry()
	reg[name] = binPath
	_ = saveRegistry(reg)

	fmt.Printf("  ✓ %s installed\n", name)
	return nil
}

func Uninstall(name string) error {
	reg := LoadRegistry()
	binPath, ok := reg[name]
	if !ok {
		return fmt.Errorf("module %q is not installed", name)
	}
	_ = os.Remove(binPath)
	delete(reg, name)
	return saveRegistry(reg)
}

func List() {
	reg := LoadRegistry()
	if len(reg) == 0 {
		fmt.Println("  no modules installed — run: opentrace install <name>")
		return
	}
	fmt.Println()
	for name := range reg {
		fmt.Printf("  %s\n", name)
	}
	fmt.Println()
}