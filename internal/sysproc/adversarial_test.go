//go:build windows

package sysproc_test

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"openidm/internal/sysproc"
)

func TestAdversarialSysprocExecutionAndFlags(t *testing.T) {
	// 1. Testa HiddenCommand e execução real suprimida no Windows
	cmd := sysproc.HiddenCommand("cmd.exe", "/c", "echo sysproc_execution_ok")
	if cmd == nil {
		t.Fatal("HiddenCommand retornou nil")
	}
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr não deve ser nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Errorf("HideWindow esperado true, obtido %v", cmd.SysProcAttr.HideWindow)
	}
	const expectedFlag = 0x08000000
	if cmd.SysProcAttr.CreationFlags&expectedFlag == 0 {
		t.Errorf("CreationFlags não contém 0x08000000: 0x%08x", cmd.SysProcAttr.CreationFlags)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("falha ao executar comando oculto: %v", err)
	}
	if !strings.Contains(string(out), "sysproc_execution_ok") {
		t.Errorf("saída inesperada: %s", string(out))
	}

	// 2. Testa SetHidden com SysProcAttr pré-existente
	preExistingFlag := uint32(0x00000200) // CREATE_NEW_PROCESS_GROUP
	cmd2 := exec.Command("cmd.exe", "/c", "echo sysproc_preserves_flags")
	cmd2.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: preExistingFlag,
	}
	sysproc.SetHidden(cmd2)
	if !cmd2.SysProcAttr.HideWindow {
		t.Errorf("HideWindow esperado true após SetHidden")
	}
	if cmd2.SysProcAttr.CreationFlags&(preExistingFlag|expectedFlag) != (preExistingFlag | expectedFlag) {
		t.Errorf("CreationFlags esperava conter 0x%08x, obtido: 0x%08x", preExistingFlag|expectedFlag, cmd2.SysProcAttr.CreationFlags)
	}

	out2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("falha na execução com flags preservadas: %v", err)
	}
	if !strings.Contains(string(out2), "sysproc_preserves_flags") {
		t.Errorf("saída inesperada: %s", string(out2))
	}

	// 3. Testa Idempotência (chamada repetida de SetHidden)
	sysproc.SetHidden(cmd2)
	if cmd2.SysProcAttr.CreationFlags != (preExistingFlag | expectedFlag) {
		t.Errorf("Idempotência falhou: 0x%08x != 0x%08x", cmd2.SysProcAttr.CreationFlags, preExistingFlag|expectedFlag)
	}

	// 4. Testa chamada com nil
	sysproc.SetHidden(nil)
}
