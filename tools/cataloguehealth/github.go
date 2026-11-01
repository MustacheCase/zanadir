package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// apiBase and client are replaced in tests.
var (
	apiBase = "https://api.github.com"
	client  = &http.Client{Timeout: time.Minute}
)

type repository struct {
	Name     string
	Missing  bool
	Archived bool
	Pushed   time.Time
	// Released is the later of the latest release and the newest version
	// tag: many actions tag without publishing releases.
	Released time.Time
	Tags     map[string]bool
	Newest   []int
}

// github reads each repository once.
type github struct {
	token string
	repos map[string]*repository
}

func (gh *github) repository(name string) (*repository, error) {
	if repo, ok := gh.repos[name]; ok {
		return repo, nil
	}
	repo := &repository{Name: name}
	var info struct {
		Archived bool      `json:"archived"`
		Pushed   time.Time `json:"pushed_at"`
	}
	found, err := gh.get("/repos/"+name, &info)
	if err != nil {
		return nil, err
	}
	repo.Missing, repo.Archived, repo.Pushed = !found, info.Archived, info.Pushed
	if found {
		if err := gh.releases(repo); err != nil {
			return nil, err
		}
	}
	gh.repos[name] = repo
	return repo, nil
}

// commitOf returns the commit a ref points at, through an annotated tag.
func (gh *github) commitOf(name, ref string) (string, error) {
	var commit struct {
		SHA string `json:"sha"`
	}
	_, err := gh.get("/repos/"+name+"/commits/"+ref, &commit)
	return commit.SHA, err
}

func (gh *github) releases(repo *repository) error {
	var release struct {
		Published time.Time `json:"published_at"`
	}
	if _, err := gh.get("/repos/"+repo.Name+"/releases/latest", &release); err != nil {
		return err
	}
	repo.Released = release.Published

	var refs []struct {
		Ref string `json:"ref"`
	}
	if _, err := gh.get("/repos/"+repo.Name+"/git/matching-refs/tags", &refs); err != nil {
		return err
	}
	repo.Tags = map[string]bool{}
	newest := ""
	for _, ref := range refs {
		tag := strings.TrimPrefix(ref.Ref, "refs/tags/")
		repo.Tags[tag] = true
		if version := versionOf(tag); version != nil && slices.Compare(version, repo.Newest) > 0 {
			newest, repo.Newest = tag, version
		}
	}
	if newest == "" {
		return nil
	}
	var commit struct {
		Commit struct {
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if _, err := gh.get("/repos/"+repo.Name+"/commits/"+newest, &commit); err != nil {
		return err
	}
	if tagged := commit.Commit.Committer.Date; tagged.After(repo.Released) {
		repo.Released = tagged
	}
	return nil
}

// get reads one resource, reporting whether it exists.
func (gh *github) get(path string, out any) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, apiBase+path, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "zanadir-cataloguehealth")
	if gh.token != "" {
		req.Header.Set("Authorization", "Bearer "+gh.token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, json.NewDecoder(resp.Body).Decode(out)
	case http.StatusNotFound:
		return false, nil
	}
	return false, fmt.Errorf("GitHub answered %s for %s", resp.Status, path)
}
