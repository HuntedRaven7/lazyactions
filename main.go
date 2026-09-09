package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/robin/lazyactions/backend"
	"github.com/robin/lazyactions/tui"
)

func main() {
	backendName := flag.String("backend", "", "backend to use: github, docker, k8s, ssh, hermes")
	flag.Parse()

	var b backend.Backend
	switch *backendName {
	case "docker", "podman":
		b = backend.NewDocker("auto")
	case "k8s", "k0s", "kubernetes":
		b = backend.NewKubernetes("")
	case "ssh":
		b = backend.NewSSH("localhost", "root", 22)
	case "hermes":
		b = backend.NewHermes("", "")
	case "github":
		b = backend.NewGitHub("")
	case "":
		b = nil
	default:
		fmt.Fprintf(os.Stderr, "unknown backend %q\n", *backendName)
		os.Exit(1)
	}

	if err := tui.RunWithBackend(b); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
