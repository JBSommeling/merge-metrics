package github

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gogithub "github.com/google/go-github/v72/github"
	"golang.org/x/oauth2"
)

// Client wraps the go-github library with pagination and rate limit handling.
type Client struct {
	gh *gogithub.Client
}

// PullRequest is a simplified domain type for a GitHub pull request.
type PullRequest struct {
	Number    int
	Title     string
	State     string
	CreatedAt time.Time
	UpdatedAt time.Time
	User      string
	Additions int
	Deletions int
	Draft     bool
}

// Review is a simplified domain type for a pull request review.
type Review struct {
	ID          int64
	User        string
	State       string // "APPROVED", "CHANGES_REQUESTED", "COMMENTED"
	SubmittedAt time.Time
}

// ReviewRequest represents a requested reviewer on a pull request.
type ReviewRequest struct {
	Login string
}

// Commit is a simplified domain type for a repository commit.
type Commit struct {
	SHA     string
	Author  string
	Date    time.Time
	Message string
}

// Release is a simplified domain type for a GitHub release.
type Release struct {
	TagName     string
	Name        string
	PublishedAt time.Time
	Draft       bool
	Prerelease  bool
}

// Tag is a simplified domain type for a repository tag.
type Tag struct {
	Name string
}

// Repository is a simplified domain type for a GitHub repository.
type Repository struct {
	Name            string
	FullName        string
	Description     string
	OpenIssuesCount int
	DefaultBranch   string
}

// NewClient creates a new Client authenticated with the given personal access token.
func NewClient(token string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	gh := gogithub.NewClient(tc)
	return &Client{gh: gh}
}

// waitForRateLimit sleeps until the rate limit resets when a 403 is returned with
// rate limit information, then returns true so the caller can retry.
func waitForRateLimit(resp *gogithub.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusForbidden && resp.Rate.Remaining == 0 {
		sleepDuration := time.Until(resp.Rate.Reset.Time) + time.Second
		if sleepDuration > 5*time.Minute {
			sleepDuration = 5 * time.Minute
		}
		if sleepDuration > 0 {
			time.Sleep(sleepDuration)
		}
		return true
	}
	return false
}

// mapPR converts a go-github PullRequest into our domain PullRequest.
func mapPR(pr *gogithub.PullRequest) PullRequest {
	out := PullRequest{
		Number: pr.GetNumber(),
		Title:  pr.GetTitle(),
		State:  pr.GetState(),
		Draft:  pr.GetDraft(),
	}
	if pr.CreatedAt != nil {
		out.CreatedAt = pr.CreatedAt.Time
	}
	if pr.UpdatedAt != nil {
		out.UpdatedAt = pr.UpdatedAt.Time
	}
	if pr.User != nil {
		out.User = pr.User.GetLogin()
	}
	out.Additions = pr.GetAdditions()
	out.Deletions = pr.GetDeletions()
	return out
}

// ListOpenPullRequests returns all open pull requests for the given repository.
func (c *Client) ListOpenPullRequests(ctx context.Context, owner, repo string) ([]PullRequest, error) {
	opts := &gogithub.PullRequestListOptions{
		State: "open",
		ListOptions: gogithub.ListOptions{
			PerPage: 100,
		},
	}

	var all []PullRequest
	for {
		var (
			prs     []*gogithub.PullRequest
			resp    *gogithub.Response
			err     error
			retries int
		)
		for {
			prs, resp, err = c.gh.PullRequests.List(ctx, owner, repo, opts)
			if err != nil {
				if retries < 3 && resp != nil && waitForRateLimit(resp) {
					retries++
					continue
				}
				return nil, fmt.Errorf("listing open pull requests: %w", err)
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("listing open pull requests: %w", err)
		}
		for _, pr := range prs {
			all = append(all, mapPR(pr))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListPullRequestReviews returns all reviews for the given pull request.
func (c *Client) ListPullRequestReviews(ctx context.Context, owner, repo string, number int) ([]Review, error) {
	opts := &gogithub.ListOptions{PerPage: 100}

	var all []Review
	for {
		var (
			reviews []*gogithub.PullRequestReview
			resp    *gogithub.Response
			err     error
			retries int
		)
		for {
			reviews, resp, err = c.gh.PullRequests.ListReviews(ctx, owner, repo, number, opts)
			if err != nil {
				if retries < 3 && resp != nil && waitForRateLimit(resp) {
					retries++
					continue
				}
				return nil, fmt.Errorf("listing pull request reviews: %w", err)
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("listing pull request reviews: %w", err)
		}
		for _, r := range reviews {
			rev := Review{
				ID:    r.GetID(),
				State: r.GetState(),
			}
			if r.User != nil {
				rev.User = r.User.GetLogin()
			}
			if r.SubmittedAt != nil {
				rev.SubmittedAt = r.SubmittedAt.Time
			}
			all = append(all, rev)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListRequestedReviewers returns all requested reviewers for the given pull request.
func (c *Client) ListRequestedReviewers(ctx context.Context, owner, repo string, number int) ([]ReviewRequest, error) {
	opts := &gogithub.ListOptions{PerPage: 100}

	var all []ReviewRequest
	for {
		var (
			reviewers *gogithub.Reviewers
			resp      *gogithub.Response
			err       error
			retries   int
		)
		for {
			reviewers, resp, err = c.gh.PullRequests.ListReviewers(ctx, owner, repo, number, opts)
			if err != nil {
				if retries < 3 && resp != nil && waitForRateLimit(resp) {
					retries++
					continue
				}
				return nil, fmt.Errorf("listing requested reviewers: %w", err)
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("listing requested reviewers: %w", err)
		}
		for _, u := range reviewers.Users {
			all = append(all, ReviewRequest{Login: u.GetLogin()})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListClosedPullRequests returns closed pull requests updated at or after since,
// sorted by updated descending. Pagination stops as soon as a PR's UpdatedAt is
// before since.
func (c *Client) ListClosedPullRequests(ctx context.Context, owner, repo string, since time.Time) ([]PullRequest, error) {
	opts := &gogithub.PullRequestListOptions{
		State:     "closed",
		Sort:      "updated",
		Direction: "desc",
		ListOptions: gogithub.ListOptions{
			PerPage: 100,
		},
	}

	var all []PullRequest
outer:
	for {
		var (
			prs     []*gogithub.PullRequest
			resp    *gogithub.Response
			err     error
			retries int
		)
		for {
			prs, resp, err = c.gh.PullRequests.List(ctx, owner, repo, opts)
			if err != nil {
				if retries < 3 && resp != nil && waitForRateLimit(resp) {
					retries++
					continue
				}
				return nil, fmt.Errorf("listing closed pull requests: %w", err)
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("listing closed pull requests: %w", err)
		}
		for _, pr := range prs {
			updatedAt := pr.GetUpdatedAt().Time
			if updatedAt.Before(since) {
				break outer
			}
			all = append(all, mapPR(pr))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListCommits returns all commits to the default branch since the given time.
func (c *Client) ListCommits(ctx context.Context, owner, repo string, since time.Time) ([]Commit, error) {
	opts := &gogithub.CommitsListOptions{
		Since: since,
		ListOptions: gogithub.ListOptions{
			PerPage: 100,
		},
	}

	var all []Commit
	for {
		var (
			commits []*gogithub.RepositoryCommit
			resp    *gogithub.Response
			err     error
			retries int
		)
		for {
			commits, resp, err = c.gh.Repositories.ListCommits(ctx, owner, repo, opts)
			if err != nil {
				if retries < 3 && resp != nil && waitForRateLimit(resp) {
					retries++
					continue
				}
				return nil, fmt.Errorf("listing commits: %w", err)
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("listing commits: %w", err)
		}
		for _, rc := range commits {
			commit := Commit{
				SHA: rc.GetSHA(),
			}
			if rc.Commit != nil {
				commit.Message = rc.Commit.GetMessage()
				if rc.Commit.Author != nil {
					commit.Author = rc.Commit.Author.GetName()
					if rc.Commit.Author.Date != nil {
						commit.Date = rc.Commit.Author.Date.Time
					}
				}
			}
			all = append(all, commit)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListReleases returns all non-draft releases for the given repository.
func (c *Client) ListReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	opts := &gogithub.ListOptions{PerPage: 100}

	var all []Release
	for {
		var (
			releases []*gogithub.RepositoryRelease
			resp     *gogithub.Response
			err      error
			retries  int
		)
		for {
			releases, resp, err = c.gh.Repositories.ListReleases(ctx, owner, repo, opts)
			if err != nil {
				if retries < 3 && resp != nil && waitForRateLimit(resp) {
					retries++
					continue
				}
				return nil, fmt.Errorf("listing releases: %w", err)
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("listing releases: %w", err)
		}
		for _, r := range releases {
			if r.GetDraft() {
				continue
			}
			rel := Release{
				TagName:    r.GetTagName(),
				Name:       r.GetName(),
				Prerelease: r.GetPrerelease(),
			}
			if r.PublishedAt != nil {
				rel.PublishedAt = r.PublishedAt.Time
			}
			all = append(all, rel)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListTags returns all tags for the given repository.
func (c *Client) ListTags(ctx context.Context, owner, repo string) ([]Tag, error) {
	opts := &gogithub.ListOptions{PerPage: 100}

	var all []Tag
	for {
		var (
			tags    []*gogithub.RepositoryTag
			resp    *gogithub.Response
			err     error
			retries int
		)
		for {
			tags, resp, err = c.gh.Repositories.ListTags(ctx, owner, repo, opts)
			if err != nil {
				if retries < 3 && resp != nil && waitForRateLimit(resp) {
					retries++
					continue
				}
				return nil, fmt.Errorf("listing tags: %w", err)
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("listing tags: %w", err)
		}
		for _, t := range tags {
			all = append(all, Tag{Name: t.GetName()})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// GetRepository returns metadata for the given repository.
func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	var (
		r    *gogithub.Repository
		resp *gogithub.Response
		err  error
	)
	for {
		r, resp, err = c.gh.Repositories.Get(ctx, owner, repo)
		if err != nil {
			if resp != nil && waitForRateLimit(resp) {
				continue
			}
			return nil, err
		}
		break
	}
	return &Repository{
		Name:            r.GetName(),
		FullName:        r.GetFullName(),
		Description:     r.GetDescription(),
		OpenIssuesCount: r.GetOpenIssuesCount(),
		DefaultBranch:   r.GetDefaultBranch(),
	}, nil
}
