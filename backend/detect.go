package backend

import (
	"fmt"
	"os/exec"
)

func Detect() (Backend, error) {
	if isK0s() || isKubernetes() {
		return NewKubernetes(""), nil
	}
	if isPodman() || isDocker() {
		return NewDocker("auto"), nil
	}
	if isHermes() {
		return NewHermes("", ""), nil
	}
	return nil, fmt.Errorf("no supported backend detected")
}

func MustDetect() Backend {
	b, err := Detect()
	if err != nil {
		return nil
	}
	return b
}

func isDocker() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func isPodman() bool {
	_, err := exec.LookPath("podman")
	return err == nil
}

func isKubernetes() bool {
	_, err := exec.LookPath("kubectl")
	return err == nil
}

func isK0s() bool {
	_, err := exec.LookPath("k0s")
	return err == nil
}

func isHermes() bool {
	return false
}
