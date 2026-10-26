package skills

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/MustacheCase/zanadir/internal/repofs"
)

// Categories of finding.
const (
	Quality = "Quality"
	Safety  = "Safety"
)

// Severities. An error is something certainly broken or hidden from a
// reviewer; a warning makes a skill less likely to work as meant, or riskier.
const (
	Error   = "error"
	Warning = "warning"
)

// Limits from the Agent Skills specification, and the size past which a
// SKILL.md should move detail into reference files.
const (
	maxNameLength        = 64
	maxDescriptionLength = 1024
	minDescriptionLength = 40
	maxBodyLines         = 500
	maxScannedFileBytes  = 1 << 20
	overlapThreshold     = 0.5
	minOverlapWords      = 6
	minTemplateGroup     = 8
)

// Finding is one problem in one skill.
type Finding struct {
	Rule     string `json:"rule"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Skill    string `json:"skill"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
}

var (
	namePattern    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	triggerPattern = regexp.MustCompile(`(?i)\b(used? (it |this( skill)? |them |only )?(when|for|to|if|with|whenever|before|after|on|in)|preferred (for|when|over)|when [a-z]+ing|when (the )?(user|you|working|asked|a |an )|whenever|trigger|invoke|applies when|for when|should be used)\b|\busage:`)
	bareBash       = regexp.MustCompile(`(^|[\s,\[])Bash(\(\s*:?\*?\s*\))?($|[\s,\]])`)
	linkPattern    = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	wordPattern    = regexp.MustCompile(`[a-z0-9]+`)
	inlineCode     = regexp.MustCompile("`[^`]*`")
)

type pattern struct {
	rule, message string
	re            *regexp.Regexp
	shellOnly     bool
}

var safetyPatterns = []pattern{
	{
		rule:    "pipe-to-shell",
		message: "downloads a script and pipes it straight into a shell",
		re:      regexp.MustCompile(`\b(curl|wget)\b[^|\n]*\b(https?://|[a-z0-9-]+\.[a-z]{2,}/)[^|\n]*\|\s*(sudo\s+)?(ba|z|da)?sh\b`),
	},
	{
		rule:    "skip-approvals",
		message: "runs the agent without approvals or outside its sandbox",
		re:      regexp.MustCompile(`--dangerously-skip-permissions|--dangerously-bypass-approvals-and-sandbox|\bbypassPermissions\b|\bdangerouslyDisableSandbox\b`),
		// Programs mention these settings mostly to check for them.
		shellOnly: true,
	},
}

var (
	shellExtensions   = map[string]bool{".sh": true, ".bash": true, ".zsh": true, ".ps1": true}
	programExtensions = map[string]bool{".py": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".rb": true, ".pl": true}
)

// runnable reports whether a file is one the agent follows or executes, not
// reference material that may describe a risky pattern, and whether it is
// instructions or a shell script rather than a program.
func runnable(file string) (run, shell bool) {
	base := path.Base(file)
	ext := strings.ToLower(path.Ext(base))
	shell = base == FileName || shellExtensions[ext] || base == "Dockerfile" || base == "Makefile"
	return shell || programExtensions[ext], shell
}

// Check runs every rule over the skills found under root.
func Check(root string, found []*Skill) []Finding {
	var findings []Finding
	for _, s := range found {
		findings = append(findings, checkFrontmatter(s)...)
		findings = append(findings, checkBody(root, s)...)
		findings = append(findings, checkFiles(root, s)...)
	}
	findings = append(findings, checkOverlap(found)...)
	slices.SortStableFunc(findings, func(a, b Finding) int {
		return strings.Compare(a.Skill, b.Skill)
	})
	return findings
}

func quality(s *Skill, rule, severity string, line int, format string, args ...any) Finding {
	return Finding{Rule: rule, Category: Quality, Severity: severity, Skill: s.Dir, File: s.Path, Line: line, Message: fmt.Sprintf(format, args...)}
}

func checkFrontmatter(s *Skill) []Finding {
	if !s.HasFrontmatter {
		return []Finding{quality(s, "frontmatter-missing", Warning, 1, "has no frontmatter, so agents cannot tell when to use it: start the file with a --- block naming it and describing when to use it")}
	}
	if s.FrontmatterErr != nil {
		return []Finding{quality(s, "frontmatter-invalid", Error, 1, "frontmatter is not valid: %v", s.FrontmatterErr)}
	}

	var findings []Finding
	switch {
	case s.Name == "":
		findings = append(findings, quality(s, "name-missing", Warning, 1, "frontmatter has no name"))
	case len(s.Name) > maxNameLength || !namePattern.MatchString(s.Name):
		findings = append(findings, quality(s, "name-format", Warning, s.line("name"), "name %q must be lowercase letters, digits and single hyphens, at most %d characters", s.Name, maxNameLength))
	case s.Dir != "." && s.Name != s.DirName():
		findings = append(findings, quality(s, "name-mismatch", Warning, s.line("name"), "name %q does not match its directory %q", s.Name, s.DirName()))
	}

	switch {
	case s.Description == "":
		findings = append(findings, quality(s, "description-missing", Warning, 1, "frontmatter has no description, which is what tells the agent when to use the skill"))
	case len(s.Description) > maxDescriptionLength:
		findings = append(findings, quality(s, "description-long", Warning, s.line("description"), "description is %d characters, over the Agent Skills limit of %d; agents may cut it short", len(s.Description), maxDescriptionLength))
	case len(s.Description) < minDescriptionLength:
		findings = append(findings, quality(s, "description-short", Warning, s.line("description"), "description is %d characters, too little for the agent to know when to use it", len(s.Description)))
	case latin(s.Description) && !triggerPattern.MatchString(s.Description):
		findings = append(findings, quality(s, "description-no-trigger", Warning, s.line("description"), "description says what the skill does but not when to use it; add a \"Use when ...\" sentence"))
	}

	if bareBash.MatchString(s.AllowedTools) {
		findings = append(findings, quality(s, "bash-unrestricted", Warning, s.line("allowed-tools"), "allowed-tools grants every shell command; scope it, as in Bash(git status:*)"))
	}
	return findings
}

func checkBody(root string, s *Skill) []Finding {
	var findings []Finding
	lines := strings.Split(s.Content, "\n")
	if n := len(strings.Split(strings.TrimRight(s.Body, "\n"), "\n")); n > maxBodyLines {
		findings = append(findings, quality(s, "body-long", Warning, 1, "body is %d lines and loads whole every time; move detail into files the skill links to", n))
	}

	fenced := false
	for i, line := range lines {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		for _, match := range linkPattern.FindAllStringSubmatch(inlineCode.ReplaceAllString(line, ""), -1) {
			if target, ok := localTarget(s.Dir, match[1]); ok && !exists(root, target) {
				findings = append(findings, quality(s, "missing-reference", Error, i+1, "links %s, which does not exist", match[1]))
			}
		}
	}
	return findings
}

// localTarget resolves a link to a path relative to the scanned directory.
// Links to the web, to anchors, and outside the directory are not checked.
func localTarget(dir, link string) (string, bool) {
	if strings.ContainsAny(link, "{}$<>") || strings.Contains(link, "://") || strings.HasPrefix(link, "#") || strings.HasPrefix(link, "/") || strings.Contains(link, ":") {
		return "", false
	}
	link, _, _ = strings.Cut(link, "#")
	link, _, _ = strings.Cut(link, "?")
	if decoded, err := url.PathUnescape(link); err == nil {
		link = decoded
	}
	if !strings.ContainsAny(link, "./") {
		return "", false
	}
	target := path.Clean(path.Join(dir, link))
	if target == ".." || strings.HasPrefix(target, "../") {
		return "", false
	}
	return target, true
}

func exists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// checkFiles looks for unsafe content in every text file of the skill. A
// skill at the root of the scan is only its SKILL.md and scripts.
func checkFiles(root string, s *Skill) []Finding {
	var findings []Finding
	for _, rel := range skillFiles(root, s) {
		data, err := repofs.ReadFile(root, filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || len(data) > maxScannedFileBytes || bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		findings = append(findings, scanText(s, rel, string(data))...)
	}
	return findings
}

func skillFiles(root string, s *Skill) []string {
	dirs := []string{s.Dir}
	files := []string{}
	if s.Dir == "." {
		dirs, files = []string{"scripts"}, []string{s.Path}
	}
	for _, d := range dirs {
		base := filepath.Join(root, filepath.FromSlash(d))
		_ = filepath.WalkDir(base, func(p string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if p != base && entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
	}
	return files
}

func scanText(s *Skill, file, text string) []Finding {
	var findings []Finding
	run, shell := runnable(file)
	instructions := path.Base(file) == FileName
	for i, line := range strings.Split(text, "\n") {
		for _, p := range safetyPatterns {
			if run && (shell || !p.shellOnly) && p.re.MatchString(line) {
				findings = append(findings, safety(s, p.rule, Warning, file, i+1, p.message))
			}
		}
		if r, smuggling, ok := hiddenRune(line, i == 0); ok && (smuggling || instructions) {
			severity := Warning
			if smuggling {
				severity = Error
			}
			findings = append(findings, safety(s, "hidden-unicode", severity, file, i+1, fmt.Sprintf("contains invisible character U+%04X, which can hide text from a reviewer", r)))
		}
	}
	return findings
}

func safety(s *Skill, rule, severity, file string, line int, message string) Finding {
	return Finding{Rule: rule, Category: Safety, Severity: severity, Skill: s.Dir, File: file, Line: line, Message: message}
}

// hiddenRune finds an invisible character. Bidirectional overrides and tag
// characters can smuggle text past a reviewer; zero-width ones are often
// harmless. A byte order mark opening the file, and a zero-width joiner
// inside an emoji sequence, are allowed.
func hiddenRune(line string, first bool) (r rune, smuggling, found bool) {
	var prev rune
	for i, r := range line {
		switch {
		case r >= '\u202A' && r <= '\u202E', r >= '\u2066' && r <= '\u2069', r >= 0xE0000 && r <= 0xE007F:
			return r, true, true
		case r == '\uFEFF' && first && i == 0:
		case r == '\u200D':
			next, _ := utf8.DecodeRuneInString(line[i+utf8.RuneLen(r):])
			if prev < 0x2000 || next < 0x2000 {
				return r, false, true
			}
		case r == '\u200B' || r == '\u200C' || r == '\u2060' || r == '\uFEFF':
			return r, false, true
		}
		prev = r
	}
	return 0, false, false
}

var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "use": true, "when": true, "with": true, "this": true, "that": true,
	"user": true, "users": true, "asks": true, "wants": true, "skill": true, "from": true, "into": true, "your": true,
	"you": true, "are": true, "any": true, "such": true, "like": true, "also": true, "its": true, "their": true,
	"them": true, "about": true, "need": true, "needs": true, "using": true, "used": true, "should": true, "not": true,
	"can": true, "will": true, "how": true, "what": true, "which": true, "who": true, "all": true, "other": true,
}

func words(text string) map[string]bool {
	set := map[string]bool{}
	for _, w := range wordPattern.FindAllString(strings.ToLower(text), -1) {
		if len(w) >= 3 && !stopWords[w] {
			set[w] = true
		}
	}
	return set
}

// checkOverlap flags pairs of skills one agent loads together whose
// descriptions share most of their words, so it cannot tell which a request
// is for.
func checkOverlap(found []*Skill) []Finding {
	var findings []Finding
	sets := make([]map[string]bool, len(found))
	for i, s := range found {
		sets[i] = words(s.Description)
	}
	dropTemplateWords(found, sets)
	for i := range found {
		for j := i + 1; j < len(found); j++ {
			if found[i].scope() != found[j].scope() || !latin(found[i].Description) || !latin(found[j].Description) || len(sets[i]) < minOverlapWords || len(sets[j]) < minOverlapWords {
				continue
			}
			if score := jaccard(sets[i], sets[j]); score >= overlapThreshold {
				a, b := found[i], found[j]
				findings = append(findings, quality(a, "description-overlap", Warning, a.line("description"), "description shares %.0f%% of its words with %s; the agent may pick the wrong one", score*100, b.Dir))
			}
		}
	}
	return findings
}

// dropTemplateWords removes, within each scope of many skills, the words
// most descriptions share: a generated collection repeats one template, and
// what tells its skills apart is the rest.
func dropTemplateWords(found []*Skill, sets []map[string]bool) {
	members := map[string][]int{}
	for i, s := range found {
		members[s.scope()] = append(members[s.scope()], i)
	}
	for _, group := range members {
		if len(group) < minTemplateGroup {
			continue
		}
		frequency := map[string]int{}
		for _, i := range group {
			for w := range sets[i] {
				frequency[w]++
			}
		}
		for _, i := range group {
			for w := range sets[i] {
				if frequency[w]*4 > len(group) {
					delete(sets[i], w)
				}
			}
		}
	}
}

func jaccard(a, b map[string]bool) float64 {
	shared := 0
	for w := range a {
		if b[w] {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

// latin reports whether text is mostly ASCII, the only language the trigger
// phrases are written in.
func latin(text string) bool {
	ascii := 0
	for _, r := range text {
		if r < utf8.RuneSelf {
			ascii++
		}
	}
	return ascii*2 > utf8.RuneCountInString(text)
}
