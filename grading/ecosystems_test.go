package grading

import (
	"reflect"
	"slices"
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

func TestUncoveredEcosystems(t *testing.T) {
	goTool := Tool{Name: "govulncheck", Verdict: Enforcing, Locations: []string{"go.yml"}, Ecosystems: []string{"Go"}}
	gap := "govulncheck checks only Go; Python dependencies detected from repository manifests have no enforcing SCA check"
	tests := []struct {
		name      string
		tools     []Tool
		languages []string
		verdict   Verdict
		reason    string
	}{
		{"single covered language", []Tool{goTool}, []string{"Go"}, Enforcing, ""},
		{"extra language gap", []Tool{goTool}, []string{"Go", "Python"}, Partial, gap},
		{"complementary scanners", []Tool{goTool, {Name: "pip-audit", Verdict: Enforcing, Ecosystems: []string{"Python"}}}, []string{"Go", "Python"}, Enforcing, ""},
		{"unrestricted enforcing scanner", []Tool{goTool, {Name: "multi", Verdict: Enforcing}}, []string{"Go", "Python"}, Enforcing, ""},
		{"unrestricted advisory scanner", []Tool{goTool, {Name: "multi", Verdict: Advisory}}, []string{"Go", "Python"}, Partial, gap + " (multi covers them but cannot block a merge)"},
		{"unrestricted partial scanner", []Tool{goTool, {Name: "multi", Verdict: Partial}}, []string{"Go", "Python"}, Partial, gap + " (multi covers them but cannot block a merge)"},
		{"scoped advisory scanner", []Tool{goTool, {Name: "pip-audit", Verdict: Advisory, Ecosystems: []string{"Python"}}}, []string{"Go", "Python"}, Partial, gap + " (pip-audit covers them but cannot block a merge)"},
		{"scoped partial scanner", []Tool{goTool, {Name: "pip-audit", Verdict: Partial, Ecosystems: []string{"Python"}}}, []string{"Go", "Python"}, Partial, gap + " (pip-audit covers them but cannot block a merge)"},
		{"unrelated weaker scanner", []Tool{goTool, {Name: "cargo-audit", Verdict: Advisory, Ecosystems: []string{"Rust"}}}, []string{"Go", "Python"}, Partial, gap},
		{"unknown scoped scanner", []Tool{goTool, {Name: "pip-audit", Verdict: Unknown, Ecosystems: []string{"Python"}}}, []string{"Go", "Python"}, Partial, gap},
		{"multiple weaker scanners", []Tool{goTool, {Name: "Renovate", Verdict: Advisory}, {Name: "Dependabot", Verdict: Advisory}}, []string{"Go", "Python"}, Partial, gap + " (Dependabot covers them but cannot block a merge; Renovate covers them but cannot block a merge)"},
		{"weaker scanner covers only part of gap", []Tool{goTool, {Name: "pip-audit", Verdict: Advisory, Ecosystems: []string{"Python"}}}, []string{"Go", "Python", "Ruby"}, Partial, "govulncheck checks only Go; Python, Ruby dependencies detected from repository manifests have no enforcing SCA check (pip-audit covers Python dependencies but cannot block a merge)"},
		{"unrestricted unknown scanner", []Tool{goTool, {Name: "multi", Verdict: Unknown}}, []string{"Go", "Python"}, Partial, gap},
		{"empty languages", []Tool{goTool}, nil, Enforcing, ""},
		{"missing tool metadata", nil, []string{"Python"}, Enforcing, ""},
		{"stable deduplicated components", []Tool{{Name: "z", Verdict: Enforcing, Ecosystems: []string{"Python", "Go", "Go"}}, {Name: "a", Verdict: Enforcing, Ecosystems: []string{"Go"}}, {Name: "a", Verdict: Enforcing, Ecosystems: []string{"Go"}}}, []string{"Ruby", "Java", "Ruby"}, Partial, "a, z check only Go, Python; Java, Ruby dependencies detected from repository manifests have no enforcing SCA check"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := Assessment{Verdict: Enforcing, Location: "ci.yml", Tools: tt.tools}
			got := UncoveredEcosystems(in, tt.languages)
			want := in
			want.Verdict, want.Reason = tt.verdict, tt.reason
			if !reflect.DeepEqual(got, want) {
				t.Errorf("UncoveredEcosystems() = %+v, want %+v", got, want)
			}
			reordered := in
			reordered.Tools = slices.Clone(in.Tools)
			slices.Reverse(reordered.Tools)
			languages := slices.Clone(tt.languages)
			slices.Reverse(languages)
			if reversed := UncoveredEcosystems(reordered, languages); reversed.Verdict != got.Verdict || reversed.Reason != got.Reason {
				t.Errorf("reordering changed the verdict or reason: %+v", reversed)
			}
		})
	}
}

func TestUncoveredEcosystemsPreservesWeakerVerdicts(t *testing.T) {
	for _, verdict := range []Verdict{Partial, Advisory, Unknown} {
		t.Run(string(verdict), func(t *testing.T) {
			in := Assessment{Verdict: verdict, Reason: "existing reason", Location: "ci.yml", Tools: []Tool{{Name: "go", Verdict: Enforcing, Ecosystems: []string{"Go"}}}}
			if got := UncoveredEcosystems(in, []string{"Python"}); !reflect.DeepEqual(got, in) {
				t.Errorf("assessment changed: %+v", got)
			}
		})
	}
}

func ecosystemFinding(name string, ecosystems []string, verdict Verdict) *matcher.Finding {
	f := finding([]models.Trigger{{Event: "pull_request"}}, &models.Job{Check: name})
	f.Tool, f.Category, f.Location = name, string(models.SCA), name+".yml"
	f.Rule = &rules.Rule{Ecosystems: ecosystems}
	switch verdict {
	case Advisory:
		f.Job.ContinueOnError = true
	case Partial:
		f.Artifact.Triggers = []models.Trigger{{Event: "push"}}
	case Unknown:
		f.Job.If = "condition"
	}
	return f
}

func TestByCategoryKeepsStrongestEcosystemScopes(t *testing.T) {
	tests := []struct {
		name     string
		findings []*matcher.Finding
		want     []string
		verdict  Verdict
	}{
		{"stronger resets scopes", []*matcher.Finding{ecosystemFinding("scan", []string{"Python"}, Advisory), ecosystemFinding("SCAN", []string{"Go"}, Enforcing)}, []string{"Go"}, Enforcing},
		{"weaker cannot expand", []*matcher.Finding{ecosystemFinding("scan", []string{"Go"}, Enforcing), ecosystemFinding("SCAN", []string{"Python"}, Advisory)}, []string{"Go"}, Enforcing},
		{"equal unions", []*matcher.Finding{ecosystemFinding("scan", []string{"Python"}, Enforcing), ecosystemFinding("SCAN", []string{"Go", "Go"}, Enforcing)}, []string{"Go", "Python"}, Enforcing},
		{"equal empty dominates", []*matcher.Finding{ecosystemFinding("scan", []string{"Go"}, Enforcing), ecosystemFinding("SCAN", nil, Enforcing)}, nil, Enforcing},
		{"equal scoped cannot narrow empty", []*matcher.Finding{ecosystemFinding("scan", nil, Enforcing), ecosystemFinding("SCAN", []string{"Go"}, Enforcing)}, nil, Enforcing},
		{"stronger scoped replaces empty", []*matcher.Finding{ecosystemFinding("scan", nil, Advisory), ecosystemFinding("SCAN", []string{"Go"}, Enforcing)}, []string{"Go"}, Enforcing},
		{"weaker empty cannot expand", []*matcher.Finding{ecosystemFinding("scan", []string{"Go"}, Enforcing), ecosystemFinding("SCAN", nil, Advisory)}, []string{"Go"}, Enforcing},
		{"stronger empty replaces scoped", []*matcher.Finding{ecosystemFinding("scan", []string{"Go"}, Advisory), ecosystemFinding("SCAN", nil, Enforcing)}, nil, Enforcing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, findings := range [][]*matcher.Finding{tt.findings, {tt.findings[1], tt.findings[0]}} {
				got := ByCategory(findings, nil)[string(models.SCA)]
				if len(got.Tools) != 1 || got.Tools[0].Verdict != tt.verdict || !reflect.DeepEqual(got.Tools[0].Ecosystems, tt.want) {
					t.Errorf("tools = %+v, want verdict %s scope %v", got.Tools, tt.verdict, tt.want)
				}
			}
		})
	}
}

func TestEcosystemScopesDoNotMutateInputs(t *testing.T) {
	scope := []string{"Python", "Go", "Go"}
	languages := []string{"Ruby", "Java", "Ruby"}
	f := ecosystemFinding("scan", scope, Enforcing)
	a := ByCategory([]*matcher.Finding{f}, nil)[string(models.SCA)]
	before := []string{"Go", "Python"}
	_ = UncoveredEcosystems(a, languages)
	if !reflect.DeepEqual(scope, []string{"Python", "Go", "Go"}) || !reflect.DeepEqual(languages, []string{"Ruby", "Java", "Ruby"}) || !reflect.DeepEqual(a.Tools[0].Ecosystems, before) {
		t.Fatal("grading modified input slices")
	}
	a.Tools[0].Ecosystems[0] = "changed"
	if scope[0] != "Python" || scope[1] != "Go" {
		t.Fatal("tool scope aliases the rule catalogue")
	}
}

func TestEcosystemCoverageUsesFinalGatedVerdicts(t *testing.T) {
	tests := []struct {
		name     string
		merge    *Merge
		disabled bool
		want     Verdict
		reason   string
	}{
		{"required scoped scanner leaves gap", &Merge{Required: func(job *models.Job) bool { return job.Check == "go" }}, false, Partial, "go checks only Go; Python dependencies detected from repository manifests have no enforcing SCA check (other covers them but cannot block a merge)"},
		{"both required cover languages", &Merge{Required: func(*models.Job) bool { return true }}, false, Enforcing, ""},
		{"disabled broad scanner leaves gap", &Merge{Required: func(*models.Job) bool { return true }, Scanned: func(*models.Job) bool { return true }}, true, Partial, "go checks only Go; Python dependencies detected from repository manifests have no enforcing SCA check (other covers them but cannot block a merge)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goScan := ecosystemFinding("go", []string{"Go"}, Enforcing)
			other := ecosystemFinding("other", []string{"Python"}, Enforcing)
			other.Artifact.Disabled = tt.disabled
			if tt.disabled {
				other.Rule.Ecosystems = nil
			}
			a := ByCategory([]*matcher.Finding{goScan, other}, tt.merge)[string(models.SCA)]
			got := UncoveredEcosystems(a, []string{"Go", "Python"})
			if got.Verdict != tt.want || got.Reason != tt.reason || !reflect.DeepEqual(got.Tools, a.Tools) || got.Location != a.Location {
				t.Errorf("assessment = %+v, want %s (%s) with unchanged evidence", got, tt.want, tt.reason)
			}
		})
	}
}

func TestEcosystemMetadataCompatibility(t *testing.T) {
	scoped := ecosystemFinding("go", []string{"Go"}, Enforcing)
	unscoped := ecosystemFinding("legacy", nil, Enforcing)
	unscoped.Rule = nil
	a := ByCategory([]*matcher.Finding{nil, scoped, unscoped}, nil)[string(models.SCA)]
	if got := UncoveredEcosystems(a, []string{"Python"}); !reflect.DeepEqual(got, a) {
		t.Errorf("legacy rule metadata changed grading: %+v", got)
	}
	scoped.Category = string(models.SAST)
	if got := ByCategory([]*matcher.Finding{scoped}, nil)[string(models.SAST)]; len(got.Tools[0].Ecosystems) != 0 {
		t.Errorf("non-SCA tool acquired scopes: %+v", got.Tools)
	}
	unaudited := Unaudited(a, "uv.lock", []string{"package"})
	if got := UncoveredEcosystems(unaudited, []string{"Python"}); !reflect.DeepEqual(got, unaudited) {
		t.Errorf("unaudited reason changed: %+v", got)
	}
}
