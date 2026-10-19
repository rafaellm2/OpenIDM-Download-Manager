//go:build windows

package sysproc

import (
	"os/exec"
	"syscall"
)

// Constante da API do Windows para evitar alocação de janela de console (CMD)
const CREATE_NO_WINDOW = 0x08000000

// HiddenSysProcAttr retorna SysProcAttr configurado para suprimir janelas de console no Windows.
func HiddenSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: CREATE_NO_WINDOW,
	}
}

// SetHidden configura um *exec.Cmd para executar sem abrir janela de console.
func SetHidden(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = HiddenSysProcAttr()
	} else {
		cmd.SysProcAttr.HideWindow = true
		cmd.SysProcAttr.CreationFlags |= CREATE_NO_WINDOW
	}
}

// HiddenCommand cria um comando com os atributos de supressão de janela já aplicados.
func HiddenCommand(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	SetHidden(cmd)
	return cmd
}
