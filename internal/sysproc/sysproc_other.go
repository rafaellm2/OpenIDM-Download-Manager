//go:build !windows

package sysproc

import (
	"os/exec"
	"syscall"
)

// Constante fictícia para compatibilidade em sistemas não-Windows
const CREATE_NO_WINDOW = 0

// HiddenSysProcAttr retorna nil em sistemas não-Windows
func HiddenSysProcAttr() *syscall.SysProcAttr {
	return nil
}

// SetHidden é uma operação no-op em sistemas não-Windows
func SetHidden(cmd *exec.Cmd) {
	// Nenhuma ação necessária fora do Windows
}

// HiddenCommand cria um *exec.Cmd padrão fora do Windows
func HiddenCommand(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}
