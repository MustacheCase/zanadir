// Package remote fetches a repository from its host so it can be scanned
// without a clone.
package remote

import (
	"archive/tar"
	"bytes"
	"cmp"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/MustacheCase/zanadir/utils"
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

// Token is the GitHub token from the environment, if any: GITHUB_TOKEN, as
// in Actions, or GH_TOKEN, as the gh CLI sets it.
func Token() string {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("GH_TOKEN")
}

// apiBase, rawBase and client are replaced in tests.
var (
	apiBase = "https://api.github.com"
	rawBase = "https://raw.githubusercontent.com"
	client  = &http.Client{Timeout: 5 * time.Minute}
)

// Limits on what one archive may unpack to, since a tarball is untrusted
// input. Variables so tests can lower them.
var (
	maxFiles           = 200_000
	maxFileBytes int64 = 4 << 20
	maxKeptBytes int64 = 256 << 20
	maxHeadBytes int64 = 64 << 20
)

// headBytes is as much of a JSON file as IaC detection reads.
const headBytes = 32 << 10

const tooLarge = "clone it and scan with --dir"

type content int

const (
	placeholder content = iota
	head
	whole
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
		recordDefaultBranch(repo, token, dir)
		err = download(repo, token, dir)
	}
	if err == nil {
		err = restoreExportIgnored(repo, token, dir)
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

// recordDefaultBranch writes origin/HEAD as git clone would, so grading reads
// the default branch of a fetched repository as it does a checkout's. A
// failure leaves it unset, and grading falls back to common names.
func recordDefaultBranch(repo Repository, token, dir string) {
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if getJSON(fmt.Sprintf("%s/repos/%s/%s", apiBase, repo.Owner, repo.Name), token, &info) != nil || info.DefaultBranch == "" {
		return
	}
	head := "ref: refs/remotes/origin/" + info.DefaultBranch + "\n"
	_ = writeFile(filepath.Join(dir, ".git", "refs", "remotes", "origin", "HEAD"), strings.NewReader(head), int64(len(head)))
}

func download(repo Repository, token, dir string) error {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/tarball", apiBase, repo.Owner, repo.Name)
	if repo.Ref != "" {
		endpoint += "/" + (&url.URL{Path: repo.Ref}).EscapedPath()
	}
	resp, err := get(endpoint, token)
	if err != nil {
		return downloadFailed(repo, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnprocessableEntity:
		return fmt.Errorf("GitHub could not produce an archive of %s (%s), which usually means the repository is too large; %s", repo, resp.Status, tooLarge)
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%s was not found: check the name, or set GITHUB_TOKEN if it is private", repo)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("GitHub refused %s (%s): the token may be invalid, or the rate limit for requests without GITHUB_TOKEN was reached", repo, resp.Status)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("downloading %s: %s", repo, resp.Status)
	}
	if err := extract(resp.Body, dir); err != nil {
		return downloadFailed(repo, err)
	}
	return nil
}

func downloadFailed(repo Repository, err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("downloading %s took longer than %s, which usually means the repository is too large; %s", repo, client.Timeout, tooLarge)
	}
	return fmt.Errorf("downloading %s: %w", repo, err)
}

func get(endpoint, token string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "zanadir")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return client.Do(req)
}

// restoreExportIgnored puts back what the archive left out. GitHub builds it
// with git archive, which drops every path .gitattributes marks
// export-ignore; projects often mark .github that way, and the archive alone
// then reads as a repository with no CI. .gitattributes often leaves itself
// out too, so it is fetched when the archive lacks it. The tree lists every
// file, so each missing one is written: with its contents if zanadir reads
// them, as a placeholder otherwise.
func restoreExportIgnored(repo Repository, token, dir string) error {
	ref := (&url.URL{Path: cmp.Or(repo.Ref, "HEAD")}).EscapedPath()
	raw := func(path string) string {
		return fmt.Sprintf("%s/%s/%s/%s/%s", rawBase, repo.Owner, repo.Name, ref, (&url.URL{Path: path}).EscapedPath())
	}
	attributes, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	if err != nil {
		attributes, err = fetchAttributes(raw(".gitattributes"), token)
	}
	if err != nil || !bytes.Contains(attributes, []byte("export-ignore")) {
		return err
	}
	failed := func(err error) error {
		return fmt.Errorf("%s leaves files out of its archive (export-ignore), and fetching them failed: %w; "+
			"set GITHUB_TOKEN, or scan a checkout with --dir", repo, err)
	}

	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"tree"`
	}
	if err := getJSON(fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", apiBase, repo.Owner, repo.Name, ref), token, &tree); err != nil {
		return failed(err)
	}
	files, kept := 0, int64(0)
	for _, entry := range tree.Tree {
		target, err := within(dir, entry.Path)
		if err != nil {
			return err
		}
		if entry.Type != "blob" || exists(target) {
			continue
		}
		if files++; files > maxFiles {
			return fmt.Errorf("the repository has more than %d files; %s", maxFiles, tooLarge)
		}
		// A project that leaves its tests out of the archive has thousands of
		// fixtures, so none is fetched.
		if contentOf(entry.Path) != whole || entry.Size > maxFileBytes || underTests(entry.Path) {
			err = writeFile(target, nil, 0)
		} else if kept += entry.Size; kept > maxKeptBytes {
			return fmt.Errorf("the repository's configuration files exceed %d bytes; %s", maxKeptBytes, tooLarge)
		} else {
			err = fetchFile(raw(entry.Path), token, target, entry.Size)
		}
		if err != nil {
			return failed(err)
		}
	}
	return nil
}

func getJSON(endpoint, token string, out any) error {
	resp, err := get(endpoint, token)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func fetchFile(endpoint, token, target string, size int64) error {
	resp, err := get(endpoint, token)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %s for %s", resp.Status, endpoint)
	}
	return writeFile(target, resp.Body, size)
}

// fetchAttributes reads a .gitattributes the archive left out; a repository
// without one has nothing to restore.
func fetchAttributes(endpoint, token string) ([]byte, error) {
	resp, err := get(endpoint, token)
	if err != nil {
		return nil, fmt.Errorf("checking for files the archive leaves out: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return io.ReadAll(io.LimitReader(resp.Body, maxFileBytes))
	case http.StatusNotFound:
		return nil, nil
	}
	return nil, fmt.Errorf("checking for files the archive leaves out: GitHub answered %s", resp.Status)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// contentOf says how much of a file zanadir reads: CI and build files
// whole, the head of other JSON for IaC detection, else only the name.
func contentOf(name string) content {
	if keepContent(name) {
		return whole
	}
	if ext := strings.ToLower(filepath.Ext(name)); ext == ".json" || ext == ".json5" {
		return head
	}
	return placeholder
}

func keepContent(name string) bool {
	if slices.Contains(strings.Split(filepath.ToSlash(name), "/"), ".make") {
		return true
	}
	base := strings.ToLower(filepath.Base(name))
	switch filepath.Ext(base) {
	case ".yml", ".yaml", ".mk":
		return true
	}
	switch base {
	case ".renovaterc", ".renovaterc.json", ".renovaterc.json5", "renovate.json", "renovate.json5", "tox.ini", ".gitattributes":
		return true
	}
	// Makefile.common and the like are included by the Makefile.
	return strings.HasPrefix(base, "jenkinsfile") || strings.HasPrefix(base, "makefile") || strings.HasPrefix(base, "gnumakefile")
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

	files, kept, heads := 0, int64(0), int64(0)
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
				return fmt.Errorf("the archive has more than %d files; %s", maxFiles, tooLarge)
			}
			// A repository of JSON data runs out of heads rather than failing.
			var size int64
			switch contentOf(name) {
			case whole:
				if header.Size <= maxFileBytes {
					size = header.Size
				}
				if kept += size; kept > maxKeptBytes {
					return fmt.Errorf("the archive's configuration files exceed %d bytes; %s", maxKeptBytes, tooLarge)
				}
			case head:
				if heads < maxHeadBytes {
					size = min(header.Size, headBytes)
					heads += size
				}
			}
			if err := writeFile(target, archive, size); err != nil {
				return err
			}
		}
	}
}

func underTests(name string) bool {
	return slices.ContainsFunc(strings.Split(name, "/"), utils.IsTestDir)
}

// within joins name onto dir, refusing a name that is absolute or resolves
// outside it.
func within(dir, name string) (string, error) {
	target := filepath.Join(dir, filepath.FromSlash(name))
	if filepath.IsAbs(filepath.FromSlash(name)) || !strings.HasPrefix(target, filepath.Clean(dir)+string(filepath.Separator)) {
		return "", fmt.Errorf("the archive has an entry outside the repository: %q", name)
	}
	return target, nil
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
