package compat

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// readModuleFile reads a file out of a module version in the local module
// cache, downloading it if absent. It is how a fixture inspects the source of
// a DEPLOYED dependency version rather than the one in this workspace.
func readModuleFile(t *testing.T, module, rel string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("go", "mod", "download", "-json", module)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	dir := jsonField(string(out), "Dir")
	if dir == "" {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(dir, rel))
}

// jsonField pulls one string field out of `go mod download -json` output
// without pulling in a JSON decode of the whole record.
func jsonField(s, key string) string {
	marker := `"` + key + `": "`
	i := strings.Index(s, marker)
	if i < 0 {
		return ""
	}
	rest := s[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return strings.ReplaceAll(rest[:j], `\\`, `\`)
}
