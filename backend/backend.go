package backend

import (
	"io"
	"os/exec"
)

type Resource struct {
	ID         string
	Name       string
	Status     string
	Details    map[string]string
	Actions    []string
	Raw        map[string]any
}

type Backend interface {
	Name() string
	Tabs() []string
	Columns(tab string) []string
	Fetch(tab, filter string) ([]Resource, error)
	Inspect(tab, id string) (Resource, error)
	RunAction(tab, id, action string, args []string) (string, error)
	Stream(tab, id string) (*exec.Cmd, io.ReadCloser, error)
}
