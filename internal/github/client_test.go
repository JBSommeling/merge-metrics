package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v72/github"
)

// newTestClient creates a Client whose underlying go-github client points at the
// given httptest.Server base URL, bypassing OAuth transport.
func newTestClient(server *httptest.Server) *Client {
	gh := gogithub.NewClient(nil)
	baseURL, _ := url.Parse(server.URL + "/")
	gh.BaseURL = baseURL
	gh.UploadURL = baseURL
	return &Client{gh: gh}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// GitHub API list response uses Link headers for pagination.
func setLinkHeader(w http.ResponseWriter, serverURL string, nextPage int) {
	if nextPage > 0 {
		link := fmt.Sprintf(`<%s/repos/owner/repo/pulls?page=%d>; rel="next"`, serverURL, nextPage)
		w.Header().Set("Link", link)
	}
}

func TestListOpenPullRequests(t *testing.T) {
	page1PR := map[string]any{
		"number":     1,
		"title":      "First PR",
		"state":      "open",
		"draft":      false,
		"additions":  10,
		"deletions":  2,
		"created_at": "2024-01-01T00:00:00Z",
		"updated_at": "2024-01-02T00:00:00Z",
		"user":       map[string]any{"login": "alice"},
	}
	page2PR := map[string]any{
		"number":     2,
		"title":      "Second PR",
		"state":      "open",
		"draft":      true,
		"additions":  5,
		"deletions":  1,
		"created_at": "2024-02-01T00:00:00Z",
		"updated_at": "2024-02-02T00:00:00Z",
		"user":       map[string]any{"login": "bob"},
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "2" {
			writeJSON(w, []any{page2PR})
		} else {
			setLinkHeader(w, server.URL, 2)
			writeJSON(w, []any{page1PR})
		}
	})

	client := newTestClient(server)
	prs, err := client.ListOpenPullRequests(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(prs))
	}

	if prs[0].Number != 1 || prs[0].Title != "First PR" || prs[0].User != "alice" || prs[0].Additions != 10 || prs[0].Draft != false {
		t.Errorf("unexpected first PR: %+v", prs[0])
	}
	if prs[1].Number != 2 || prs[1].Title != "Second PR" || prs[1].User != "bob" || prs[1].Draft != true {
		t.Errorf("unexpected second PR: %+v", prs[1])
	}
}

func TestListCommits(t *testing.T) {
	since := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	commits := []map[string]any{
		{
			"sha": "abc123",
			"commit": map[string]any{
				"message": "fix: something",
				"author": map[string]any{
					"name": "alice",
					"date": "2024-01-15T10:00:00Z",
				},
			},
		},
		{
			"sha": "def456",
			"commit": map[string]any{
				"message": "feat: another thing",
				"author": map[string]any{
					"name": "bob",
					"date": "2024-01-20T12:00:00Z",
				},
			},
		},
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/repos/owner/repo/commits", func(w http.ResponseWriter, r *http.Request) {
		sinceParam := r.URL.Query().Get("since")
		if sinceParam == "" {
			t.Error("expected since parameter")
		}
		writeJSON(w, commits)
	})

	client := newTestClient(server)
	result, err := client.ListCommits(context.Background(), "owner", "repo", since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(result))
	}
	if result[0].SHA != "abc123" || result[0].Author != "alice" || result[0].Message != "fix: something" {
		t.Errorf("unexpected first commit: %+v", result[0])
	}
	if result[1].SHA != "def456" || result[1].Author != "bob" {
		t.Errorf("unexpected second commit: %+v", result[1])
	}
}

func TestGetRepository(t *testing.T) {
	repoData := map[string]any{
		"name":              "my-repo",
		"full_name":         "owner/my-repo",
		"description":       "A test repository",
		"open_issues_count": 42,
		"default_branch":    "main",
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/repos/owner/my-repo", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, repoData)
	})

	client := newTestClient(server)
	repo, err := client.GetRepository(context.Background(), "owner", "my-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.Name != "my-repo" {
		t.Errorf("expected name %q, got %q", "my-repo", repo.Name)
	}
	if repo.FullName != "owner/my-repo" {
		t.Errorf("expected full_name %q, got %q", "owner/my-repo", repo.FullName)
	}
	if repo.Description != "A test repository" {
		t.Errorf("expected description %q, got %q", "A test repository", repo.Description)
	}
	if repo.OpenIssuesCount != 42 {
		t.Errorf("expected open_issues_count 42, got %d", repo.OpenIssuesCount)
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("expected default_branch %q, got %q", "main", repo.DefaultBranch)
	}
}

func TestListReleases(t *testing.T) {
	releases := []map[string]any{
		{
			"tag_name":     "v1.0.0",
			"name":         "Release 1.0.0",
			"draft":        false,
			"prerelease":   false,
			"published_at": "2024-01-10T00:00:00Z",
		},
		{
			// Draft release — should be skipped
			"tag_name":     "v1.1.0-draft",
			"name":         "Draft Release",
			"draft":        true,
			"prerelease":   false,
			"published_at": "2024-02-01T00:00:00Z",
		},
		{
			"tag_name":     "v0.9.0",
			"name":         "Pre-release 0.9.0",
			"draft":        false,
			"prerelease":   true,
			"published_at": "2023-12-01T00:00:00Z",
		},
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, releases)
	})

	client := newTestClient(server)
	result, err := client.ListReleases(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Draft release should be filtered out
	if len(result) != 2 {
		t.Fatalf("expected 2 non-draft releases, got %d", len(result))
	}
	if result[0].TagName != "v1.0.0" {
		t.Errorf("expected tag v1.0.0, got %q", result[0].TagName)
	}
	if result[0].Draft != false {
		t.Errorf("expected draft=false for first release")
	}
	if result[1].TagName != "v0.9.0" {
		t.Errorf("expected tag v0.9.0, got %q", result[1].TagName)
	}
	if result[1].Prerelease != true {
		t.Errorf("expected prerelease=true for second release")
	}
}
