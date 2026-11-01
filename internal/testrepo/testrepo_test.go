package testrepo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite(t *testing.T) {
	dir := Write(t, map[string]string{"a/b/c.yml": "x: 1\n", "top": ""})
	got, err := os.ReadFile(filepath.Join(dir, "a", "b", "c.yml"))
	if err != nil || string(got) != "x: 1\n" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "top")); err != nil {
		t.Fatal(err)
	}
}

func TestTouch(t *testing.T) {
	dir := Touch(t, "go.mod", "web/package.json")
	for _, rel := range []string{"go.mod", "web/package.json"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNeedsPermissions(t *testing.T) {
	defer func(was bool) { permissionsIgnored = was }(permissionsIgnored)
	for _, ignored := range []bool{false, true} {
		permissionsIgnored = ignored
		ran := false
		t.Run("", func(t *testing.T) { NeedsPermissions(t); ran = true })
		if ran == ignored {
			t.Errorf("with permissions ignored %v, the test ran: %v", ignored, ran)
		}
	}
}
