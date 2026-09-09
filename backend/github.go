package backend

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/robin/lazyactions/gh"
)

type githubBackend struct {
	repo string
}

func NewGitHub(repo string) Backend {
	return &githubBackend{repo: repo}
}

func (b *githubBackend) Name() string {
	return "GitHub"
}

func (b *githubBackend) Tabs() []string {
	return []string{"runs", "jobs", "prs", "repos"}
}

func (b *githubBackend) Columns(tab string) []string {
	switch tab {
	case "runs":
		return []string{"id", "workflow", "status", "branch"}
	case "jobs":
		return []string{"job", "status"}
	case "prs":
		return []string{"number", "title", "state", "branch"}
	case "repos":
		return []string{"repository", "description"}
	default:
		return nil
	}
}

func (b *githubBackend) Fetch(tab, filter string) ([]Resource, error) {
	switch tab {
	case "runs":
		runs, err := gh.GetRuns(b.repo, 100)
		if err != nil {
			return nil, err
		}
		out := make([]Resource, 0, len(runs))
		for _, r := range runs {
			out = append(out, Resource{
				ID:     fmt.Sprintf("%d", r.DatabaseID),
				Name:   r.Workflow,
				Status: r.Status,
				Details: map[string]string{
					"branch": r.HeadBranch,
					"event":  r.Event,
					"url":    r.URL,
				},
				Actions: []string{"logs", "cancel", "rerun", "browse"},
				Raw: map[string]any{
					"run": r,
				},
			})
		}
		return out, nil
	case "jobs":
		return nil, fmt.Errorf("jobs tab requires a selected run")
	case "prs":
		prs, err := gh.GetPRs(b.repo, 100)
		if err != nil {
			return nil, err
		}
		out := make([]Resource, 0, len(prs))
		for _, p := range prs {
			out = append(out, Resource{
				ID:     fmt.Sprintf("%d", p.Number),
				Name:   p.Title,
				Status: p.State,
				Details: map[string]string{
					"branch":    p.HeadRefName + " -> " + p.BaseRefName,
					"author":    p.Author.Login,
					"url":       p.URL,
					"draft":     fmt.Sprintf("%v", p.IsDraft),
					"review":    p.ReviewDecision,
					"mergeable": p.Mergeable,
				},
				Actions: []string{"open", "checkout", "close", "merge", "approve", "ready", "detail", "diff"},
				Raw: map[string]any{
					"pr": p,
				},
			})
		}
		return out, nil
	case "repos":
		repos, err := gh.GetRepos(100)
		if err != nil {
			return nil, err
		}
		out := make([]Resource, 0, len(repos))
		for _, repo := range repos {
			out = append(out, Resource{
				ID:     repo.NameWithOwner,
				Name:   repo.NameWithOwner,
				Status: "",
				Details: map[string]string{
					"description": repo.Description,
					"url":         repo.URL,
					"private":     fmt.Sprintf("%v", repo.IsPrivate),
					"updated":     repo.UpdatedAt.Format(time.RFC3339),
				},
				Actions: []string{"select"},
				Raw: map[string]any{
					"repo": repo,
				},
			})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown tab %q", tab)
	}
}

func (b *githubBackend) Inspect(tab, id string) (Resource, error) {
	switch tab {
	case "runs":
		run, err := gh.GetRun(b.repo, id)
		if err != nil {
			return Resource{}, err
		}
		details := map[string]string{
			"workflow": run.Workflow,
			"event":    run.Event,
			"created":  run.CreatedAt.Format(time.RFC3339),
			"url":      run.URL,
		}
		jobs := make([]string, 0, len(run.Jobs))
		for _, j := range run.Jobs {
			jobs = append(jobs, j.Name)
		}
		return Resource{
			ID:      fmt.Sprintf("%d", run.DatabaseID),
			Name:    run.Workflow,
			Status:  run.Status,
			Details: details,
			Actions: []string{"logs", "cancel", "rerun", "browse"},
			Raw: map[string]any{
				"run":  run,
				"jobs": jobs,
			},
		}, nil
	case "prs":
		pr, err := gh.GetPRDetail(b.repo, mustInt(id))
		if err != nil {
			return Resource{}, err
		}
		details := map[string]string{
			"title":      pr.Title,
			"state":      pr.State,
			"branch":     pr.HeadRefName + " -> " + pr.BaseRefName,
			"author":     pr.Author.Login,
			"created":    pr.CreatedAt.Format(time.RFC3339),
			"updated":    pr.UpdatedAt.Format(time.RFC3339),
			"url":        pr.URL,
			"body":       pr.Body,
			"review":     pr.ReviewDecision,
			"mergeable":  pr.Mergeable,
		}
		labels := make([]string, 0, len(pr.Labels))
		for _, l := range pr.Labels {
			labels = append(labels, l.Name)
		}
		assignees := make([]string, 0, len(pr.Assignees))
		for _, a := range pr.Assignees {
			assignees = append(assignees, a.Login)
		}
		comments := make([]string, 0, len(pr.Comments))
		for _, c := range pr.Comments {
			comments = append(comments, c.Author.Login+": "+c.Body)
		}
		reviews := make([]string, 0, len(pr.Reviews))
		for _, r := range pr.Reviews {
			reviews = append(reviews, r.Author.Login+": "+r.State)
		}
		return Resource{
			ID:      id,
			Name:    pr.Title,
			Status:  pr.State,
			Details: details,
			Actions: []string{"open", "checkout", "close", "merge", "approve", "ready", "diff"},
			Raw: map[string]any{
				"pr":        pr,
				"labels":    labels,
				"assignees": assignees,
				"comments":  comments,
				"reviews":   reviews,
			},
		}, nil
	default:
		return Resource{}, fmt.Errorf("inspect not supported for %q", tab)
	}
}

func (b *githubBackend) RunAction(tab, id, action string, args []string) (string, error) {
	switch tab {
	case "runs":
		switch action {
		case "cancel":
			return "", gh.CancelRun(b.repo, id)
		case "rerun":
			return "", gh.RerunRun(b.repo, id)
		case "browse":
			return "", gh.BrowseRun(b.repo, id)
		case "logs":
			out, err := gh.GetLogs(b.repo, id, "")
			return out, err
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	case "prs":
		switch action {
		case "open":
			return "", gh.BrowsePR(b.repo, mustInt(id))
		case "checkout":
			return "", gh.CheckoutPR(b.repo, mustInt(id))
		case "close":
			return "", gh.ClosePR(b.repo, mustInt(id))
		case "merge":
			return "", gh.MergePR(b.repo, mustInt(id))
		case "approve":
			return "", gh.ApprovePR(b.repo, mustInt(id))
		case "ready":
			return "", gh.ReadyPR(b.repo, mustInt(id))
		case "detail":
			pr, err := gh.GetPRDetail(b.repo, mustInt(id))
			if err != nil {
				return "", err
			}
			parts := []string{pr.Title, "", pr.Body}
			for _, c := range pr.Comments {
				parts = append(parts, "", c.Author.Login+": "+c.Body)
			}
			for _, r := range pr.Reviews {
				parts = append(parts, "", r.Author.Login+" reviewed: "+r.Body)
			}
			return strings.Join(parts, "\n"), nil
		case "diff":
			return gh.GetPRDiff(b.repo, mustInt(id))
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	default:
		return "", fmt.Errorf("actions not supported for %q", tab)
	}
}

func (b *githubBackend) Stream(tab, id string) (*exec.Cmd, io.ReadCloser, error) {
	if tab == "runs" {
		return gh.WatchRun(b.repo, id)
	}
	return nil, nil, fmt.Errorf("streaming not supported for %q", tab)
}

func mustInt(s string) int {
	n, _ := parseInt(s)
	return n
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscan(s, &n)
	return n, err
}
