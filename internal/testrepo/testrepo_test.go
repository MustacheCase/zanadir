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
