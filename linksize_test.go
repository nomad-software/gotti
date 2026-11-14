package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The linker keeps only the qute methods termite reaches. One call of
// reflect's Method or MethodByName with a computed name anywhere in the
// program (text/template and html/template make them) would make it keep
// every exported method of every type: termite calls no Override setter, so
// any in the binary means that happened, and the stripped binary roughly
// doubles (22.9 MB against 45.2 MB with qute v1.0.21, which made one).
func TestLinkerDropsUnusedMethods(t *testing.T) {
	if testing.Short() {
		t.Skip("builds termite")
	}
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command:", err)
	}
	bin := filepath.Join(t.TempDir(), "termite")
	if out, err := exec.Command(gobin, "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(gobin, "tool", "nm", bin).Output()
	if err != nil {
		t.Fatalf("go tool nm: %v", err)
	}
	var kept []string
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		if name := f[len(f)-1]; strings.HasPrefix(name, "github.com/nomad-software/qute/") && strings.Contains(name, ").Override") {
			kept = append(kept, name)
		}
	}
	if len(kept) > 0 {
		t.Errorf("termite links %d qute Override setters it never calls (first: %s); something calls reflect's Method or MethodByName with a computed name",
			len(kept), kept[0])
	}
}
