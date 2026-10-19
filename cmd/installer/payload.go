package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// payloadZip contém openidm.exe, openidm-native-host.exe e browser-extension/ quando o instalador
// é compilado com a tag "embedpayload" (ver scripts/build.ps1). Sem a tag, fica vazio e o instalador
// usa os arquivos que estiverem ao lado do executável (modo de desenvolvimento).
var payloadZip []byte

func hasEmbeddedPayload() bool {
	return len(payloadZip) > 0
}

// extractEmbeddedPayload descompacta o pacote embutido em uma pasta temporária e copia o próprio
// instalador para lá como OpenIDM-Setup.exe, para que ele seja instalado junto (desinstalar/reparar).
func extractEmbeddedPayload() (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(payloadZip), int64(len(payloadZip)))
	if err != nil {
		return "", fmt.Errorf("pacote embutido inválido: %w", err)
	}

	dir, err := os.MkdirTemp("", "openidm-setup-payload-*")
	if err != nil {
		return "", err
	}

	for _, f := range zr.File {
		name := filepath.FromSlash(strings.ReplaceAll(f.Name, `\`, "/"))
		dst := filepath.Join(dir, name)
		if !strings.HasPrefix(dst, filepath.Clean(dir)+string(os.PathSeparator)) {
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("caminho inválido no pacote: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				_ = os.RemoveAll(dir)
				return "", err
			}
			continue
		}
		if err := extractZipFile(f, dst); err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
	}

	if self, err := os.Executable(); err == nil {
		_ = copyFile(self, filepath.Join(dir, "OpenIDM-Setup.exe"))
	}
	return dir, nil
}

func extractZipFile(f *zip.File, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
