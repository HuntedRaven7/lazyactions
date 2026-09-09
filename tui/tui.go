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
	"github.com/robin/lazyactions/backend"
	"github.com/robin/lazyactions/gh"
)

type viewType int

const (
	viewMain viewType = iota
	viewLogs
	viewRepo
	viewPR
	viewBackend
	viewBackendSelect
	viewNotifications
)

type model struct {
	backend     backend.Backend
	backendTab  string
	backendTabs []string
	resources   []backend.Resource
	selectedIdx int
	offset      int
	filter      string
	details     backend.Resource
	backends    []backend.Backend
	selectedBackend int
	previousView viewType
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

	notifications []notification
	notification  *notification
	notificationTimer int

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

	prDetail      gh.PRDetail
	prDiff        string
	prDetailOffset int
	prDiffOffset   int

	helpVisible bool
}

type notification struct {
	message string
	level   string
	time    time.Time
}

const (
	notificationSuccess = "success"
	notificationError   = "error"
	notificationWarning = "warning"
	notificationInfo    = "info"
)

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
	m := model{}
	m.backends = availableBackends()
	m.view = viewBackendSelect
	m.previousView = viewBackendSelect
	return m
}

func NewWithBackend(b backend.Backend) model {
	m := New()
	if b != nil {
		m.backend = b
		m.backendTabs = b.Tabs()
		if len(m.backendTabs) > 0 {
			m.backendTab = m.backendTabs[0]
		}
		if b.Name() == "GitHub" {
			m.view = viewMain
		} else {
			m.view = viewBackend
		}
	}
	return m
}

func (m model) Init() tea.Cmd {
	if m.view == viewBackendSelect {
		return nil
	}
	if m.backend != nil {
		return m.fetchBackendResources()
	}
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

func availableBackends() []backend.Backend {
	return []backend.Backend{
		backend.NewGitHub(""),
		backend.NewDocker("docker"),
		backend.NewDocker("podman"),
		backend.NewKubernetes(""),
		backend.NewHermes("", ""),
		backend.NewSSH("localhost", "root", 22),
	}
}

func (m model) switchBackend(b backend.Backend) (tea.Model, tea.Cmd) {
	m.previousView = m.view
	m.backend = b
	m.backendTabs = b.Tabs()
	if len(m.backendTabs) > 0 {
		m.backendTab = m.backendTabs[0]
	}
	m.resources = nil
	m.selectedIdx = 0
	m.offset = 0
	m.filter = ""
	m.details = backend.Resource{}
	if b.Name() == "GitHub" {
		m.view = viewMain
		return m, fetchCurrentRepo
	}
	m.view = viewBackend
	return m, m.fetchBackendResources()
}

func (m model) fetchBackendResources() tea.Cmd {
	return func() tea.Msg {
		if m.backend == nil || m.backendTab == "" {
			return errMsg{fmt.Errorf("no backend configured")}
		}
		resources, err := m.backend.Fetch(m.backendTab, m.filter)
		if err != nil {
			return errMsg{err}
		}
		return backendResourcesMsg(resources)
	}
}

func (m model) fetchBackendDetails() tea.Cmd {
	return func() tea.Msg {
		if m.backend == nil || m.backendTab == "" || len(m.resources) == 0 {
			return errMsg{fmt.Errorf("no resource selected")}
		}
		res := m.resources[m.selectedIdx]
		details, err := m.backend.Inspect(m.backendTab, res.ID)
		if err != nil {
			return errMsg{err}
		}
		return backendDetailsMsg(details)
	}
}

func (m model) runBackendAction(action string) tea.Cmd {
	return func() tea.Msg {
		if m.backend == nil || m.backendTab == "" || len(m.resources) == 0 {
			return errMsg{fmt.Errorf("no resource selected")}
		}
		res := m.resources[m.selectedIdx]
		out, err := m.backend.RunAction(m.backendTab, res.ID, action, nil)
		if err != nil {
			return errMsg{err}
		}
		return statusMsg(fmt.Sprintf("Action %s completed: %s", action, out))
	}
}

func (m model) runBackendActionForSelected(action string) tea.Cmd {
	if len(m.resources) == 0 || m.selectedIdx >= len(m.resources) {
		return nil
	}
	res := m.resources[m.selectedIdx]
	return func() tea.Msg {
		out, err := m.backend.RunAction(m.backendTab, res.ID, action, nil)
		if err != nil {
			return errMsg{err}
		}
		return statusMsg(fmt.Sprintf("Action %s completed: %s", action, out))
	}
}

func (m model) addNotification(message, level string) tea.Cmd {
	n := notification{
		message: message,
		level:   level,
		time:    time.Now(),
	}
	return func() tea.Msg {
		return notificationMsg{notification: n}
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

func fetchMyGlobalPRs() tea.Cmd {
	return func() tea.Msg {
		prs, err := gh.SearchMyPRs(100)
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

func notificationTick() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return notificationTickMsg{}
	})
}

type notificationTickMsg struct{}

type currentRepoMsg gh.Repo
type runsMsg []gh.Run
type detailMsg gh.RunDetail
type logsMsg string
type errMsg struct{ err error }
type statusMsg string
type notificationMsg struct {
		notification notification
	}
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
type backendResourcesMsg []backend.Resource
type backendDetailsMsg backend.Resource

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
	case notificationMsg:
		m.notification = &msg.notification
		m.notificationTimer = 180
		m.notifications = append(m.notifications, msg.notification)
		return m, notificationTick()
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
	case notificationTickMsg:
		if m.notificationTimer > 0 {
			m.notificationTimer--
		}
		if m.notificationTimer <= 0 {
			m.notification = nil
		}
		return m, nil
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
	case backendResourcesMsg:
		m.resources = msg
		m.loading = false
		m.selectedIdx = 0
		m.offset = 0
		if len(m.resources) > 0 {
			return m, m.fetchBackendDetails()
		}
		return m, nil
	case backendDetailsMsg:
		m.details = backend.Resource(msg)
		m.loading = false
		return m, nil
	}
	return m, nil
}

func (m model) renderBackendSelect() string {
	title := titleStyle.Render(" Select Backend ")
	var rows []string
	for i, b := range m.backends {
		cursor := " "
		if i == m.selectedBackend {
			cursor = ">"
		}
		row := fmt.Sprintf("%s %s", cursor, b.Name())
		if i == m.selectedBackend {
			row = selectedStyle.Render(row)
		}
		rows = append(rows, row)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	help := helpStyle.Render("j/k:move  enter:select  esc:back")
	return panelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, content, "", help))
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.statusTimer > 0 {
		m.statusTimer--
	}

	switch m.view {
	case viewLogs:
		switch key {
		case "?":
			m.helpVisible = !m.helpVisible
			return m, nil
		case "n":
			m.view = viewNotifications
			return m, nil
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
		case "?":
			m.helpVisible = !m.helpVisible
			return m, nil
		case "n":
			m.view = viewNotifications
			return m, nil
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
		case "?":
			m.helpVisible = !m.helpVisible
			return m, nil
		case "q", "esc":
			m.view = viewMain
			return m, nil
		case "r":
			m.loading = true
			m.err = nil
			m.prPage = 0
			m.prOffset = 0
			if m.prScope == "global" && m.myPRsOnly {
				return m, fetchMyGlobalPRs()
			} else if m.prScope == "global" {
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
			if m.prScope == "global" && m.myPRsOnly {
				return m, fetchMyGlobalPRs()
			} else if m.prScope == "global" {
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
			if m.prScope == "global" && m.myPRsOnly {
				return m, fetchMyGlobalPRs()
			} else if m.prScope == "global" {
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
	case viewBackend:
		switch key {
		case "q", "esc":
			m.view = viewMain
			return m, nil
		case "?":
			m.helpVisible = !m.helpVisible
			return m, nil
		case "n":
			m.view = viewNotifications
			return m, nil
		case "b":
			m.backends = availableBackends()
			m.selectedBackend = 0
			m.previousView = m.view
			m.view = viewBackendSelect
			return m, nil
		case "r":
			m.loading = true
			m.err = nil
			return m, m.fetchBackendResources()
		case "1", "2", "3", "4", "5":
			idx, _ := strconv.Atoi(key)
			if idx-1 < len(m.backendTabs) {
				m.backendTab = m.backendTabs[idx-1]
				m.resources = nil
				m.selectedIdx = 0
				m.offset = 0
				m.filter = ""
				return m, m.fetchBackendResources()
			}
			return m, nil
		case "j", "down":
			if m.selectedIdx < len(m.resources)-1 {
				m.selectedIdx++
				if m.selectedIdx >= m.offset+listHeight() {
					m.offset++
				}
				return m, m.fetchBackendDetails()
			}
			return m, nil
		case "k", "up":
			if m.selectedIdx > 0 {
				m.selectedIdx--
				if m.selectedIdx < m.offset {
					m.offset--
				}
				return m, m.fetchBackendDetails()
			}
			return m, nil
		case "g":
			m.selectedIdx = 0
			m.offset = 0
			return m, m.fetchBackendDetails()
		case "G":
			if len(m.resources) > 0 {
				m.selectedIdx = len(m.resources) - 1
				return m, m.fetchBackendDetails()
			}
			return m, nil
		case "enter":
			if len(m.resources) > 0 {
				res := m.resources[m.selectedIdx]
				return m, func() tea.Msg {
					out, err := m.backend.Inspect(m.backendTab, res.ID)
					if err != nil {
						return errMsg{err}
					}
					return statusMsg(fmt.Sprintf("Inspect: %s", out.Name))
				}
			}
			return m, nil
		case "s":
			return m, m.runBackendActionForSelected("start")
		case "x":
			return m, m.runBackendActionForSelected("stop")
		case "R":
			return m, m.runBackendActionForSelected("restart")
		case "e":
			return m, m.runBackendActionForSelected("exec")
		case "l":
			return m, m.runBackendActionForSelected("logs")
		case "d":
			return m, m.runBackendActionForSelected("rm")
		case "i":
			return m, m.runBackendActionForSelected("inspect")
		}
		return m, nil
	case viewBackendSelect:
		switch key {
		case "q", "esc":
			if m.previousView == viewBackendSelect {
				return m, tea.Quit
			}
			m.view = m.previousView
			m.previousView = viewBackendSelect
			return m, nil
		case "n":
			m.view = viewNotifications
			return m, nil
		case "j", "down":
			if m.selectedBackend < len(m.backends)-1 {
				m.selectedBackend++
			}
			return m, nil
		case "k", "up":
			if m.selectedBackend > 0 {
				m.selectedBackend--
			}
			return m, nil
		case "enter":
			if len(m.backends) > 0 {
				return m.switchBackend(m.backends[m.selectedBackend])
			}
			return m, nil
		}
		return m, nil
	case viewNotifications:
		switch key {
		case "q", "esc", "n":
			m.view = viewMain
			return m, nil
		}
		return m, nil
	case viewMain:
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.helpVisible = !m.helpVisible
			return m, nil
		case "n":
			m.view = viewNotifications
			return m, nil
		case "B":
			m.backends = availableBackends()
			m.selectedBackend = 0
			m.previousView = m.view
			m.view = viewBackendSelect
			return m, nil
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
	case viewBackend:
		b.WriteString(m.renderBackend())
	case viewBackendSelect:
		b.WriteString(m.renderBackendSelect())
	case viewNotifications:
		b.WriteString(m.renderNotificationLog())
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

	if m.notification != nil {
		b.WriteString("\n")
		b.WriteString(m.renderNotification())
	}

	output := b.String()
	if m.height > 0 {
		output = lipgloss.NewStyle().Height(m.height).Render(output)
	}
	if m.helpVisible {
		output = lipgloss.JoinVertical(lipgloss.Top, output, m.renderStatusBar())
	}
	return output
}

func (m model) renderNotification() string {
	if m.notification == nil {
		return ""
	}
	style := successStyle
	switch m.notification.level {
	case notificationError:
		style = failureStyle
	case notificationWarning:
		style = pendingStyle
	case notificationInfo:
		style = mutedStyle
	}
	text := style.Render(fmt.Sprintf(" %s ", m.notification.message))
	return lipgloss.NewStyle().Width(m.width).Render(text)
}

func (m model) renderNotificationLog() string {
	if len(m.notifications) == 0 {
		return panelStyle.Render("No notifications")
	}
	title := titleStyle.Render(" Notifications ")
	var rows []string
	for i := len(m.notifications) - 1; i >= 0; i-- {
		n := m.notifications[i]
		style := successStyle
		switch n.level {
		case notificationError:
			style = failureStyle
		case notificationWarning:
			style = pendingStyle
		case notificationInfo:
			style = mutedStyle
		}
		rows = append(rows, style.Render(fmt.Sprintf("[%s] %s", n.time.Format("15:04:05"), n.message)))
	}
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	help := helpStyle.Render("q/esc:close")
	return panelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, content, "", help))
}

func (m model) renderStatusBar() string {
	mode := ""
	bindings := ""
	switch m.view {
	case viewMain:
		mode = "NORMAL"
		bindings = "r:refresh  o:repo  p:prs  P:my prs  B:backend  w:watch  l:logs  q:quit  n:notifications  ?:help"
	case viewLogs:
		mode = "LOGS"
		bindings = "q:back  g:top  G:bot  j/k:scroll  ctrl+u/d:half-page  n:notifications"
	case viewRepo:
		mode = "REPOS"
		bindings = "type:filter  j/k:move  enter:select  esc:back  g/G:top/bot  n:notifications"
	case viewPR:
		mode = "PRS"
		bindings = "j/k:move  b:open  d:detail  D:diff  c:checkout  C:close  M:merge  a:approve  A:ready  r:refresh  m:mine  s:scope  q:back  n:notifications"
	case viewBackend:
		mode = strings.ToUpper(m.backendTab)
		bindings = fmt.Sprintf("1-5:tabs  j/k:move  enter:inspect  s:start  x:stop  R:restart  e:exec  l:logs  d:delete  i:inspect  q:back  n:notifications  ?:help")
	case viewBackendSelect:
		mode = "BACKEND"
		bindings = "j/k:move  enter:select  esc:back  n:notifications"
	}
	label := titleStyle.Render(" " + mode + " ")
	keys := helpStyle.Render(" " + bindings + " ")
	if len(m.notifications) > 0 {
		bell := titleStyle.Render(fmt.Sprintf(" 🔔%d ", len(m.notifications)))
		keys = lipgloss.JoinHorizontal(lipgloss.Top, keys, bell)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, label, keys)
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
	help := helpStyle.Render("r:refresh  o:change repo  B:backend  q:quit")
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, repoLabel, help)

	output := lipgloss.JoinVertical(lipgloss.Top, main, bottom)
	if m.height > 0 {
		output = lipgloss.NewStyle().Height(m.height).Render(output)
	}
	return output
}

func listHeight() int {
	return 12
}

func (m model) renderBackend() string {
	if m.backend == nil {
		return panelStyle.Render("No backend configured")
	}

	title := titleStyle.Render(fmt.Sprintf(" %s ", m.backend.Name()))

	tabHeader := ""
	for i, tab := range m.backendTabs {
		style := mutedStyle
		if tab == m.backendTab {
			style = selectedStyle
		}
		tabHeader += style.Render(fmt.Sprintf(" %d:%s ", i+1, tab))
	}

	columns := m.backend.Columns(m.backendTab)
	header := ""
	if len(columns) > 0 {
		header = headerStyle.Render(formatColumns(columns))
	}

	var rows []string
	for i, res := range m.resources {
		cursor := " "
		if i == m.selectedIdx {
			cursor = ">"
		}
		row := fmt.Sprintf("%s %s", cursor, formatResourceRow(res, columns))
		if i == m.selectedIdx {
			row = selectedStyle.Render(row)
		}
		rows = append(rows, row)
	}

	listH := listHeight()
	if m.offset < 0 {
		m.offset = 0
	}
	maxOffset := len(rows) - listH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.selectedIdx < m.offset {
		m.offset = m.selectedIdx
	}
	if m.selectedIdx >= m.offset+listH {
		m.offset = m.selectedIdx - listH + 1
	}
	end := m.offset + listH
	if end > len(rows) {
		end = len(rows)
	}
	visible := rows[m.offset:end]

	body := ""
	if len(columns) > 0 {
		body = lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, visible...)...)
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left, visible...)
	}

	right := ""
	if len(m.resources) > 0 && m.selectedIdx < len(m.resources) {
		res := m.resources[m.selectedIdx]
		var details []string
		details = append(details, headerStyle.Render(" Details "))
		if res.Status != "" {
			details = append(details, fmt.Sprintf("Status: %s", res.Status))
		}
		for k, v := range res.Details {
			details = append(details, fmt.Sprintf("%s: %s", k, v))
		}
		if len(res.Actions) > 0 {
			details = append(details, "", headerStyle.Render(" Actions "))
			for i, action := range res.Actions {
				details = append(details, fmt.Sprintf("%d. %s", i+1, action))
			}
		}
		right = lipgloss.JoinVertical(lipgloss.Left, details...)
		right = lipgloss.NewStyle().Width(m.width/2 - 2).Render(right)
	}

	filterLine := ""
	if m.filter != "" {
		filterLine = mutedStyle.Render("Filter: " + m.filter)
	}

	help := helpStyle.Render("1-5:tabs  j/k:move  g/G:top/bot  enter:inspect  s:start  x:stop  R:restart  e:exec  l:logs  d:delete  i:inspect  q:back")

	leftPane := lipgloss.JoinVertical(lipgloss.Left, title, "", tabHeader, "", body)
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "", right)
	if filterLine != "" {
		content = lipgloss.JoinVertical(lipgloss.Top, content, "", filterLine)
	}
	content = lipgloss.JoinVertical(lipgloss.Top, content, "", help)

	if m.height > 0 {
		content = lipgloss.NewStyle().Height(m.height).Render(content)
	}
	return content
}

func formatColumns(columns []string) string {
	parts := make([]string, 0, len(columns))
	for _, c := range columns {
		parts = append(parts, fmt.Sprintf("%-16s", c))
	}
	return strings.Join(parts, " ")
}

func formatResourceRow(res backend.Resource, columns []string) string {
	if len(columns) == 0 {
		return res.Name
	}
	parts := make([]string, 0, len(columns))
	for _, c := range columns {
		parts = append(parts, truncate(res.Details[c], 16))
	}
	return strings.Join(parts, " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
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
	return RunWithBackend(nil)
}

func RunWithBackend(b backend.Backend) error {
	var initModel model
	if b != nil {
		initModel = NewWithBackend(b)
	} else {
		initModel = New()
	}
	p := tea.NewProgram(initModel, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
