// Package jenkins provides a minimal live-Jenkins client used to import jobs
// (pipelines and freestyle projects) from an existing Jenkins instance into
// BuildWorld. It intentionally depends only on the standard library so it can
// run both inside the API server and from the CLI without extra weight.
package jenkins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client talks to a Jenkins instance using HTTP Basic auth.
type Client struct {
	BaseURL    string
	User       string
	APIToken   string
	HTTPClient *http.Client
}

// NewClient builds a Jenkins client. token may be a password or an API token;
// Jenkins accepts either via Basic auth.
func NewClient(baseURL, user, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		User:       user,
		APIToken:   token,
		HTTPClient: http.DefaultClient,
	}
}

// jobURL assembles the Jenkins REST URL for a job, honouring an optional folder
// chain (Jenkins nests jobs under folder projects).
func jobURL(baseURL string, folders []string, job string) string {
	parts := []string{strings.TrimRight(baseURL, "/")}
	for _, folder := range folders {
		parts = append(parts, "job", url.PathEscape(folder))
	}
	parts = append(parts, "job", url.PathEscape(job))
	return strings.Join(parts, "/")
}

// FetchConfig retrieves the config.xml of the named job. folders is the chain of
// parent folder names (may be empty for a top-level job).
func (c *Client) FetchConfig(ctx context.Context, folders []string, job string) ([]byte, error) {
	endpoint := jobURL(c.BaseURL, folders, job) + "/config.xml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build jenkins request: %w", err)
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.APIToken)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to jenkins: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return nil, fmt.Errorf("jenkins returned %d for %s: %s", resp.StatusCode, endpoint, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

// ListJobs returns the immediate child jobs of a folder (or the root when folders
// is empty). It is used by the sync command to discover importable jobs.
func (c *Client) ListJobs(ctx context.Context, folders []string) ([]JobSummary, error) {
	endpoint := jobURL(c.BaseURL, folders, "")
	if endpoint == c.BaseURL {
		endpoint = c.BaseURL + "/api/json"
	} else {
		endpoint += "/api/json"
	}
	endpoint += "?tree=jobs[name,url,type]"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build jenkins request: %w", err)
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.APIToken)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to jenkins: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jenkins returned %d for %s", resp.StatusCode, endpoint)
	}
	var payload struct {
		Jobs []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
			Type string `json:"_class"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode jenkins response: %w", err)
	}
	out := make([]JobSummary, 0, len(payload.Jobs))
	for _, j := range payload.Jobs {
		out = append(out, JobSummary{Name: j.Name, URL: j.URL, Class: j.Type})
	}
	return out, nil
}

// JobSummary is a lightweight description of a Jenkins job.
type JobSummary struct {
	Name  string
	URL   string
	Class string
}
