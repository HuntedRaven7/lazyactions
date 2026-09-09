package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type dockerBackend struct {
	engine string
}

func NewDocker(engine string) Backend {
	if engine == "" {
		engine = "auto"
	}
	return &dockerBackend{engine: engine}
}

func (b *dockerBackend) Name() string {
	if b.engine == "podman" {
		return "Podman"
	}
	if b.engine == "docker" {
		return "Docker"
	}
	return "Docker/Podman"
}

func (b *dockerBackend) Tabs() []string {
	return []string{"containers", "images", "volumes", "networks"}
}

func (b *dockerBackend) Columns(tab string) []string {
	switch tab {
	case "containers":
		return []string{"name", "image", "status", "ports"}
	case "images":
		return []string{"repository", "tag", "size", "created"}
	case "volumes":
		return []string{"name", "driver", "mountpoint"}
	case "networks":
		return []string{"name", "driver", "subnet"}
	default:
		return nil
	}
}

func (b *dockerBackend) Fetch(tab, filter string) ([]Resource, error) {
	switch tab {
	case "containers":
		return b.fetchContainers(filter)
	case "images":
		return b.fetchImages(filter)
	case "volumes":
		return b.fetchVolumes(filter)
	case "networks":
		return b.fetchNetworks(filter)
	default:
		return nil, fmt.Errorf("unknown tab %q", tab)
	}
}

func (b *dockerBackend) fetchContainers(filter string) ([]Resource, error) {
	args := []string{"ps", "-a", "--format", "{{json .}}"}
	if filter != "" {
		args = append(args, "--filter", filter)
	}
	out, err := runDocker(b.engine, args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	outResources := make([]Resource, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var c struct {
			ID      string `json:"ID"`
			Names   string `json:"Names"`
			Image   string `json:"Image"`
			Status  string `json:"Status"`
			Ports   string `json:"Ports"`
			State   string `json:"State"`
		}
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		status := c.Status
		if status == "" {
			status = c.State
		}
	outResources = append(outResources, Resource{
		ID:     c.ID,
		Name:   c.Names,
		Status: status,
		Details: map[string]string{
			"name":    c.Names,
			"image":   c.Image,
			"status":  status,
			"ports":   c.Ports,
			"state":   c.State,
		},
		Actions: []string{"start", "stop", "restart", "logs", "rm", "inspect"},
		Raw: map[string]any{
			"container": c,
		},
	})
	}
	return outResources, nil
}

func (b *dockerBackend) fetchImages(filter string) ([]Resource, error) {
	args := []string{"images", "--format", "{{json .}}"}
	if filter != "" {
		args = append(args, filter)
	}
	out, err := runDocker(b.engine, args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	outResources := make([]Resource, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var img struct {
			ID         string `json:"ID"`
			Repository string `json:"Repository"`
			Tag        string `json:"Tag"`
			Size       string `json:"Size"`
			CreatedAt  string `json:"CreatedAt"`
		}
		if err := json.Unmarshal([]byte(line), &img); err != nil {
			continue
		}
	outResources = append(outResources, Resource{
		ID:     img.ID,
		Name:   img.Repository + ":" + img.Tag,
		Status: img.Size,
		Details: map[string]string{
			"repository": img.Repository,
			"tag":        img.Tag,
			"size":       img.Size,
			"created":    img.CreatedAt,
		},
		Actions: []string{"rm", "inspect", "history"},
		Raw: map[string]any{
			"image": img,
		},
	})
	}
	return outResources, nil
}

func (b *dockerBackend) fetchVolumes(filter string) ([]Resource, error) {
	args := []string{"volume", "ls", "--format", "{{json .}}"}
	if filter != "" {
		args = append(args, "--filter", filter)
	}
	out, err := runDocker(b.engine, args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	outResources := make([]Resource, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var vol struct {
			Name        string `json:"Name"`
			Driver      string `json:"Driver"`
			Mountpoint  string `json:"Mountpoint"`
		}
		if err := json.Unmarshal([]byte(line), &vol); err != nil {
			continue
		}
	outResources = append(outResources, Resource{
		ID:     vol.Name,
		Name:   vol.Name,
		Status: vol.Driver,
		Details: map[string]string{
			"name":      vol.Name,
			"driver":    vol.Driver,
			"mountpoint": vol.Mountpoint,
		},
		Actions: []string{"rm", "inspect"},
		Raw: map[string]any{
			"volume": vol,
		},
	})
	}
	return outResources, nil
}

func (b *dockerBackend) fetchNetworks(filter string) ([]Resource, error) {
	args := []string{"network", "ls", "--format", "{{json .}}"}
	if filter != "" {
		args = append(args, "--filter", filter)
	}
	out, err := runDocker(b.engine, args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	outResources := make([]Resource, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var net struct {
			ID     string `json:"ID"`
			Name   string `json:"Name"`
			Driver string `json:"Driver"`
			Scope  string `json:"Scope"`
		}
		if err := json.Unmarshal([]byte(line), &net); err != nil {
			continue
		}
	outResources = append(outResources, Resource{
		ID:     net.ID,
		Name:   net.Name,
		Status: net.Driver,
		Details: map[string]string{
			"name":   net.Name,
			"driver": net.Driver,
			"subnet": net.Scope,
		},
		Actions: []string{"rm", "inspect"},
		Raw: map[string]any{
			"network": net,
		},
	})
	}
	return outResources, nil
}

func (b *dockerBackend) Inspect(tab, id string) (Resource, error) {
	switch tab {
	case "containers":
		out, err := runDocker(b.engine, "inspect", "--type", "container", id)
		if err != nil {
			return Resource{}, err
		}
		var details []map[string]any
		if err := json.Unmarshal([]byte(out), &details); err != nil {
			return Resource{}, err
		}
		raw := map[string]any{"inspect": details}
		if len(details) > 0 {
			state := ""
			if s, ok := details[0]["State"].(map[string]any); ok {
				state = fmt.Sprintf("%v", s["Status"])
			}
			name := fmt.Sprintf("%v", details[0]["Name"])
			return Resource{
				ID:      id,
				Name:    name,
				Status:  state,
				Details: map[string]string{},
				Actions: []string{"start", "stop", "restart", "logs", "rm"},
				Raw:     raw,
			}, nil
		}
		return Resource{}, fmt.Errorf("no inspect data")
	case "images":
		out, err := runDocker(b.engine, "inspect", id)
		if err != nil {
			return Resource{}, err
		}
		var details []map[string]any
		if err := json.Unmarshal([]byte(out), &details); err != nil {
			return Resource{}, err
		}
		raw := map[string]any{"inspect": details}
		if len(details) > 0 {
			return Resource{
				ID:      id,
				Name:    fmt.Sprintf("%v", details[0]["RepoTags"]),
				Status:  fmt.Sprintf("%v", details[0]["Size"]),
				Details: map[string]string{},
				Actions: []string{"rm", "history"},
				Raw:     raw,
			}, nil
		}
		return Resource{}, fmt.Errorf("no inspect data")
	default:
		return Resource{}, fmt.Errorf("inspect not supported for %q", tab)
	}
}

func (b *dockerBackend) RunAction(tab, id, action string, args []string) (string, error) {
	switch tab {
	case "containers":
		switch action {
		case "start":
			_, err := runDocker(b.engine, "start", id)
			return "", err
		case "stop":
			_, err := runDocker(b.engine, "stop", id)
			return "", err
		case "restart":
			_, err := runDocker(b.engine, "restart", id)
			return "", err
		case "rm":
			_, err := runDocker(b.engine, "rm", id)
			return "", err
		case "logs":
			return runDocker(b.engine, "logs", id)
		case "inspect":
			return runDocker(b.engine, "inspect", id)
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	case "images":
		switch action {
		case "rm":
			_, err := runDocker(b.engine, "rmi", id)
			return "", err
		case "history":
			return runDocker(b.engine, "history", id)
		case "inspect":
			return runDocker(b.engine, "inspect", id)
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	case "volumes":
		switch action {
		case "rm":
			_, err := runDocker(b.engine, "volume", "rm", id)
			return "", err
		case "inspect":
			return runDocker(b.engine, "volume", "inspect", id)
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	case "networks":
		switch action {
		case "rm":
			_, err := runDocker(b.engine, "network", "rm", id)
			return "", err
		case "inspect":
			return runDocker(b.engine, "network", "inspect", id)
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	default:
		return "", fmt.Errorf("actions not supported for %q", tab)
	}
}

func (b *dockerBackend) Stream(tab, id string) (*exec.Cmd, io.ReadCloser, error) {
	if tab == "containers" {
		cmd := exec.Command(dockerBinary(b.engine), "logs", "-f", id)
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

func runDocker(engine string, args ...string) (string, error) {
	cmd := exec.Command(dockerBinary(engine), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

func dockerBinary(engine string) string {
	switch engine {
	case "podman":
		return "podman"
	case "docker":
		return "docker"
	default:
		if exists("podman") {
			return "podman"
		}
		return "docker"
	}
}

func exists(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
