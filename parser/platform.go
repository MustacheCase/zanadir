package parser

// Platform IDs, as the fix templates name them.
const (
	GitHub    = "github"
	GitLab    = "gitlab"
	CircleCI  = "circleci"
	Bitbucket = "bitbucket"
	Azure     = "azure"
	Jenkins   = "jenkins"
)

// Platform is a CI system zanadir reads. Each one is described here and
// nowhere else; the scanner, fix and the applicability check all read this.
type Platform struct {
	ID     string
	Parser Parser
	// Target is the file fix tells the user to paste a snippet into.
	Target string
}

// Platforms is in detection order: fix writes for the first one a repository
// has.
var Platforms = []Platform{
	{ID: GitHub, Parser: NewGithubParser(), Target: ".github/workflows/security.yml"},
	{ID: GitLab, Parser: NewGitlabParser(), Target: gitlabFile},
	{ID: CircleCI, Parser: NewCircleCIParser(), Target: ".circleci/config.yml"},
	{ID: Bitbucket, Parser: NewBitbucketParser(), Target: bitbucketFile},
	{ID: Azure, Parser: NewAzureParser(), Target: azureFiles[0]},
	{ID: Jenkins, Parser: NewJenkinsParser(), Target: jenkinsFile},
}

// Detect returns the platform a repository uses, or GitHub Actions when it
// has none of them.
func Detect(root string) Platform {
	for _, p := range Platforms {
		if p.Parser.Exists(root) {
			return p
		}
	}
	return Platforms[0]
}

// Lookup returns the platform with this ID.
func Lookup(id string) (Platform, bool) {
	for _, p := range Platforms {
		if p.ID == id {
			return p, true
		}
	}
	return Platform{}, false
}

// Uses reports whether a repository has configuration for platform id.
func Uses(root, id string) bool {
	p, ok := Lookup(id)
	return ok && p.Parser.Exists(root)
}

// Found lists the IDs of every platform a repository has configuration for,
// in detection order.
func Found(root string) []string {
	found := []string{}
	for _, p := range Platforms {
		if p.Parser.Exists(root) {
			found = append(found, p.ID)
		}
	}
	return found
}
