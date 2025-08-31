package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/v6/table"
)

const envVarPrimaryPAT = "LAZY_DEV_OPS_PAT"

type prResponse struct {
	Value []pullRequest `json:"value"`
	Count int           `json:"count"`
}

type identity struct {
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName"`
}

type repositoryInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type reviewer struct {
	DisplayName string `json:"displayName"`
	Vote        int    `json:"vote"`
}

type links struct {
	Web struct {
		Href string `json:"href"`
	} `json:"web"`
}

type pullRequest struct {
	PullRequestID int            `json:"pullRequestId"`
	Title         string         `json:"title"`
	Status        string         `json:"status"`
	CreationDate  time.Time      `json:"creationDate"`
	Repository    repositoryInfo `json:"repository"`
	CreatedBy     identity       `json:"createdBy"`
	SourceRefName string         `json:"sourceRefName"`
	TargetRefName string         `json:"targetRefName"`
	Reviewers     []reviewer     `json:"reviewers"`
	Links         links          `json:"_links"`
}

type prStatusContext struct {
	Name  string `json:"name"`
	Genre string `json:"genre"`
}

type prStatus struct {
	State        string          `json:"state"`
	Description  string          `json:"description"`
	Context      prStatusContext `json:"context"`
	TargetURL    string          `json:"targetUrl"`
	CreationDate time.Time       `json:"creationDate"`
	UpdatedDate  time.Time       `json:"updatedDate"`
}

type prStatusResponse struct {
	Value []prStatus `json:"value"`
	Count int        `json:"count"`
}

type config struct {
	Org     string
	Project string
	Pat     string
	Top     int
	ApiVer  string
}

func main() {
	cfg := getConfig()

	prs, err := fetchActivePRs(cfg)
	if err != nil {
		log.Fatalln("Error: ", err)
	}

	if len(prs) == 0 {
		fmt.Println("No active pull requests found.")
		return
	}

	// sort by creation date desc
	sort.Slice(prs, func(i, j int) bool { return prs[i].CreationDate.After(prs[j].CreationDate) })

	printTable(cfg, prs)
}

func getConfig() config {
	// Flags
	org := flag.String("org", "", "Azure DevOps organization (e.g., myorg)")
	project := flag.String("project", "", "Azure DevOps project name")
	top := flag.Int("top", 50, "Max number of PRs to fetch")
	apiVer := flag.String("api-version", "7.1-preview.1", "Azure DevOps API version")
	flag.Parse()

	pat := os.Getenv(envVarPrimaryPAT)

	if *org == "" || *project == "" {
		failUsage("--org and --project are required. Set " + envVarPrimaryPAT + " env var for authentication.")
	}
	if pat == "" {
		failUsage("Environment variable " + envVarPrimaryPAT + " is required for authentication.")
	}

	cfg := config{
		Org:     *org,
		Project: *project,
		Pat:     pat,
		Top:     *top,
		ApiVer:  *apiVer,
	}
	return cfg
}

func fetchActivePRs(cfg config) ([]pullRequest, error) {
	base := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/pullrequests", url.PathEscape(cfg.Org), url.PathEscape(cfg.Project))
	q := url.Values{}
	q.Set("searchCriteria.status", "active")
	if cfg.Top > 0 {
		q.Set("$top", fmt.Sprintf("%d", cfg.Top))
	}
	q.Set("api-version", cfg.ApiVer)

	endpoint := base + "?" + q.Encode()

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// PAT via basic auth (username can be empty or anything)
	token := base64.StdEncoding.EncodeToString([]byte(":" + cfg.Pat))
	req.Header.Set("Authorization", "Basic "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, errors.New("authentication failed (401/403). Ensure " + envVarPrimaryPAT + " is valid and has Code (Read) scope")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed: %s", resp.Status)
	}

	var prr prResponse
	dec := json.NewDecoder(resp.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&prr); err != nil {
		// if strict decode fails due to preview changes, retry with permissive decode
		resp.Body.Close()
		// Re-do request to get a fresh body
		req2, _ := http.NewRequest("GET", endpoint, nil)
		req2.Header = req.Header.Clone()
		resp2, e2 := client.Do(req2)
		if e2 != nil {
			return nil, e2
		}
		defer resp2.Body.Close()
		if resp2.StatusCode < 200 || resp2.StatusCode >= 300 {
			return nil, fmt.Errorf("request failed: %s", resp2.Status)
		}
		type loose struct {
			Value []pullRequest `json:"value"`
		}
		var lr loose
		if e := json.NewDecoder(resp2.Body).Decode(&lr); e != nil {
			return nil, e
		}
		return lr.Value, nil
	}

	return prr.Value, nil
}

func printTable(cfg config, prs []pullRequest) {
	w := table.NewWriter()
	w.SetOutputMirror(os.Stdout)
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"PR", "Title", "Author", "Repo", "Source->Target", "Votes", "Checks", "Created", "URL"})

	for _, pr := range prs {
		votes := summarizeVotesTyped(pr.Reviewers)
		title := pr.Title
		author := pr.CreatedBy.DisplayName
		repo := pr.Repository.Name
		st := refShort(pr.SourceRefName) + "->" + refShort(pr.TargetRefName)
		created := humanize.Time(pr.CreationDate)
		href := pr.Links.Web.Href
		status := getPRStatusOverall(cfg, pr)
		w.AppendRow(table.Row{
			fmt.Sprintf("%d", pr.PullRequestID),
			title,
			author,
			repo,
			st,
			votes,
			status,
			created,
			href,
		})
	}

	w.Render()
}

func getPRStatusOverall(cfg config, pr pullRequest) string {
	// Build endpoint: https://dev.azure.com/{org}/{project}/_apis/git/repositories/{repoId}/pullRequests/{pullRequestId}/statuses?api-version=...
	base := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/statuses", url.PathEscape(cfg.Org), url.PathEscape(cfg.Project), url.PathEscape(pr.Repository.ID), pr.PullRequestID)
	q := url.Values{}
	q.Set("api-version", cfg.ApiVer)
	endpoint := base + "?" + q.Encode()

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "Unknown"
	}
	token := base64.StdEncoding.EncodeToString([]byte(":" + cfg.Pat))
	req.Header.Set("Authorization", "Basic "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "Unknown"
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return "Unauthorized"
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "Unknown"
	}

	var sr prStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return "Unknown"
	}

	if len(sr.Value) == 0 {
		return "No checks"
	}

	anyPending := false
	anyFailed := false
	anyError := false
	anySucceeded := false
	allSucceededOrNA := true

	for _, s := range sr.Value {
		state := strings.ToLower(s.State)
		switch state {
		case "succeeded", "success":
			anySucceeded = true
		case "pending", "inprogress", "in_progress":
			anyPending = true
			allSucceededOrNA = false
		case "failed", "failure":
			anyFailed = true
			allSucceededOrNA = false
		case "error":
			anyError = true
			allSucceededOrNA = false
		case "notapplicable", "not_applicable", "notset":
			// neutral
		default:
			// unknown -> treat as not fully succeeded
			allSucceededOrNA = false
		}
	}

	if anyFailed || anyError {
		return "Failed"
	}
	if anyPending {
		return "In Progress"
	}
	if anySucceeded && allSucceededOrNA {
		return "Passed"
	}
	// If we reached here and there were statuses but none conclusive
	return "Unknown"
}

func refShort(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/")
	return ref
}

func summarizeVotesTyped(reviewers []reviewer) string {
	if len(reviewers) == 0 {
		return "0"
	}
	up, down, wait := 0, 0, 0
	for _, r := range reviewers {
		switch {
		case r.Vote > 0:
			up++
		case r.Vote < 0:
			down++
		default:
			wait++
		}
	}
	if down > 0 {
		return fmt.Sprintf("-%d/%d", down, len(reviewers))
	}
	if up > 0 {
		return fmt.Sprintf("+%d/%d", up, len(reviewers))
	}
	if wait > 0 {
		return fmt.Sprintf("~%d/%d", wait, len(reviewers))
	}
	return fmt.Sprintf("%d", len(reviewers))
}

func failUsage(msg string) {
	fmt.Fprintln(os.Stderr, "Error:", msg)
	fmt.Fprintln(os.Stderr, "\nUsage: lazydevops --org <org> --project <project> [--repo <repo>] [--top N]\nSet "+envVarPrimaryPAT+" environment variable with a Personal Access Token (Code: Read).")
	os.Exit(2)
}
