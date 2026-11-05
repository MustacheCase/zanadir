package skills

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
)

// HomeDirs are where agents load a user's own skills, relative to the home
// directory: Claude Code, Codex (.codex/skills in older versions) and Cursor,
// which also reads Claude's and Codex's.
var HomeDirs = []string{".claude/skills", ".agents/skills", ".codex/skills", ".cursor/skills"}

// maxHomeDepth bounds the category folders searched below a skills directory.
const maxHomeDepth = 3

// RunHome finds and checks the skills under home's HomeDirs.
func RunHome(home string) Report {
	found := DiscoverHome(home)
	findings := Check(home, found)
	if findings == nil {
		findings = []Finding{}
	}
	return Report{Skills: Names(found), Findings: findings}
}

// DiscoverHome finds the skills under home's HomeDirs, shown as ~/.claude/...
// Agents follow a symlinked skill directory, so it is read where it points;
// a skill reached twice is reported once.
func DiscoverHome(home string) []*Skill {
	seen := map[string]bool{}
	var found []*Skill
	for _, rel := range HomeDirs {
		dir, shown := filepath.Join(home, filepath.FromSlash(rel)), "~/"+rel
		if config := os.Getenv("CLAUDE_CONFIG_DIR"); rel == ".claude/skills" && config != "" {
			dir, shown = filepath.Join(config, "skills"), "$CLAUDE_CONFIG_DIR/skills"
		}
		found = append(found, walkHome(dir, shown, 0, seen)...)
	}
	return found
}

func walkHome(dir, shown string, depth int, seen map[string]bool) []*Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var found []*Skill
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || skipped(name) {
			continue
		}
		real, err := filepath.EvalSymlinks(filepath.Join(dir, name))
		if err != nil || seen[real] {
			continue
		}
		if info, err := os.Stat(real); err != nil || !info.IsDir() {
			continue
		}
		seen[real] = true
		if data, err := repofs.ReadFile(real, filepath.Join(real, FileName)); err == nil {
			s := Parse(shown+"/"+name+"/"+FileName, data)
			s.root, s.src = filepath.Dir(real), filepath.Base(real)
			found = append(found, s)
		} else if depth < maxHomeDepth {
			found = append(found, walkHome(real, shown+"/"+name, depth+1, seen)...)
		}
	}
	return found
}
