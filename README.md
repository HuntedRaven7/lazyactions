# lazyactions

A lazygit-inspired TUI for GitHub Actions and PR's built with Go + Bubble Tea + lipgloss.

## Prerequisites

- [Go 1.21+](https://go.dev/dl/)
- [GitHub CLI (`gh`)](https://cli.github.com/) installed and authenticated (`gh auth login`)

## Build

```bash
go build -o lazyactions .
```

Or install to `$GOPATH/bin`:

```bash
go install .
```

## Usage

```bash
./lazyactions
```

## Keybindings

### Global
| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `r` | Refresh run list |
| `o` | Open repository selector |
| `p` | Open pull request dashboard (current repo) |
| `P` (shift+p) | Open your PRs across all repos |
| `Tab` | Switch focus between runs list and jobs panel |

### Runs panel
| Key | Action |
|-----|--------|
| `j` / `↓` | Next run |
| `k` / `↑` | Previous run |
| `g` | Top of list |
| `G` | Bottom of list |
| `c` | Cancel selected run |
| `R` (shift+r) | Rerun selected run |
| `W` (shift+w) | Open selected run in browser |
| `w` | Watch in-progress/queued run live |

### Jobs panel
| Key | Action |
|-----|--------|
| `j` / `↓` | Next job |
| `k` / `↑` | Previous job |
| `l` | View logs for selected job |
| `Enter` | View logs for selected job |

### Log viewer
| Key | Action |
|-----|--------|
| `q` / `Esc` | Back to main view (or stop watch) |
| `j` / `↓` | Scroll down |
| `k` / `↑` | Scroll up |
| `g` | Top of logs |
| `G` | Bottom of logs |
| `Ctrl+U` | Half page up |
| `Ctrl+D` | Half page down |

### Repository selector
| Key | Action |
|-----|--------|
| `type` | Filter repositories by name or description |
| `backspace` | Delete filter character |
| `j` / `↓` | Next repository |
| `k` / `↑` | Previous repository |
| `Enter` | Select repository |
| `q` / `Esc` | Cancel and go back |

### Pull requests
| Key | Action |
|-----|--------|
| `j` / `↓` | Next PR |
| `k` / `↑` | Previous PR |
| `g` | Top of list |
| `G` | Bottom of list |
| `b` / `Enter` | Open selected PR in browser |
| `d` | View PR detail (body, comments, reviews) |
| `D` | View PR diff |
| `c` | Checkout selected PR |
| `C` (shift+c) | Close selected PR |
| `M` (shift+m) | Merge selected PR |
| `a` | Approve selected PR |
| `A` (shift+a) | Mark selected PR as ready |
| `r` | Refresh PR list |
| `m` | Toggle showing only your PRs |
| `s` | Toggle between repo-specific and GitHub-wide PR search |
| `q` / `Esc` | Back to main view |

## Features

- Browse recent workflow runs across your repo
- View run details and job statuses
- Stream logs for any job
- Cancel or rerun runs directly from the TUI
- Switch between repositories with a searchable repo selector
- View open PRs and their action check statuses
- Filter PR list to show only your PRs
- Search PRs GitHub-wide or within a specific repo
- View rich PR details including body, comments, and reviews
- View PR code diffs in-terminal
- Open runs and PRs directly in your browser
- Checkout, close, merge, approve, and mark PRs ready from the TUI
- Color-coded status indicators (success / failure / running / queued)
