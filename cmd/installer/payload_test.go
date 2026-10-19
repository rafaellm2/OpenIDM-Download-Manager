package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractEmbeddedPayload(t *testing.T) {
	old := payloadZip
	defer func() { payloadZip = old }()

	// Compress-Archive do Windows PowerShell 5.1 grava separadores "\"; ambos devem funcionar.
	payloadZip = buildTestZip(t, map[string]string{
		"openidm.exe":                        "app",
		"openidm-native-host.exe":            "host",
		"browser-extension/manifest.json":    "{}",
		`browser-extension\icons\icon16.png`: "png",
	})

	dir, err := extractEmbeddedPayload()
	if err != nil {
		t.Fatalf("extractEmbeddedPayload: %v", err)
	}
	defer os.RemoveAll(dir)

	for name, want := range map[string]string{
		"openidm.exe":             "app",
		"openidm-native-host.exe": "host",
		filepath.Join("browser-extension", "manifest.json"):       "{}",
		filepath.Join("browser-extension", "icons", "icon16.png"): "png",
	} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s não extraído: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, esperado %q", name, got, want)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "OpenIDM-Setup.exe")); err != nil {
		t.Errorf("instalador não copiado para a pasta extraída: %v", err)
	}
}

func TestExtractEmbeddedPayloadRejectsPathTraversal(t *testing.T) {
	old := payloadZip
	defer func() { payloadZip = old }()

	payloadZip = buildTestZip(t, map[string]string{"../fora.txt": "x"})

	if dir, err := extractEmbeddedPayload(); err == nil {
		os.RemoveAll(dir)
		t.Fatal("esperado erro para caminho fora da pasta de destino")
	}
}
