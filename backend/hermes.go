package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type hermesBackend struct {
	baseURL   string
	apiKeyEnv string
}

func NewHermes(baseURL, apiKeyEnv string) Backend {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8642/v1"
	}
	if apiKeyEnv == "" {
		apiKeyEnv = "HERMES_API_KEY"
	}
	return &hermesBackend{baseURL: baseURL, apiKeyEnv: apiKeyEnv}
}

func (b *hermesBackend) Name() string {
	return "Hermes"
}

func (b *hermesBackend) Tabs() []string {
	return []string{"chats", "status"}
}

func (b *hermesBackend) Columns(tab string) []string {
	switch tab {
	case "chats":
		return []string{"id", "title", "updated"}
	case "status":
		return []string{"key", "value"}
	default:
		return nil
	}
}

func (b *hermesBackend) Fetch(tab, filter string) ([]Resource, error) {
	switch tab {
	case "chats":
		return b.fetchChats(filter)
	case "status":
		return b.fetchStatus(filter)
	default:
		return nil, fmt.Errorf("unknown tab %q", tab)
	}
}

func (b *hermesBackend) fetchChats(filter string) ([]Resource, error) {
	apiKey := os.Getenv(b.apiKeyEnv)
	args := []string{"-s", "-H", "Authorization: Bearer " + apiKey, b.baseURL + "/chats"}
	out, err := runHTTP(args...)
	if err != nil {
		return nil, err
	}
	var chats []struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Updated string `json:"updated_at"`
	}
	if err := json.Unmarshal([]byte(out), &chats); err != nil {
		return nil, err
	}
	outResources := make([]Resource, 0, len(chats))
	for _, c := range chats {
		outResources = append(outResources, Resource{
			ID:     c.ID,
			Name:   c.Title,
			Status: c.Updated,
			Details: map[string]string{
				"updated": c.Updated,
			},
			Actions: []string{"open", "delete"},
			Raw: map[string]any{
				"chat": c,
			},
		})
	}
	return outResources, nil
}

func (b *hermesBackend) fetchStatus(filter string) ([]Resource, error) {
	apiKey := os.Getenv(b.apiKeyEnv)
	args := []string{"-s", "-H", "Authorization: Bearer " + apiKey, b.baseURL + "/status"}
	out, err := runHTTP(args...)
	if err != nil {
		return nil, err
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		return nil, err
	}
	outResources := make([]Resource, 0)
	for k, v := range status {
		outResources = append(outResources, Resource{
			ID:     k,
			Name:   k,
			Status: fmt.Sprintf("%v", v),
			Details: map[string]string{
				"value": fmt.Sprintf("%v", v),
			},
			Actions: []string{},
			Raw: map[string]any{
				"value": v,
			},
		})
	}
	return outResources, nil
}

func (b *hermesBackend) Inspect(tab, id string) (Resource, error) {
	switch tab {
	case "chats":
		apiKey := os.Getenv(b.apiKeyEnv)
		args := []string{"-s", "-H", "Authorization: Bearer " + apiKey, b.baseURL + "/chats/" + id}
		out, err := runHTTP(args...)
		if err != nil {
			return Resource{}, err
		}
		var chat struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if err := json.Unmarshal([]byte(out), &chat); err != nil {
			return Resource{}, err
		}
		return Resource{
			ID:      id,
			Name:    chat.Title,
			Status:  "",
			Details: map[string]string{"body": chat.Body},
			Actions: []string{"open", "delete"},
			Raw: map[string]any{
				"chat": chat,
			},
		}, nil
	default:
		return Resource{}, fmt.Errorf("inspect not supported for %q", tab)
	}
}

func (b *hermesBackend) RunAction(tab, id, action string, args []string) (string, error) {
	apiKey := os.Getenv(b.apiKeyEnv)
	switch tab {
	case "chats":
		switch action {
		case "delete":
			cmdArgs := []string{"-X", "DELETE", "-H", "Authorization: Bearer " + apiKey, b.baseURL + "/chats/" + id}
			_, err := runHTTP(cmdArgs...)
			return "", err
		case "open":
			cmdArgs := []string{"-s", "-H", "Authorization: Bearer " + apiKey, b.baseURL + "/chats/" + id}
			return runHTTP(cmdArgs...)
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	default:
		return "", fmt.Errorf("actions not supported for %q", tab)
	}
}

func (b *hermesBackend) Stream(tab, id string) (*exec.Cmd, io.ReadCloser, error) {
	return nil, nil, fmt.Errorf("streaming not supported for %q", tab)
}

func runHTTP(args ...string) (string, error) {
	cmd := exec.Command("curl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}
