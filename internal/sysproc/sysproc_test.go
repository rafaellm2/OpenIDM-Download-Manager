//go:build windows

package sysproc

import (
	"testing"
)

// TestHiddenCommand verifica se os atributos de supressão são aplicados corretamente
func TestHiddenCommand(t *testing.T) {
	cmd := HiddenCommand("notepad.exe")
	if cmd == nil {
		t.Fatal("esperava comando não nulo")
	}

	// Verifica se os atributos de supressão de janela foram configurados
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr não deve ser nil no Windows")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("esperava HideWindow = true")
	}
	if cmd.SysProcAttr.CreationFlags&CREATE_NO_WINDOW == 0 {
		t.Errorf("esperava flag CREATE_NO_WINDOW (0x%08x) em CreationFlags (0x%08x)", CREATE_NO_WINDOW, cmd.SysProcAttr.CreationFlags)
	}
}

// TestSetHiddenNilCmd garante que passar nil não causa panic
func TestSetHiddenNilCmd(t *testing.T) {
	SetHidden(nil)
}
