package gh

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Run struct {
	DatabaseID int    `json:"databaseId"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	Event       string `json:"event"`
	HeadBranch  string `json:"headBranch"`
	Workflow    string `json:"workflowName"`
	CreatedAt   time.Time `json:"createdAt"`
	URL         string `json:"url"`
}

type Step struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type Job struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Steps      []Step `json:"steps"`
}

type RunDetail struct {
	Run
	Jobs []Job `json:"jobs"`
}

type Repo struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	NameWithOwner string    `json:"nameWithOwner"`
	Description   string    `json:"description"`
	UpdatedAt     time.Time `json:"updatedAt"`
	URL           string    `json:"url"`
	IsPrivate     bool      `json:"isPrivate"`
}

func runGh(repo string, args ...string) (string, error) {
	cmdArgs := append([]string{}, args...)
	if repo != "" {
		cmdArgs = append([]string{"--repo", repo}, cmdArgs...)
	}
	cmd := exec.Command("gh", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh %s: %s: %w", strings.Join(cmdArgs, " "), string(out), err)
	}
	return string(out), nil
}

func GetRuns(repo string, limit int) ([]Run, error) {
	out, err := runGh(repo, "run", "list", "--json", "databaseId,name,status,conclusion,event,headBranch,workflowName,createdAt,url", "--limit", fmt.Sprintf("%d", limit))
	if err != nil {
		return nil, err
	}
	var runs []Run
	if err := json.Unmarshal([]byte(out), &runs); err != nil {
		return nil, fmt.Errorf("parsing run list: %w", err)
	}
	return runs, nil
}

func GetRun(repo, runID string) (RunDetail, error) {
	out, err := runGh(repo, "run", "view", runID, "--json", "databaseId,name,status,conclusion,event,headBranch,workflowName,createdAt,url,jobs")
	if err != nil {
		return RunDetail{}, err
	}
	var detail RunDetail
	if err := json.Unmarshal([]byte(out), &detail); err != nil {
		return RunDetail{}, fmt.Errorf("parsing run detail: %w", err)
	}
	return detail, nil
}

func GetLogs(repo, runID, jobName string) (string, error) {
	if jobName == "" {
		out, err := runGh(repo, "run", "view", runID, "--log")
		return out, err
	}
	out, err := runGh(repo, "run", "view", runID, "--log", "--job", jobName)
	return out, err
}

func CancelRun(repo, runID string) error {
	_, err := runGh(repo, "run", "cancel", runID)
	return err
}

func RerunRun(repo, runID string) error {
	_, err := runGh(repo, "run", "rerun", runID)
	return err
}

func GetCurrentRepo() (Repo, error) {
	out, err := runGh("", "repo", "view", "--json", "id,name,nameWithOwner,description,updatedAt,url,isPrivate")
	if err != nil {
		return Repo{}, err
	}
	var repo Repo
	if err := json.Unmarshal([]byte(out), &repo); err != nil {
		return Repo{}, fmt.Errorf("parsing repo view: %w", err)
	}
	return repo, nil
}

func GetRepos(limit int) ([]Repo, error) {
	out, err := runGh("", "repo", "list", "--json", "id,name,nameWithOwner,description,updatedAt,url,isPrivate", "--limit", fmt.Sprintf("%d", limit))
	if err != nil {
		return nil, err
	}
	var repos []Repo
	if err := json.Unmarshal([]byte(out), &repos); err != nil {
		return nil, fmt.Errorf("parsing repo list: %w", err)
	}
	return repos, nil
}

func WatchRun(repo, runID string) (*exec.Cmd, io.ReadCloser, error) {
	args := []string{"run", "watch", runID}
	if repo != "" {
		args = append([]string{"--repo", repo}, args...)
	}
	cmd := exec.Command("gh", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return cmd, stdout, nil
}

func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		browser := os.Getenv("BROWSER")
		if browser == "" {
			browser = "xdg-open"
		}
		cmd = exec.Command(browser, url)
	}
	return cmd.Start()
}

func BrowseRun(repo, runID string) error {
	out, err := runGh(repo, "run", "view", runID, "--json", "url")
	if err != nil {
		return err
	}
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return fmt.Errorf("parsing run url: %w", err)
	}
	if strings.TrimSpace(payload.URL) == "" {
		return fmt.Errorf("empty run url")
	}
	return OpenBrowser(payload.URL)
}

type Author struct {
	Login string `json:"login"`
}

type Repository struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type PR struct {
	Number        int          `json:"number"`
	Title         string       `json:"title"`
	State         string       `json:"state"`
	HeadRefName   string       `json:"headRefName"`
	BaseRefName   string       `json:"baseRefName"`
	URL           string       `json:"url"`
	Mergeable     string       `json:"mergeable"`
	ReviewDecision string      `json:"reviewDecision"`
	IsDraft       bool         `json:"isDraft"`
	Author        Author       `json:"author"`
	Repository    Repository   `json:"repository"`
	CreatedAt     time.Time    `json:"createdAt"`
	UpdatedAt     time.Time    `json:"updatedAt"`
}

type Check struct {
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Bucket      string    `json:"bucket"`
	Link        string    `json:"link"`
	Event       string    `json:"event"`
	Workflow    string    `json:"workflow"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	Description string    `json:"description"`
}

func GetPRs(repo string, limit int) ([]PR, error) {
	out, err := runGh(repo, "pr", "list", "--json", "number,title,state,headRefName,baseRefName,url,mergeable,reviewDecision,isDraft,author,createdAt,updatedAt", "--limit", fmt.Sprintf("%d", limit))
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("parsing pr list: %w", err)
	}
	return prs, nil
}

func GetMyPRs(repo string, limit int) ([]PR, error) {
	prs, err := GetPRs(repo, limit)
	if err != nil {
		return nil, err
	}

	user, err := getCurrentUser()
	if err != nil {
		return nil, err
	}

	var myPRs []PR
	for _, pr := range prs {
		if pr.Author.Login == user {
			myPRs = append(myPRs, pr)
		}
	}
	return myPRs, nil
}

func getCurrentUser() (string, error) {
	out, err := runGh("", "api", "user", "-q", ".login")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func SearchPRs(query string, limit int) ([]PR, error) {
	searchQuery := fmt.Sprintf("%s in:all", query)
	out, err := runGh("", "search", "prs", searchQuery, "--json", "number,title,state,isDraft,author,repository,createdAt,updatedAt,url", "--limit", fmt.Sprintf("%d", limit))
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("parsing pr search: %w", err)
	}
	return prs, nil
}

func GetPRChecks(repo string, prNumber int) ([]Check, error) {
	out, err := runGh(repo, "pr", "checks", fmt.Sprintf("%d", prNumber), "--json", "name,state,bucket,link,event,workflow,startedAt,completedAt,description")
	if err != nil {
		return nil, err
	}
	var checks []Check
	if err := json.Unmarshal([]byte(out), &checks); err != nil {
		return nil, fmt.Errorf("parsing pr checks: %w", err)
	}
	return checks, nil
}

func BrowsePR(repo string, prNumber int) error {
	out, err := runGh(repo, "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "url")
	if err != nil {
		return err
	}
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return fmt.Errorf("parsing pr url: %w", err)
	}
	if strings.TrimSpace(payload.URL) == "" {
		return fmt.Errorf("empty pr url")
	}
	return OpenBrowser(payload.URL)
}
