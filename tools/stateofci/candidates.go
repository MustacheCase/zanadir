package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type candidate struct {
	FullName string `json:"full_name"`
	Stars    int    `json:"stargazers_count"`
}

// apiBase, client and pause are replaced in tests.
var (
	apiBase = "https://api.github.com"
	client  = &http.Client{Timeout: time.Minute}
	// Search allows 30 requests a minute with a token.
	pause = 2100 * time.Millisecond
	// rateLimitFloor is the least a refused search waits before retrying.
	rateLimitFloor = time.Second
)

// loadCandidates reads the saved candidate list, or lists twice n of the
// most-starred repositories and saves them: many of the most-starred are
// lists, books or docs with no CI at all.
func loadCandidates(path string, n int, token string) ([]candidate, error) {
	if data, err := os.ReadFile(path); err == nil {
		var saved []candidate
		return saved, json.Unmarshal(data, &saved)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	found, err := searchMostStarred(2*n, token)
	if err != nil {
		return nil, err
	}
	return found, writeJSON(path, found)
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

// searchMostStarred lists active, non-fork repositories by stars. Search
// returns at most 1,000 results per query, so each query continues below the
// lowest star count the last one reached.
func searchMostStarred(want int, token string) ([]candidate, error) {
	var found []candidate
	seen := map[string]bool{}
	ceiling := 0
	for len(found) < want {
		query := "archived:false fork:false stars:>=1"
		if ceiling > 0 {
			query = fmt.Sprintf("archived:false fork:false stars:1..%d", ceiling)
		}
		lowest := ceiling
		for page := 1; page <= 10 && len(found) < want; page++ {
			items, err := searchPage(query, page, token)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				lowest = item.Stars
				if !seen[item.FullName] {
					seen[item.FullName] = true
					found = append(found, item)
				}
			}
			if len(items) < 100 {
				return found[:min(want, len(found))], nil
			}
		}
		if lowest == ceiling {
			return found[:min(want, len(found))], nil
		}
		ceiling = lowest
	}
	return found[:want], nil
}

// searchPage retries what GitHub refuses for rate: a 403 secondary limit or
// a 429, waiting as long as Retry-After says, or a growing backoff.
func searchPage(query string, page int, token string) ([]candidate, error) {
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		var items []candidate
		var wait time.Duration
		items, wait, err = searchOnce(query, page, token)
		if wait == 0 {
			return items, err
		}
		time.Sleep(max(wait, time.Duration(attempt)*backoff))
	}
	return nil, err
}

// searchOnce makes one request. A positive wait means it was rate limited.
func searchOnce(query string, page int, token string) ([]candidate, time.Duration, error) {
	time.Sleep(pause)
	params := url.Values{"q": {query}, "sort": {"stars"}, "order": {"desc"}, "per_page": {"100"}, "page": {fmt.Sprint(page)}}
	req, err := http.NewRequest(http.MethodGet, apiBase+"/search/repositories?"+params.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		err := fmt.Errorf("search %q page %d: %s: %s", query, page, resp.Status, body)
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			seconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			return nil, max(time.Duration(seconds)*time.Second, rateLimitFloor), err
		}
		return nil, 0, err
	}
	var result struct {
		Items []candidate `json:"items"`
	}
	return result.Items, 0, json.NewDecoder(resp.Body).Decode(&result)
}
