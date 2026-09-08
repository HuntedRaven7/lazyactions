package tui

import (
	"bufio"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/robin/lazyactions/gh"
)

type viewType int

const (
	viewMain viewType = iota
	viewLogs
	viewRepo
	viewPR
)

type model struct {
	runs        []gh.Run
	detail      gh.RunDetail
	logs        string
	logOffset   int
	selectedRun int
	runsOffset  int
	selectedJob int
	jobsOffset  int
	view        viewType
	focus       focusType
	width       int
	height      int
	err         error
	loading     bool
	statusMsg   string
	statusTimer int

	repos        []gh.Repo
	selectedRepo int
	repoOffset   int
	repoFilter   string
	currentRepo  gh.Repo

	watching    bool
	watchCmd    *exec.Cmd
	watchLogs   string
	watchCh     chan string

	prs        []gh.PR
	checks     []gh.Check
	selectedPR int
	prOffset   int
	myPRsOnly  bool
	prScope    string
	prPage     int
	prPageSize int
}

type focusType int

const (
	focusRuns focusType = iota
	focusJobs
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			Margin(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	failureStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

	pendingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Margin(0, 1)

	focusBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("42")).
				Padding(0, 1)

	unfocusBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
)

func formatStatus(status, conclusion string) string {
	switch status {
	case "completed":
		switch conclusion {
		case "success":
			return successStyle.Render("success")
		case "failure":
			return failureStyle.Render("failed")
		case "cancelled":
			return pendingStyle.Render("cancelled")
		default:
			return pendingStyle.Render(conclusion)
		}
	case "in_progress":
		return runningStyle.Render("running")
	case "queued":
		return pendingStyle.Render("queued")
	default:
		return pendingStyle.Render(status)
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func New() model {
	return model{focus: focusRuns}
}

func (m model) Init() tea.Cmd {
	return fetchCurrentRepo
}

func fetchCurrentRepo() tea.Msg {
	repo, err := gh.GetCurrentRepo()
	if err != nil {
		return errMsg{err}
	}
	return currentRepoMsg(repo)
}

func fetchRunsForRepo(repo string) tea.Cmd {
	return func() tea.Msg {
		runs, err := gh.GetRuns(repo, 50)
		if err != nil {
			return errMsg{err}
		}
		return runsMsg(runs)
	}
}

func fetchDetailForRepo(repo, runID string) tea.Cmd {
	return func() tea.Msg {
		detail, err := gh.GetRun(repo, runID)
		if err != nil {
			return errMsg{err}
		}
		return detailMsg(detail)
	}
}

func fetchLogsForRepo(repo, runID, jobName string) tea.Cmd {
	return func() tea.Msg {
		logs, err := gh.GetLogs(repo, runID, jobName)
		if err != nil {
			return errMsg{err}
		}
		return logsMsg(logs)
	}
}

func cancelRunForRepo(repo, runID string) tea.Cmd {
	return func() tea.Msg {
		err := gh.CancelRun(repo, runID)
		if err != nil {
			return errMsg{err}
		}
		return statusMsg("Run cancelled")
	}
}

func rerunRunForRepo(repo, runID string) tea.Cmd {
	return func() tea.Msg {
		err := gh.RerunRun(repo, runID)
		if err != nil {
			return errMsg{err}
		}
		return statusMsg("Run rerun triggered")
	}
}

func fetchRepos() tea.Msg {
	repos, err := gh.GetRepos(100)
	if err != nil {
		return errMsg{err}
	}
	sort.Slice(repos, func(i, j int) bool {
		return strings.ToLower(repos[i].NameWithOwner) < strings.ToLower(repos[j].NameWithOwner)
	})
	return reposMsg(repos)
}

func fetchPRs(repo string) tea.Cmd {
	return func() tea.Msg {
		return fetchPRsImpl(repo, false, false)
	}
}

func fetchMyPRs(repo string) tea.Cmd {
	return func() tea.Msg {
		return fetchPRsImpl(repo, true, false)
	}
}

func fetchGlobalPRs(query string) tea.Cmd {
	return func() tea.Msg {
		prs, err := gh.SearchPRs(query, 100)
		if err != nil {
			return errMsg{err}
		}
		sort.Slice(prs, func(i, j int) bool {
			return prs[i].Number > prs[j].Number
		})
		return prsMsg(prs)
	}
}

func fetchPRsImpl(repo string, myPRsOnly bool, global bool) tea.Msg {
	var prs []gh.PR
	var err error
	if global {
		prs, err = gh.SearchPRs("", 100)
	} else if myPRsOnly {
		prs, err = gh.GetMyPRs(repo, 100)
	} else {
		prs, err = gh.GetPRs(repo, 100)
	}
	if err != nil {
		return errMsg{err}
	}
	sort.Slice(prs, func(i, j int) bool {
		return prs[i].Number > prs[j].Number
	})
	return prsMsg(prs)
}

func fetchPRChecks(repo string, prNumber int) tea.Cmd {
	return func() tea.Msg {
		checks, err := gh.GetPRChecks(repo, prNumber)
		if err != nil {
			return errMsg{err}
		}
		return checksMsg(checks)
	}
}

func startWatch(repo, runID string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"run", "watch", runID}
		if repo != "" {
			args = append([]string{"--repo", repo}, args...)
		}
		cmd := exec.Command("gh", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return errMsg{err}
		}
		if err := cmd.Start(); err != nil {
			return errMsg{err}
		}

		ch := make(chan string, 100)
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				ch <- scanner.Text()
			}
			cmd.Wait()
			close(ch)
		}()

		return watchStartMsg{cmd: cmd, ch: ch}
	}
}

func watchTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return watchMsg{}
	})
}

type currentRepoMsg gh.Repo
type runsMsg []gh.Run
type detailMsg gh.RunDetail
type logsMsg string
type errMsg struct{ err error }
type statusMsg string
type reposMsg []gh.Repo
type watchStartMsg struct {
	cmd    *exec.Cmd
	ch     chan string
}
type watchMsg struct {
	line string
	done bool
}
type prsMsg []gh.PR
type checksMsg []gh.Check

func reverseSlice[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func (e errMsg) Error() string { return e.err.Error() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case currentRepoMsg:
		m.currentRepo = gh.Repo(msg)
		return m, fetchRunsForRepo(m.currentRepo.NameWithOwner)
	case runsMsg:
		m.runs = msg
		reverseSlice(m.runs)
		m.loading = false
		m.selectedRun = 0
		if len(m.runs) > 0 {
			return m, fetchDetailForRepo(m.currentRepo.NameWithOwner, strconv.Itoa(m.runs[0].DatabaseID))
		}
		return m, nil
	case detailMsg:
		m.detail = gh.RunDetail(msg)
		m.selectedJob = 0
		return m, nil
	case logsMsg:
		m.logs = string(msg)
		m.logOffset = 0
		return m, nil
	case errMsg:
		m.err = msg.err
		m.loading = false
		return m, nil
	case statusMsg:
		m.statusMsg = string(msg)
		m.statusTimer = 60
		return m, nil
	case reposMsg:
		m.repos = msg
		reverseSlice(m.repos)
		m.loading = false
		m.selectedRepo = 0
		return m, nil
	case watchStartMsg:
		m.watching = true
		m.watchCmd = msg.cmd
		m.watchCh = msg.ch
		m.watchLogs = ""
		m.view = viewLogs
		m.logOffset = 0
		return m, watchTick()
	case watchMsg:
		if !m.watching || m.watchCh == nil {
			return m, nil
		}

		select {
		case line, ok := <-m.watchCh:
			if !ok {
				m.watching = false
				m.watchCh = nil
				if m.watchCmd != nil && m.watchCmd.Process != nil {
					m.watchCmd.Process.Kill()
					m.watchCmd.Wait()
				}
				m.watchCmd = nil
				return m, nil
			}
			m.watchLogs += line + "\n"
			m.logs = m.watchLogs
			m.logOffset = len(strings.Split(m.watchLogs, "\n"))
			return m, watchTick()
		default:
			return m, watchTick()
		}
	case prsMsg:
		m.prs = msg
		reverseSlice(m.prs)
		m.loading = false
		m.selectedPR = 0
		m.prOffset = 0
		m.prPage = 0
		if len(m.prs) > 0 && m.prScope != "global" {
			return m, fetchPRChecks(m.currentRepo.NameWithOwner, m.prs[0].Number)
		}
		return m, nil
	case checksMsg:
		m.checks = msg
		return m, nil
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.statusTimer > 0 {
		m.statusTimer--
	}

	switch m.view {
	case viewLogs:
		switch key {
		case "q", "esc":
			if m.watching {
				m.watching = false
				if m.watchCh != nil {
					close(m.watchCh)
				}
				if m.watchCmd != nil && m.watchCmd.Process != nil {
					m.watchCmd.Process.Kill()
					m.watchCmd.Wait()
				}
				m.watchCh = nil
				m.watchCmd = nil
				return m, nil
			}
			m.view = viewMain
			return m, nil
		case "g":
			m.logOffset = 0
			return m, nil
		case "G":
			m.logOffset = len(strings.Split(m.logs, "\n"))
			return m, nil
		case "j", "down":
			m.logOffset++
			return m, nil
		case "k", "up":
			if m.logOffset > 0 {
				m.logOffset--
			}
			return m, nil
		case "ctrl+u":
			m.logOffset -= m.height / 2
			if m.logOffset < 0 {
				m.logOffset = 0
			}
			return m, nil
		case "ctrl+d":
			m.logOffset += m.height / 2
			return m, nil
		}
		return m, nil
	case viewRepo:
		if msg.Type == tea.KeyRunes {
			m.repoFilter += key
			m.selectedRepo = 0
			return m, nil
		}
		switch key {
		case "q", "esc":
			m.view = viewMain
			return m, nil
		case "backspace":
			if len(m.repoFilter) > 0 {
				m.repoFilter = m.repoFilter[:len(m.repoFilter)-1]
				m.selectedRepo = 0
			}
			return m, nil
		case "j", "down":
			vis := m.visibleRepos()
			if m.selectedRepo < len(vis)-1 {
				m.selectedRepo++
			}
			return m, nil
		case "k", "up":
			if m.selectedRepo > 0 {
				m.selectedRepo--
			}
			return m, nil
		case "g":
			m.selectedRepo = 0
			m.repoOffset = 0
			return m, nil
		case "G":
			vis := m.visibleRepos()
			if len(vis) > 0 {
				m.selectedRepo = len(vis) - 1
			}
			return m, nil
		case "ctrl+u":
			half := (m.height - 8) / 2
			if half < 1 {
				half = 1
			}
			m.selectedRepo -= half
			if m.selectedRepo < 0 {
				m.selectedRepo = 0
			}
			return m, nil
		case "ctrl+d":
			vis := m.visibleRepos()
			half := (m.height - 8) / 2
			if half < 1 {
				half = 1
			}
			m.selectedRepo += half
			if m.selectedRepo >= len(vis) {
				m.selectedRepo = len(vis) - 1
			}
			return m, nil
		case "enter":
			vis := m.visibleRepos()
			if len(vis) > 0 && m.selectedRepo < len(vis) {
				m.currentRepo = vis[m.selectedRepo]
				m.repoFilter = ""
				m.view = viewMain
				m.selectedRun = 0
				m.selectedJob = 0
				return m, fetchRunsForRepo(m.currentRepo.NameWithOwner)
			}
			return m, nil
		}
		return m, nil
	case viewPR:
		switch key {
		case "q", "esc":
			m.view = viewMain
			return m, nil
		case "r":
			m.loading = true
			m.err = nil
			m.prPage = 0
			m.prOffset = 0
			if m.prScope == "global" {
				return m, fetchGlobalPRs("")
			} else if m.myPRsOnly {
				return m, fetchMyPRs(m.currentRepo.NameWithOwner)
			}
			return m, fetchPRs(m.currentRepo.NameWithOwner)
		case "s":
			if m.prScope == "repo" {
				m.prScope = "global"
			} else {
				m.prScope = "repo"
			}
			m.loading = true
			m.err = nil
			m.selectedPR = 0
			m.prPage = 0
			m.prOffset = 0
			if m.prScope == "global" {
				return m, fetchGlobalPRs("")
			} else if m.myPRsOnly {
				return m, fetchMyPRs(m.currentRepo.NameWithOwner)
			}
			return m, fetchPRs(m.currentRepo.NameWithOwner)
		case "m":
			m.myPRsOnly = !m.myPRsOnly
			m.loading = true
			m.err = nil
			m.selectedPR = 0
			m.prPage = 0
			m.prOffset = 0
			if m.prScope == "global" {
				return m, fetchGlobalPRs("")
			} else if m.myPRsOnly {
				return m, fetchMyPRs(m.currentRepo.NameWithOwner)
			}
			return m, fetchPRs(m.currentRepo.NameWithOwner)
		case "n":
			if m.prPageSize > 0 {
				totalPages := (len(m.prs) + m.prPageSize - 1) / m.prPageSize
				if m.prPage < totalPages-1 {
					m.prPage++
					m.selectedPR = m.prPage * m.prPageSize
					m.prOffset = 0
					if len(m.prs) > 0 && m.prScope != "global" {
						idx := m.selectedPR
						if idx >= len(m.prs) {
							idx = len(m.prs) - 1
						}
						return m, fetchPRChecks(m.currentRepo.NameWithOwner, m.prs[idx].Number)
					}
				}
			}
			return m, nil
		case "p":
			if m.prPage > 0 {
				m.prPage--
				m.selectedPR = m.prPage * m.prPageSize
				m.prOffset = 0
				if len(m.prs) > 0 && m.prScope != "global" {
					return m, fetchPRChecks(m.currentRepo.NameWithOwner, m.prs[m.selectedPR].Number)
				}
			}
			return m, nil
		case "j", "down":
			if m.selectedPR < len(m.prs)-1 {
				m.selectedPR++
				m.prPage = m.selectedPR / m.prPageSize
				if len(m.prs) > 0 && m.prScope != "global" {
					return m, fetchPRChecks(m.currentRepo.NameWithOwner, m.prs[m.selectedPR].Number)
				}
			}
			return m, nil
		case "k", "up":
			if m.selectedPR > 0 {
				m.selectedPR--
				m.prPage = m.selectedPR / m.prPageSize
				if len(m.prs) > 0 && m.prScope != "global" {
					return m, fetchPRChecks(m.currentRepo.NameWithOwner, m.prs[m.selectedPR].Number)
				}
			}
			return m, nil
		case "g":
			m.selectedPR = 0
			m.prPage = 0
			if len(m.prs) > 0 && m.prScope != "global" {
				return m, fetchPRChecks(m.currentRepo.NameWithOwner, m.prs[0].Number)
			}
			return m, nil
		case "G":
			if len(m.prs) > 0 {
				m.selectedPR = len(m.prs) - 1
				m.prPage = m.selectedPR / m.prPageSize
				if m.prScope != "global" {
					return m, fetchPRChecks(m.currentRepo.NameWithOwner, m.prs[m.selectedPR].Number)
				}
			}
			return m, nil
		case "b":
			if len(m.prs) > 0 {
				pr := m.prs[m.selectedPR]
				if m.prScope == "global" && pr.URL != "" {
					go func(url string) {
						_ = gh.OpenBrowser(url)
					}(pr.URL)
				} else {
					go func(n int) {
						_ = gh.BrowsePR(m.currentRepo.NameWithOwner, n)
					}(pr.Number)
				}
			}
			return m, nil
		case "enter":
			if len(m.prs) > 0 {
				pr := m.prs[m.selectedPR]
				if m.prScope == "global" && pr.URL != "" {
					go func(url string) {
						_ = gh.OpenBrowser(url)
					}(pr.URL)
				} else {
					go func(n int) {
						_ = gh.BrowsePR(m.currentRepo.NameWithOwner, n)
					}(pr.Number)
				}
			}
			return m, nil
		}
		return m, nil
	case viewMain:
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "o":
			m.view = viewRepo
			m.repoFilter = ""
			m.selectedRepo = 0
			return m, fetchRepos
		case "p":
			m.view = viewPR
			m.prs = nil
			m.checks = nil
			m.selectedPR = 0
			m.prOffset = 0
			m.myPRsOnly = false
			m.prScope = "repo"
			m.prPage = 0
			m.prPageSize = 8
			return m, fetchPRs(m.currentRepo.NameWithOwner)
		case "tab":
			if m.focus == focusRuns {
				if len(m.detail.Jobs) > 0 {
					m.focus = focusJobs
				}
			} else {
				m.focus = focusRuns
			}
			return m, nil
		case "r":
			m.loading = true
			m.err = nil
			return m, fetchRunsForRepo(m.currentRepo.NameWithOwner)
		case "w":
			if !m.watching && len(m.runs) > 0 {
				run := m.runs[m.selectedRun]
				if run.Status == "in_progress" || run.Status == "queued" {
					return m, startWatch(m.currentRepo.NameWithOwner, strconv.Itoa(run.DatabaseID))
				}
			}
			return m, nil
		case "l":
			if len(m.runs) > 0 && len(m.detail.Jobs) > 0 {
				run := m.runs[m.selectedRun]
				job := m.detail.Jobs[m.selectedJob]
				m.view = viewLogs
				return m, fetchLogsForRepo(m.currentRepo.NameWithOwner, strconv.Itoa(run.DatabaseID), job.ID)
			}
			return m, nil
		case "c":
			if len(m.runs) > 0 {
				run := m.runs[m.selectedRun]
				return m, cancelRunForRepo(m.currentRepo.NameWithOwner, strconv.Itoa(run.DatabaseID))
			}
			return m, nil
		case "R":
			if len(m.runs) > 0 {
				run := m.runs[m.selectedRun]
				return m, rerunRunForRepo(m.currentRepo.NameWithOwner, strconv.Itoa(run.DatabaseID))
			}
			return m, nil
		case "W":
			if len(m.runs) > 0 {
				run := m.runs[m.selectedRun]
				go func(id string) {
					_ = gh.BrowseRun(m.currentRepo.NameWithOwner, id)
				}(strconv.Itoa(run.DatabaseID))
				return m, nil
			}
			return m, nil
		case "j", "down":
			if m.focus == focusRuns {
				if m.selectedRun < len(m.runs)-1 {
					m.selectedRun++
					if len(m.runs) > 0 {
						return m, fetchDetailForRepo(m.currentRepo.NameWithOwner, strconv.Itoa(m.runs[m.selectedRun].DatabaseID))
					}
				}
			} else {
				if len(m.detail.Jobs) > 0 && m.selectedJob < len(m.detail.Jobs)-1 {
					m.selectedJob++
				}
			}
			return m, nil
		case "k", "up":
			if m.focus == focusRuns {
				if m.selectedRun > 0 {
					m.selectedRun--
					if len(m.runs) > 0 {
						return m, fetchDetailForRepo(m.currentRepo.NameWithOwner, strconv.Itoa(m.runs[m.selectedRun].DatabaseID))
					}
				}
			} else {
				if m.selectedJob > 0 {
					m.selectedJob--
				}
			}
			return m, nil
		case "g":
			if m.focus == focusRuns {
				m.selectedRun = 0
				if len(m.runs) > 0 {
					return m, fetchDetailForRepo(m.currentRepo.NameWithOwner, strconv.Itoa(m.runs[m.selectedRun].DatabaseID))
				}
			} else {
				m.selectedJob = 0
			}
			return m, nil
		case "G":
			if m.focus == focusRuns {
				if len(m.runs) > 0 {
					m.selectedRun = len(m.runs) - 1
					return m, fetchDetailForRepo(m.currentRepo.NameWithOwner, strconv.Itoa(m.runs[m.selectedRun].DatabaseID))
				}
			} else {
				if len(m.detail.Jobs) > 0 {
					m.selectedJob = len(m.detail.Jobs) - 1
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	var b strings.Builder

	switch m.view {
	case viewLogs:
		b.WriteString(m.renderLogs())
	case viewRepo:
		b.WriteString(m.renderRepoSelector())
	case viewPR:
		b.WriteString(m.renderPRView())
	default:
		b.WriteString(m.renderMain())
	}

	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(failureStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	if m.statusTimer > 0 && m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(successStyle.Render(m.statusMsg))
	}

	output := b.String()
	if m.height > 0 {
		output = lipgloss.NewStyle().Height(m.height).Render(output)
	}
	return output
}

func (m model) renderMain() string {
	splitW := int(float64(m.width) * 0.50)
	if splitW < 25 {
		splitW = 25
	}
	detailW := m.width - splitW - 4

	runsPanel := m.renderRuns(splitW)
	jobsPanel := m.renderJobs(detailW)

	main := lipgloss.JoinHorizontal(lipgloss.Top, runsPanel, jobsPanel)

	repoLabel := mutedStyle.Render(fmt.Sprintf("Repo: %s  |  ", m.currentRepo.NameWithOwner))
	help := helpStyle.Render("r:refresh  o:change repo  q:quit")
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, repoLabel, help)

	output := lipgloss.JoinVertical(lipgloss.Top, main, bottom)
	if m.height > 0 {
		output = lipgloss.NewStyle().Height(m.height).Render(output)
	}
	return output
}

func (m model) renderRuns(w int) string {
	title := titleStyle.Render(fmt.Sprintf(" Workflow Runs (%d) ", len(m.runs)))
	header := headerStyle.Render(fmt.Sprintf(" %-10s %s %s %s", "ID", "Workflow", "Status", "Branch"))

	allRows := make([]string, 0, len(m.runs))
	for i, run := range m.runs {
		cursor := " "
		if i == m.selectedRun && m.focus == focusRuns {
			cursor = ">"
		}
		id := strconv.Itoa(run.DatabaseID)
		if len(id) > 10 {
			id = id[:10]
		}
		workflow := run.Workflow
		status := formatStatus(run.Status, run.Conclusion)
		branch := run.HeadBranch
		if len(branch) > 20 {
			branch = branch[:20]
		}

		row := fmt.Sprintf("%s %-10s %s %s %s", cursor, id, workflow, status, branch)
		if i == m.selectedRun && m.focus == focusRuns {
			row = selectedStyle.Render(row)
		}
		allRows = append(allRows, row)
	}

	listHeight := m.height - 8
	if listHeight < 2 {
		listHeight = 2
	}

	if m.selectedRun < m.runsOffset {
		m.runsOffset = m.selectedRun
	}
	if m.selectedRun >= m.runsOffset+listHeight {
		m.runsOffset = m.selectedRun - listHeight + 1
	}
	if m.runsOffset < 0 {
		m.runsOffset = 0
	}
	maxOffset := len(allRows) - listHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.runsOffset > maxOffset {
		m.runsOffset = maxOffset
	}

	end := m.runsOffset + listHeight
	if end > len(allRows) {
		end = len(allRows)
	}
	visibleRows := allRows[m.runsOffset:end]

	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, visibleRows...)...)
	content = lipgloss.NewStyle().Width(w - 2).Render(content)

	borderStyle := unfocusBorderStyle
	if m.focus == focusRuns {
		borderStyle = focusBorderStyle
	}

	return borderStyle.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, title, content))
}

func (m model) renderJobs(w int) string {
	if len(m.runs) == 0 {
		return unfocusBorderStyle.Width(w).Render(headerStyle.Render(" Jobs ") + "\nNo runs selected")
	}

	run := m.runs[m.selectedRun]
	detail := m.detail

	title := titleStyle.Render(fmt.Sprintf(" Run #%s ", strconv.Itoa(run.DatabaseID)))

	var info []string
	info = append(info, fmt.Sprintf("Workflow: %s", run.Workflow))
	info = append(info, fmt.Sprintf("Branch:   %s", run.HeadBranch))
	info = append(info, fmt.Sprintf("Event:    %s", run.Event))
	info = append(info, fmt.Sprintf("Status:   %s", formatStatus(run.Status, run.Conclusion)))
	info = append(info, fmt.Sprintf("Created:  %s", formatTime(run.CreatedAt)))
	info = append(info, fmt.Sprintf("URL:      %s", run.URL))

	infoPanel := lipgloss.JoinVertical(lipgloss.Left, info...)
	infoPanel = lipgloss.NewStyle().Width(w - 2).Render(infoPanel)

	var jobsContent []string
	if len(detail.Jobs) == 0 {
		jobsContent = append(jobsContent, mutedStyle.Render("No jobs"))
	} else {
		sort.Slice(detail.Jobs, func(i, j int) bool {
			return detail.Jobs[i].Name < detail.Jobs[j].Name
		})
		jobHeader := fmt.Sprintf("%-25s %s", "Job", "Status")
		allJobRows := []string{headerStyle.Render(jobHeader)}
		for i, job := range detail.Jobs {
			cursor := " "
			if i == m.selectedJob && m.focus == focusJobs {
				cursor = ">"
			}
			row := fmt.Sprintf("%s %-24s %s", cursor, job.Name, formatStatus(job.Status, job.Conclusion))
			if i == m.selectedJob && m.focus == focusJobs {
				row = selectedStyle.Render(row)
			}
			allJobRows = append(allJobRows, row)
		}

		listHeight := m.height - 14
		if listHeight < 2 {
			listHeight = 2
		}

		if m.selectedJob < m.jobsOffset {
			m.jobsOffset = m.selectedJob
		}
		if m.selectedJob >= m.jobsOffset+listHeight {
			m.jobsOffset = m.selectedJob - listHeight + 1
		}
		if m.jobsOffset < 0 {
			m.jobsOffset = 0
		}
		maxOffset := len(allJobRows) - listHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.jobsOffset > maxOffset {
			m.jobsOffset = maxOffset
		}

		end := m.jobsOffset + listHeight
		if end > len(allJobRows) {
			end = len(allJobRows)
		}
		visibleJobRows := allJobRows[m.jobsOffset:end]

		jobsContent = append(jobsContent, lipgloss.JoinVertical(lipgloss.Left, visibleJobRows...))
	}

	jobsPanel := lipgloss.NewStyle().Width(w - 2).Render(lipgloss.JoinVertical(lipgloss.Left, jobsContent...))
	body := lipgloss.JoinVertical(lipgloss.Left, infoPanel, "", jobsPanel)

	borderStyle := unfocusBorderStyle
	if m.focus == focusJobs {
		borderStyle = focusBorderStyle
	}

	return borderStyle.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, title, body))
}

func (m model) visibleRepos() []gh.Repo {
	if m.repoFilter == "" {
		return m.repos
	}
	filter := strings.ToLower(m.repoFilter)
	var out []gh.Repo
	for _, r := range m.repos {
		if strings.Contains(strings.ToLower(r.NameWithOwner), filter) || strings.Contains(strings.ToLower(r.Description), filter) {
			out = append(out, r)
		}
	}
	return out
}

func (m model) renderRepoSelector() string {
	title := titleStyle.Render(" Select Repository ")
	filterLine := helpStyle.Render(fmt.Sprintf("Filter: %s", m.repoFilter))

	vis := m.visibleRepos()
	if m.selectedRepo >= len(vis) {
		m.selectedRepo = len(vis) - 1
	}
	if m.selectedRepo < 0 {
		m.selectedRepo = 0
	}

	header := headerStyle.Render(fmt.Sprintf(" %-30s %s", "Repository", "Description"))

	repoRows := make([]string, 0, len(vis))
	for _, repo := range vis {
		desc := repo.Description
		if len(desc) > 40 {
			desc = desc[:40]
		}
		repoRows = append(repoRows, fmt.Sprintf(" %-30s %s", repo.NameWithOwner, desc))
	}

	listHeight := m.height - 8
	if listHeight < 2 {
		listHeight = 2
	}

	if m.selectedRepo < m.repoOffset {
		m.repoOffset = m.selectedRepo
	}
	if m.selectedRepo >= m.repoOffset+listHeight {
		m.repoOffset = m.selectedRepo - listHeight + 1
	}
	if m.repoOffset < 0 {
		m.repoOffset = 0
	}
	maxOffset := len(repoRows) - listHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.repoOffset > maxOffset {
		m.repoOffset = maxOffset
	}

	end := m.repoOffset + listHeight
	if end > len(repoRows) {
		end = len(repoRows)
	}
	visibleRows := repoRows[m.repoOffset:end]

	var displayRows []string
	displayRows = append(displayRows, header)
	for i, row := range visibleRows {
		idx := m.repoOffset + i
		if idx == m.selectedRepo {
			displayRows = append(displayRows, selectedStyle.Render(">"+row))
		} else {
			displayRows = append(displayRows, " "+row)
		}
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left, displayRows...)
	listContent = lipgloss.NewStyle().Width(m.width - 4).Render(listContent)

	scrollInfo := ""
	if len(repoRows) > listHeight {
		scrollInfo = helpStyle.Render(fmt.Sprintf(" %d/%d ", m.repoOffset+1, len(repoRows)-listHeight+1))
	}

	help := helpStyle.Render("type:filter  j/k:move  enter:select  esc:back  g/G:top/bot  ctrl+u/d:half-page")

	return panelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, filterLine, "", listContent, "", help, scrollInfo))
}

func (m model) renderPRView() string {
	if m.width < 60 {
		return m.renderPRList(m.width - 2)
	}

	splitW := int(float64(m.width) * 0.45)
	if splitW < 25 {
		splitW = 25
	}
	detailW := m.width - splitW - 4
	if detailW < 20 {
		detailW = 20
	}

	prPanel := m.renderPRList(splitW)
	checkPanel := m.renderPRChecks(detailW)

	main := lipgloss.JoinHorizontal(lipgloss.Top, prPanel, checkPanel)

	filterLabel := ""
	if m.prScope == "global" {
		filterLabel = mutedStyle.Render("Scope: global  |  ")
	} else if m.myPRsOnly {
		filterLabel = mutedStyle.Render("Filter: mine  |  ")
	}
	help := helpStyle.Render("r:refresh  m:my prs  s:scope  b:open  q:back  n/p:page")
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, filterLabel, help)

	return lipgloss.JoinVertical(lipgloss.Top, main, bottom)
}

func (m model) renderPRList(w int) string {
	totalItems := len(m.prs)
	pageSize := m.prPageSize
	if pageSize <= 0 {
		pageSize = 8
	}
	totalPages := (totalItems + pageSize - 1) / pageSize
	if totalPages <= 0 {
		totalPages = 1
	}
	if m.prPage >= totalPages {
		m.prPage = totalPages - 1
	}
	if m.prPage < 0 {
		m.prPage = 0
	}

	title := titleStyle.Render(fmt.Sprintf(" Pull Requests (%d) ", totalItems))
	if m.prScope == "global" {
		title = titleStyle.Render(" Global Pull Requests ")
	} else if m.myPRsOnly {
		title = titleStyle.Render(" My Pull Requests ")
	}

	headerRepo := "Branch"
	if m.prScope == "global" {
		headerRepo = "Repo"
	}
	header := headerStyle.Render(fmt.Sprintf(" %-6s %-28s %-12s %s", "#", "Title", "State", headerRepo))

	allRows := make([]string, 0, len(m.prs))
	for i, pr := range m.prs {
		cursor := " "
		if i == m.selectedPR {
			cursor = ">"
		}
		titleText := pr.Title
		if len(titleText) > 28 {
			titleText = titleText[:28]
		}
		state := pr.State
		if pr.IsDraft {
			state = "draft"
		}

		repoOrBranch := ""
		if m.prScope == "global" {
			repoOrBranch = pr.Repository.NameWithOwner
		} else {
			repoOrBranch = pr.HeadRefName
		}
		if len(repoOrBranch) > 18 {
			repoOrBranch = repoOrBranch[:18]
		}

		row := fmt.Sprintf(" %s%-6s %-28s %-12s %s", cursor, strconv.Itoa(pr.Number), titleText, state, repoOrBranch)
		if i == m.selectedPR {
			row = selectedStyle.Render(row)
		}
		allRows = append(allRows, row)
	}

	listHeight := m.height - 6
	if listHeight < 2 {
		listHeight = 2
	}

	pageStart := m.prPage * pageSize
	pageEnd := pageStart + pageSize
	if pageEnd > totalItems {
		pageEnd = totalItems
	}
	pageItems := allRows[pageStart:pageEnd]

	if m.selectedPR < pageStart {
		m.selectedPR = pageStart
	}
	if m.selectedPR >= pageEnd {
		m.selectedPR = pageEnd - 1
	}
	if m.selectedPR < 0 {
		m.selectedPR = 0
	}
	displayedRows := pageItems
	if len(pageItems) > listHeight {
		if m.prOffset < 0 {
			m.prOffset = 0
		}
		maxOffset := len(pageItems) - listHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.prOffset > maxOffset {
			m.prOffset = maxOffset
		}

		selectedInPage := m.selectedPR - pageStart
		if selectedInPage < m.prOffset {
			m.prOffset = selectedInPage
		}
		if selectedInPage >= m.prOffset+listHeight {
			m.prOffset = selectedInPage - listHeight + 1
		}

		end := m.prOffset + listHeight
		if end > len(pageItems) {
			end = len(pageItems)
		}
		displayedRows = pageItems[m.prOffset:end]
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, displayedRows...)...)
	listContent = lipgloss.NewStyle().Width(w - 2).Render(listContent)

	pageInfo := ""
	if totalItems > 0 {
		pageInfo = helpStyle.Render(fmt.Sprintf(" Page %d/%d ", m.prPage+1, totalPages))
	}
	help := helpStyle.Render("r:refresh  m:my prs  s:scope  b:open  q:back  n/p:page")

	borderStyle := focusBorderStyle
	panel := borderStyle.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, title, listContent, "", help, pageInfo))
	if m.height > 0 {
		panel = lipgloss.NewStyle().Height(m.height).Render(panel)
	}
	return panel
}

func (m model) renderPRChecks(w int) string {
	if len(m.prs) == 0 {
		return unfocusBorderStyle.Width(w).Render(headerStyle.Render(" Pull Request Details ") + "\nNo PRs")
	}

	pr := m.prs[m.selectedPR]
	title := titleStyle.Render(fmt.Sprintf(" PR #%d ", pr.Number))

	var info []string
	info = append(info, fmt.Sprintf("Title:   %s", pr.Title))
	info = append(info, fmt.Sprintf("State:   %s", strings.ToLower(pr.State)))
	if pr.IsDraft {
		info = append(info, mutedStyle.Render("Draft:   Yes"))
	}
	if m.prScope == "global" {
		info = append(info, fmt.Sprintf("Repo:    %s", pr.Repository.NameWithOwner))
	} else {
		info = append(info, fmt.Sprintf("Branch:  %s -> %s", pr.HeadRefName, pr.BaseRefName))
	}
	info = append(info, fmt.Sprintf("Author:  %s", pr.Author.Login))
	info = append(info, fmt.Sprintf("URL:     %s", pr.URL))

	infoPanel := lipgloss.JoinVertical(lipgloss.Left, info...)
	infoPanel = lipgloss.NewStyle().Width(w - 2).Render(infoPanel)

	var checksContent []string
	if len(m.checks) == 0 {
		checksContent = append(checksContent, mutedStyle.Render("No checks"))
	} else {
		checkHeader := fmt.Sprintf("%-30s %-10s %s", "Check", "State", "Event")
		checksContent = append(checksContent, headerStyle.Render(checkHeader))
		for _, check := range m.checks {
			state := check.State
			if state == "" {
				state = check.Event
			}
			row := fmt.Sprintf(" %-29s %-10s %s", check.Name, state, check.Event)
			checksContent = append(checksContent, row)
		}
	}

	checksPanel := lipgloss.NewStyle().Width(w - 2).Render(lipgloss.JoinVertical(lipgloss.Left, checksContent...))
	body := lipgloss.JoinVertical(lipgloss.Left, infoPanel, "", checksPanel)

	borderStyle := unfocusBorderStyle
	panel := borderStyle.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, title, body))
	if m.height > 0 {
		panel = lipgloss.NewStyle().Height(m.height).Render(panel)
	}
	return panel
}

func (m model) renderLogs() string {
	title := titleStyle.Render(" Logs ")
	if m.watching {
		title = titleStyle.Render(" Watching (live) ")
	}
	lines := strings.Split(m.logs, "\n")

	if m.logOffset >= len(lines) {
		m.logOffset = len(lines) - 1
	}
	if m.logOffset < 0 {
		m.logOffset = 0
	}

	viewH := m.height - 6
	if viewH < 1 {
		viewH = 1
	}

	end := m.logOffset + viewH
	if end > len(lines) {
		end = len(lines)
	}

	visible := lines[m.logOffset:end]
	content := strings.Join(visible, "\n")
	content = lipgloss.NewStyle().Width(m.width - 4).Render(content)

	scrollInfo := fmt.Sprintf(" %d/%d ", m.logOffset+1, len(lines))
	scrollBar := lipgloss.NewStyle().Align(lipgloss.Right).Render(scrollInfo)

	help := "q:back g:top G:bot j/k:scroll ctrl+u/d:half-page"
	if m.watching {
		help = "q:stop watch"
	}
	helpLine := helpStyle.Render(help)

	return panelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, content, "", helpLine, scrollBar))
}

func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
