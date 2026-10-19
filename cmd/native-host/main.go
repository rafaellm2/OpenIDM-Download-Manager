package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"openidm/internal/integration"
)

// main implementa o host de Native Messaging do OpenIDM
// comunicando-se com as extensões Chrome, Edge e Firefox via stdin/stdout binário.
func main() {
	installFlag := flag.Bool("install", false, "Registra o Native Messaging Host no Windows Registry (Chrome/Edge/Firefox)")
	extIDFlag := flag.String("ext-id", "", "ID da extensão Chrome/Edge para o manifesto")
	flag.Parse()

	if *installFlag {
		exePath, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao obter executável: %v\n", err)
			os.Exit(1)
		}
		absPath, _ := filepath.Abs(exePath)
		manifestPath, err := integration.InstallNativeMessagingHost(absPath, *extIDFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Falha ao instalar Native Host: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Native Messaging Host instalado com sucesso: %s\n", manifestPath)
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	bridgeURL := "http://" + integration.DefaultBridgeAddr + "/api/capture"

	for {
		msg, err := integration.ReadNativeMessage(os.Stdin)
		if err != nil {
			if err == io.EOF {
				return
			}
			_ = integration.WriteNativeMessage(os.Stdout, integration.NativeMessageResponse{
				Success: false,
				Reason:  err.Error(),
			})
			return
		}

		if msg.Action == "ping" {
			_ = integration.WriteNativeMessage(os.Stdout, integration.NativeMessageResponse{
				Success: true,
				Version: integration.AppVersion,
			})
			continue
		}

		rawBody, _ := json.Marshal(msg)
		resp, err := client.Post(bridgeURL, "application/json", bytes.NewReader(rawBody))
		if err != nil {
			// Se o OpenIDM não estiver aberto, inicia o executável desktop automaticamente e tenta entregar o link.
			if launchErr := ensureOpenIDMRunning(); launchErr == nil {
				resp, err = client.Post(bridgeURL, "application/json", bytes.NewReader(rawBody))
			}
		}
		if err != nil {
			_ = integration.WriteNativeMessage(os.Stdout, integration.NativeMessageResponse{
				Success:  false,
				Captured: false,
				Reason:   "OpenIDM não está em execução (" + err.Error() + ")",
			})
			continue
		}

		var out integration.NativeMessageResponse
		_ = json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()

		_ = integration.WriteNativeMessage(os.Stdout, out)
	}
}

// ensureOpenIDMRunning inicia openidm.exe na mesma pasta caso o usuário tenha clicado em um link com o app fechado.
func ensureOpenIDMRunning() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	appExe := filepath.Join(filepath.Dir(exePath), "openidm.exe")
	if _, statErr := os.Stat(appExe); statErr != nil {
		return statErr
	}
	// Inicia oculto no relógio (--tray) para que o /api/capture exiba apenas o Card Solto independente
	cmd := exec.Command(appExe, "--tray")
	if err := cmd.Start(); err != nil {
		return err
	}
	healthURL := "http://" + integration.DefaultBridgeAddr + "/api/health"
	probeClient := &http.Client{Timeout: 500 * time.Millisecond}
	for i := 0; i < 12; i++ {
		time.Sleep(300 * time.Millisecond)
		if r, hErr := probeClient.Get(healthURL); hErr == nil {
			_ = r.Body.Close()
			return nil
		}
	}
	return nil
}

