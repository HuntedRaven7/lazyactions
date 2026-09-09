package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type k8sBackend struct {
	kubeconfig string
}

func NewKubernetes(kubeconfig string) Backend {
	if kubeconfig == "" {
		kubeconfig = "~/.kube/config"
	}
	return &k8sBackend{kubeconfig: kubeconfig}
}

func (b *k8sBackend) Name() string {
	return "Kubernetes/k0s"
}

func (b *k8sBackend) Tabs() []string {
	return []string{"pods", "deployments", "nodes", "namespaces"}
}

func (b *k8sBackend) Columns(tab string) []string {
	switch tab {
	case "pods":
		return []string{"name", "namespace", "status", "restarts"}
	case "deployments":
		return []string{"name", "namespace", "ready", "up-to-date"}
	case "nodes":
		return []string{"name", "status", "roles", "version"}
	case "namespaces":
		return []string{"name", "status"}
	default:
		return nil
	}
}

func (b *k8sBackend) Fetch(tab, filter string) ([]Resource, error) {
	switch tab {
	case "pods":
		return b.fetchPods(filter)
	case "deployments":
		return b.fetchDeployments(filter)
	case "nodes":
		return b.fetchNodes(filter)
	case "namespaces":
		return b.fetchNamespaces(filter)
	default:
		return nil, fmt.Errorf("unknown tab %q", tab)
	}
}

func (b *k8sBackend) fetchPods(filter string) ([]Resource, error) {
	args := []string{"get", "pods", "-o", "json"}
	if b.kubeconfig != "" {
		args = append([]string{"--kubeconfig", b.kubeconfig}, args...)
	}
	out, err := runKubectl(args...)
	if err != nil {
		return nil, err
	}
	var pods struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status struct {
				Phase string `json:"phase"`
				ContainerStatuses []struct {
					RestartCount int `json:"restartCount"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &pods); err != nil {
		return nil, err
	}
	outResources := make([]Resource, 0, len(pods.Items))
	for _, item := range pods.Items {
		restarts := 0
		if len(item.Status.ContainerStatuses) > 0 {
			restarts = item.Status.ContainerStatuses[0].RestartCount
		}
		outResources = append(outResources, Resource{
			ID:     item.Metadata.Namespace + "/" + item.Metadata.Name,
			Name:   item.Metadata.Name,
			Status: item.Status.Phase,
			Details: map[string]string{
				"namespace": item.Metadata.Namespace,
				"restarts":  fmt.Sprintf("%d", restarts),
			},
			Actions: []string{"logs", "describe", "exec", "delete"},
			Raw: map[string]any{
				"pod": item,
			},
		})
	}
	return outResources, nil
}

func (b *k8sBackend) fetchDeployments(filter string) ([]Resource, error) {
	args := []string{"get", "deployments", "-o", "json"}
	if b.kubeconfig != "" {
		args = append([]string{"--kubeconfig", b.kubeconfig}, args...)
	}
	out, err := runKubectl(args...)
	if err != nil {
		return nil, err
	}
	var deployments struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status struct {
				ReadyReplicas   int `json:"readyReplicas"`
				UpdatedReplicas int `json:"updatedReplicas"`
				Replicas        int `json:"replicas"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &deployments); err != nil {
		return nil, err
	}
	outResources := make([]Resource, 0, len(deployments.Items))
	for _, item := range deployments.Items {
		outResources = append(outResources, Resource{
			ID:     item.Metadata.Namespace + "/" + item.Metadata.Name,
			Name:   item.Metadata.Name,
			Status: fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, item.Status.Replicas),
			Details: map[string]string{
				"namespace":   item.Metadata.Namespace,
				"up-to-date": fmt.Sprintf("%d", item.Status.UpdatedReplicas),
			},
			Actions: []string{"describe", "scale", "rollout", "delete"},
			Raw: map[string]any{
				"deployment": item,
			},
		})
	}
	return outResources, nil
}

func (b *k8sBackend) fetchNodes(filter string) ([]Resource, error) {
	args := []string{"get", "nodes", "-o", "json"}
	if b.kubeconfig != "" {
		args = append([]string{"--kubeconfig", b.kubeconfig}, args...)
	}
	out, err := runKubectl(args...)
	if err != nil {
		return nil, err
	}
	var nodes struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &nodes); err != nil {
		return nil, err
	}
	outResources := make([]Resource, 0, len(nodes.Items))
	for _, item := range nodes.Items {
		status := "Unknown"
		for _, c := range item.Status.Conditions {
			if c.Type == "Ready" {
				status = c.Status
				break
			}
		}
		outResources = append(outResources, Resource{
			ID:     item.Metadata.Name,
			Name:   item.Metadata.Name,
			Status: status,
			Details: map[string]string{},
			Actions: []string{"describe", "cordon", "uncordon", "drain"},
			Raw: map[string]any{
				"node": item,
			},
		})
	}
	return outResources, nil
}

func (b *k8sBackend) fetchNamespaces(filter string) ([]Resource, error) {
	args := []string{"get", "namespaces", "-o", "json"}
	if b.kubeconfig != "" {
		args = append([]string{"--kubeconfig", b.kubeconfig}, args...)
	}
	out, err := runKubectl(args...)
	if err != nil {
		return nil, err
	}
	var namespaces struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &namespaces); err != nil {
		return nil, err
	}
	outResources := make([]Resource, 0, len(namespaces.Items))
	for _, item := range namespaces.Items {
		outResources = append(outResources, Resource{
			ID:     item.Metadata.Name,
			Name:   item.Metadata.Name,
			Status: item.Status.Phase,
			Details: map[string]string{},
			Actions: []string{"describe", "delete"},
			Raw: map[string]any{
				"namespace": item,
			},
		})
	}
	return outResources, nil
}

func (b *k8sBackend) Inspect(tab, id string) (Resource, error) {
	switch tab {
	case "pods", "deployments", "nodes", "namespaces":
		out, err := runKubectl("describe", tab, id)
		if err != nil {
			return Resource{}, err
		}
		return Resource{
			ID:      id,
			Name:    id,
			Status:  "",
			Details: map[string]string{"describe": out},
			Actions: []string{},
			Raw: map[string]any{
				"describe": out,
			},
		}, nil
	default:
		return Resource{}, fmt.Errorf("inspect not supported for %q", tab)
	}
}

func (b *k8sBackend) RunAction(tab, id, action string, args []string) (string, error) {
	parts := strings.SplitN(id, "/", 2)
	name := id
	ns := ""
	if len(parts) == 2 {
		ns = parts[0]
		name = parts[1]
	}
	baseArgs := []string{}
	if b.kubeconfig != "" {
		baseArgs = append(baseArgs, "--kubeconfig", b.kubeconfig)
	}
	if ns != "" {
		baseArgs = append(baseArgs, "-n", ns)
	}
	switch tab {
	case "pods":
		switch action {
		case "logs":
			a := append([]string{"logs", name}, args...)
			return runKubectl(append(append([]string{}, baseArgs...), a...)...)
		case "exec":
			a := append([]string{"exec", name}, args...)
			_, err := runKubectl(append(append([]string{}, baseArgs...), a...)...)
			return "", err
		case "delete":
			_, err := runKubectl(append(append([]string{}, baseArgs...), "delete", "pod", name)...)
			return "", err
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	case "deployments":
		switch action {
		case "scale":
			a := append([]string{"scale", "deployment", name}, args...)
			_, err := runKubectl(append(append([]string{}, baseArgs...), a...)...)
			return "", err
		case "rollout":
			a := append([]string{"rollout", "restart", "deployment", name}, args...)
			_, err := runKubectl(append(append([]string{}, baseArgs...), a...)...)
			return "", err
		case "delete":
			_, err := runKubectl(append(append([]string{}, baseArgs...), "delete", "deployment", name)...)
			return "", err
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	case "nodes":
		switch action {
		case "cordon":
			_, err := runKubectl(append(append([]string{}, baseArgs...), "cordon", name)...)
			return "", err
		case "uncordon":
			_, err := runKubectl(append(append([]string{}, baseArgs...), "uncordon", name)...)
			return "", err
		case "drain":
			a := append([]string{"drain", name}, args...)
			_, err := runKubectl(append(append([]string{}, baseArgs...), a...)...)
			return "", err
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	case "namespaces":
		switch action {
		case "delete":
			_, err := runKubectl(append(append([]string{}, baseArgs...), "delete", "namespace", name)...)
			return "", err
		default:
			return "", fmt.Errorf("unknown action %q", action)
		}
	default:
		return "", fmt.Errorf("actions not supported for %q", tab)
	}
}

func (b *k8sBackend) Stream(tab, id string) (*exec.Cmd, io.ReadCloser, error) {
	if tab == "pods" {
		parts := strings.SplitN(id, "/", 2)
		ns := ""
		name := id
		if len(parts) == 2 {
			ns = parts[0]
			name = parts[1]
		}
		args := []string{"logs", "-f", name}
		if b.kubeconfig != "" {
			args = append([]string{"--kubeconfig", b.kubeconfig}, args...)
		}
		if ns != "" {
			args = append([]string{"-n", ns}, args...)
		}
		cmd := exec.Command("kubectl", args...)
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

func runKubectl(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}
