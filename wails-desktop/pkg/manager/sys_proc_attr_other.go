//go:build !windows

package manager

import "os/exec"

func configureSysProcAttr(cmd *exec.Cmd) {
	// No-op en sistemas no-Windows
}
