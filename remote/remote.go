// Package remote fetches a repository from its host so it can be scanned
// without a clone.
package remote

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Repository is a GitHub repository and, optionally, the ref to scan.
type Repository struct {
	Owner string
	Name  string
	// Ref is a branch, tag or commit; empty means the default branch.
	Ref string
}

func (r Repository) String() string {
	s := "github.com/" + r.Owner + "/" + r.Name
	if r.Ref != "" {
		s += "@" + r.Ref
	}
	return s
}

// GitHub owner names are letters, digits and hyphens; repository names may
// also hold dots and underscores.
var (
	ownerPart = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	namePart  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	refPart   = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

// Parse reads github.com/owner/name, with or without https:// and .git, or
// the owner/name shorthand, each optionally followed by @ref.
func Parse(s string) (Repository, error) {
	raw := s
	s = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "https://"), "http://")
	var repo Repository
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s, repo.Ref = s[:i], s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")

	parts := strings.Split(s, "/")
	if len(parts) == 3 {
		if !strings.EqualFold(parts[0], "github.com") {
			return Repository{}, fmt.Errorf("%q: only github.com repositories can be scanned without a clone", raw)
		}
		parts = parts[1:]
	}
	if len(parts) != 2 || !ownerPart.MatchString(parts[0]) || !namePart.MatchString(parts[1]) {
		return Repository{}, fmt.Errorf("%q is not a repository: expected github.com/owner/name", raw)
	}
	if repo.Ref != "" && (!refPart.MatchString(repo.Ref) || strings.Contains(repo.Ref, "..")) {
		return Repository{}, fmt.Errorf("%q is not a branch, tag or commit", repo.Ref)
	}
	repo.Owner, repo.Name = parts[0], parts[1]
	return repo, nil
}

// apiBase and client are replaced in tests.
var (
	apiBase = "https://api.github.com"
	client  = &http.Client{Timeout: 5 * time.Minute}
)

// Limits on what one archive may unpack to, since a tarball is untrusted
// input. Variables so tests can lower them.
var (
	maxFiles           = 200_000
	maxFileBytes int64 = 4 << 20
	maxKeptBytes int64 = 256 << 20
)

// Fetch downloads the repository into a new temporary directory, which the
// returned function removes. Only files zanadir reads keep their contents;
// every other file is an empty placeholder, since category gating and
// language detection look at names alone.
func Fetch(repo Repository, token string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "zanadir-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	// The scanner accepts git repositories only; the history is never read.
	err = os.Mkdir(filepath.Join(dir, ".git"), 0o755)
	if err == nil {
		err = download(repo, token, dir)
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

func download(repo Repository, token, dir string) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/tarball", apiBase, repo.Owner, repo.Name)
	if repo.Ref != "" {
		endpoint += "/" + (&url.URL{Path: repo.Ref}).EscapedPath()
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "zanadir")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", repo, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%s was not found: check the name, or set GITHUB_TOKEN if it is private", repo)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("GitHub refused %s (%s): the token may be invalid, or the rate limit for requests without GITHUB_TOKEN was reached", repo, resp.Status)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("downloading %s: %s", repo, resp.Status)
	}
	return extract(resp.Body, dir)
}

// keepContent reports whether zanadir reads a file's contents: CI
// configuration and the templates it includes, Jenkinsfiles, and the
// dependency bots' configuration.
func keepContent(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	switch filepath.Ext(base) {
	case ".yml", ".yaml", ".json", ".json5":
		return true
	}
	return strings.HasPrefix(base, "jenkinsfile") || base == ".renovaterc"
}

// extract unpacks a GitHub tarball, whose entries sit under one top-level
// directory, into dir. Only regular files and directories are written; an
// entry that would land outside dir is an error.
func extract(r io.Reader, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("reading the archive: %w", err)
	}
	archive := tar.NewReader(gz)

	files, kept := 0, int64(0)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading the archive: %w", err)
		}

		// A repository's own .git would collide with the marker Fetch made.
		_, name, _ := strings.Cut(header.Name, "/")
		if name == "" || name == ".git" || strings.HasPrefix(name, ".git/") {
			continue
		}
		target, err := within(dir, name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if files++; files > maxFiles {
				return fmt.Errorf("the archive has more than %d files", maxFiles)
			}
			var size int64
			if keepContent(name) && header.Size <= maxFileBytes {
				size = header.Size
			}
			if kept += size; kept > maxKeptBytes {
				return fmt.Errorf("the archive's configuration files exceed %d bytes", maxKeptBytes)
			}
			if err := writeFile(target, archive, size); err != nil {
				return err
			}
		}
	}
}

// within joins name onto dir, refusing a name that escapes it.
func within(dir, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("the archive has an entry outside the repository: %q", name)
	}
	return filepath.Join(dir, clean), nil
}

func writeFile(path string, r io.Reader, size int64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = io.CopyN(f, r, size)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
