package backend

import (
	"fmt"
	"io"
	"os/exec"
)

type sshBackend struct {
	user string
	host string
	port int
}

func NewSSH(host, user string, port int) Backend {
	if user == "" {
		user = "root"
	}
	if port == 0 {
		port = 22
	}
	return &sshBackend{host: host, user: user, port: port}
}

func (b *sshBackend) Name() string {
	return "SSH"
}

func (b *sshBackend) Tabs() []string {
	return []string{"host", "shell"}
}

func (b *sshBackend) Columns(tab string) []string {
	switch tab {
	case "host":
		return []string{"key", "value"}
	case "shell":
		return []string{"output"}
	default:
		return nil
	}
}

func (b *sshBackend) Fetch(tab, filter string) ([]Resource, error) {
	switch tab {
	case "host":
		return b.fetchHostInfo()
	case "shell":
		if filter == "" {
			return []Resource{}, nil
		}
		out, err := b.runSSH(filter)
		if err != nil {
			return nil, err
		}
		return []Resource{
			{
				ID:     "shell",
				Name:   "shell",
				Status: "",
				Details: map[string]string{
					"output": out,
				},
				Actions: []string{},
				Raw: map[string]any{
					"output": out,
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unknown tab %q", tab)
	}
}

func (b *sshBackend) fetchHostInfo() ([]Resource, error) {
	commands := []string{
		"hostname",
		"uname -a",
		"uptime",
		"df -h",
	}
	outResources := make([]Resource, 0, len(commands))
	for _, cmd := range commands {
		out, err := b.runSSH(cmd)
		if err != nil {
			out = err.Error()
		}
		outResources = append(outResources, Resource{
			ID:     cmd,
			Name:   cmd,
			Status: "",
			Details: map[string]string{
				"output": out,
			},
			Actions: []string{},
			Raw: map[string]any{
				"output": out,
			},
		})
	}
	return outResources, nil
}

func (b *sshBackend) Inspect(tab, id string) (Resource, error) {
	switch tab {
	case "host":
		out, err := b.runSSH(id)
		if err != nil {
			return Resource{}, err
		}
		return Resource{
			ID:      id,
			Name:    id,
			Status:  "",
			Details: map[string]string{"output": out},
			Actions: []string{},
			Raw: map[string]any{
				"output": out,
			},
		}, nil
	default:
		return Resource{}, fmt.Errorf("inspect not supported for %q", tab)
	}
}

func (b *sshBackend) RunAction(tab, id, action string, args []string) (string, error) {
	switch tab {
	case "host":
		return b.runSSH(id)
	default:
		return "", fmt.Errorf("actions not supported for %q", tab)
	}
}

func (b *sshBackend) Stream(tab, id string) (*exec.Cmd, io.ReadCloser, error) {
	if tab == "shell" && id != "" {
		cmd := exec.Command("ssh", "-p", fmt.Sprintf("%d", b.port), b.user+"@"+b.host, id)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, err
		}
		return cmd, stdout, nil
	}
	return nil, nil, fmt.Errorf("streaming not supported for %q", tab)
}

func (b *sshBackend) runSSH(command string) (string, error) {
	args := []string{"-p", fmt.Sprintf("%d", b.port), b.user + "@" + b.host, command}
	cmd := exec.Command("ssh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}
