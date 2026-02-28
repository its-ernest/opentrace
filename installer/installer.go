package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const registryRepo = "https://github.com/its-ernest/opentrace-modules"

type Manifest struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Description string   `yaml:"description"`
	Author      string   `yaml:"author"`
	EntityTypes []string `yaml:"entity_types"`
}

type RegistryEntry struct {
	BinPath string `json:"bin_path"`
	Version string `json:"version"`
	Author  string `json:"author"`
	Repo    string `json:"repo"`
}

type Registry map[string]RegistryEntry

func home() string         { h, _ := os.UserHomeDir(); return h }
func BinDir() string       { return filepath.Join(home(), ".opentrace", "bin") }
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

// Install is the single entry point.
//
// Two forms accepted:
//   opentrace install ip_locator                              → looks up name in opentrace-modules registry
//   opentrace install github.com/user/repo                   → clones directly from that repo
func Install(arg string) error {
	if err := os.MkdirAll(BinDir(), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	if isRepoPath(arg) {
		return installFromRepo(arg)
	}
	return installFromRegistry(arg)
}

// isRepoPath returns true if arg looks like a repo path (contains a slash).
func isRepoPath(arg string) bool {
	return strings.Contains(arg, "/")
}

// installFromRegistry looks up the module name in opentrace-modules/registry.json
// then delegates to installFromRepo using the registered repo URL.
func installFromRegistry(name string) error {
	tmp, err := os.MkdirTemp("", "opentrace-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	fmt.Printf("  looking up %s in registry...\n", name)

	// sparse clone just the registry.json
	if out, err := exec.Command("git", "clone",
		"--depth=1", "--filter=blob:none", "--sparse",
		registryRepo, tmp,
	).CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %s: %w", string(out), err)
	}

	if out, err := exec.Command("git", "-C", tmp,
		"sparse-checkout", "set", "registry.json",
	).CombinedOutput(); err != nil {
		return fmt.Errorf("sparse-checkout: %s: %w", string(out), err)
	}

	// read registry.json
	regData, err := os.ReadFile(filepath.Join(tmp, "registry.json"))
	if err != nil {
		return fmt.Errorf("cannot read registry.json from opentrace-modules: %w", err)
	}

	// registry.json is map[name]repo_url
	var index map[string]string
	if err := json.Unmarshal(regData, &index); err != nil {
		return fmt.Errorf("invalid registry.json: %w", err)
	}

	repoURL, ok := index[name]
	if !ok {
		return fmt.Errorf(
			"module %q not found in registry\n\n"+
				"  if this is a third-party module, install it directly:\n"+
				"  opentrace install github.com/<user>/%s\n",
			name, name,
		)
	}

	fmt.Printf("  found %s → %s\n", name, repoURL)
	return installFromRepo(repoURL)
}

// installFromRepo clones a repo directly and builds the module.
// arg can be:
//   github.com/user/repo
//   https://github.com/user/repo
func installFromRepo(arg string) error {
	// normalize to full URL
	repoURL := arg
	if !strings.HasPrefix(arg, "https://") && !strings.HasPrefix(arg, "http://") {
		repoURL = "https://" + arg
	}

	// derive a local name from the last path segment
	// github.com/user/opentrace-face-osint → face-osint
	// github.com/user/contacts_graph_extract → contacts_graph_extract
	lastSegment := arg[strings.LastIndex(arg, "/")+1:]
	localName := strings.TrimPrefix(lastSegment, "opentrace-")

	tmp, err := os.MkdirTemp("", "opentrace-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	fmt.Printf("  cloning %s...\n", repoURL)

	if out, err := exec.Command("git", "clone",
		"--depth=1", repoURL, tmp,
	).CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}

	// read manifest from root of repo
	manifest, err := readManifest(filepath.Join(tmp, "manifest.yaml"))
	if err != nil {
		return fmt.Errorf("manifest: %w", err)
	}

	// manifest name takes priority over derived name
	if manifest.Name != "" {
		localName = manifest.Name
	}

	printManifest(manifest, repoURL)

	// always prompt — no module is pre-trusted
	fmt.Printf("  install %s? (y/n): ", localName)
	var confirm string
	fmt.Scan(&confirm)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("  aborted.")
		return nil
	}

	return build(localName, tmp, manifest, repoURL)
}

// build compiles the module and registers it locally.
func build(name, srcDir string, manifest *Manifest, repo string) error {
	binName := name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(BinDir(), binName)

	fmt.Printf("  building %s@%s...\n", name, manifest.Version)

	if out, err := exec.Command(
		"go", "build", "-trimpath", "-o", binPath, srcDir,
	).CombinedOutput(); err != nil {
		return fmt.Errorf("build failed:\n%s", string(out))
	}

	reg := LoadRegistry()
	reg[name] = RegistryEntry{
		BinPath: binPath,
		Version: manifest.Version,
		Author:  manifest.Author,
		Repo:    repo,
	}
	if err := saveRegistry(reg); err != nil {
		return fmt.Errorf("save registry: %w", err)
	}

	fmt.Printf("  ✓ %s@%s installed → %s\n", name, manifest.Version, binPath)
	return nil
}

func Uninstall(name string) error {
	reg := LoadRegistry()
	entry, ok := reg[name]
	if !ok {
		return fmt.Errorf("module %q is not installed", name)
	}
	_ = os.Remove(entry.BinPath)
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
	fmt.Printf("  %-26s  %-10s  %-16s  %s\n", "MODULE", "VERSION", "AUTHOR", "REPO")
	fmt.Printf("  %-26s  %-10s  %-16s  %s\n",
		"──────────────────────────", "─────────", "───────────────", "────────────────────────────────")
	for name, entry := range reg {
		fmt.Printf("  %-26s  %-10s  %-16s  %s\n",
			name, entry.Version, entry.Author, entry.Repo)
	}
	fmt.Println()
}

func BinPath(name string) (string, error) {
	reg := LoadRegistry()
	entry, ok := reg[name]
	if !ok {
		return "", fmt.Errorf("module %q is not installed", name)
	}
	return entry.BinPath, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func printManifest(m *Manifest, repo string) {
	fmt.Println()
	fmt.Printf("  name        : %s\n", m.Name)
	fmt.Printf("  version     : %s\n", m.Version)
	fmt.Printf("  author      : %s\n", m.Author)
	fmt.Printf("  description : %s\n", m.Description)
	fmt.Printf("  repo        : %s\n", repo)
	fmt.Println()
}

func readManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read manifest.yaml at repo root: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest YAML: %w", err)
	}
	if m.Name == "" || m.Version == "" {
		return nil, fmt.Errorf("manifest missing required fields: name and version")
	}
	return &m, nil
} 