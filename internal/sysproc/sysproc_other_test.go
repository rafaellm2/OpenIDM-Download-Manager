//go:build !windows

package sysproc

import (
	"os/exec"
	"testing"
)

// TestHiddenCommandOther valida a criação de comando padrão em sistemas não-Windows
func TestHiddenCommandOther(t *testing.T) {
	// Cria comando com argumentos em ambiente não-Windows
	cmd := HiddenCommand("echo", "teste")
	if cmd == nil {
		t.Fatal("esperava comando não nulo")
	}

	// Fora do Windows, SysProcAttr padrão deve permanecer nil
	if cmd.SysProcAttr != nil {
		t.Errorf("SysProcAttr esperado nil fora do Windows, obtido: %v", cmd.SysProcAttr)
	}

	// Valida se o caminho do comando e argumentos foram preservados
	if cmd.Path != "echo" && len(cmd.Args) < 2 {
		t.Errorf("argumentos ou caminho não preservados: %v", cmd.Args)
	}
}

// TestHiddenSysProcAttrOther valida que atributos de processo oculto retornam nil
func TestHiddenSysProcAttrOther(t *testing.T) {
	// Verifica se a função no-op retorna nil
	attr := HiddenSysProcAttr()
	if attr != nil {
		t.Errorf("esperava nil de HiddenSysProcAttr fora do Windows, obtido: %v", attr)
	}
}

// TestSetHiddenOther valida a segurança de chamada no-op de SetHidden
func TestSetHiddenOther(t *testing.T) {
	// Chamada com nil não deve causar panic
	SetHidden(nil)

	// Chamada com comando válido não deve alterar nem causar erro
	cmd := exec.Command("echo", "teste")
	SetHidden(cmd)
	if cmd.SysProcAttr != nil {
		t.Errorf("SetHidden não deve configurar SysProcAttr fora do Windows: %v", cmd.SysProcAttr)
	}
}

// TestCreateNoWindowConstantOther valida o valor neutro da constante fora do Windows
func TestCreateNoWindowConstantOther(t *testing.T) {
	// Fora do Windows, a constante de flag deve ser zero
	if CREATE_NO_WINDOW != 0 {
		t.Errorf("esperava CREATE_NO_WINDOW == 0 fora do Windows, obtido %d", CREATE_NO_WINDOW)
	}
}
