package integration

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"openidm/internal/sysproc"
)

const NativeHostName = "com.rafatech.openidm"

// NativeMessagePayload representa a mensagem enviada pela extensão MV3 ao Native Host.
type NativeMessagePayload struct {
	Action      string `json:"action"` // "capture", "ping", "check", "open_app", "set_language"
	URL         string `json:"url"`
	FileName    string `json:"fileName,omitempty"`
	Referrer    string `json:"referrer,omitempty"`
	ModifierKey string `json:"modifierKey,omitempty"`
	StartNow    bool   `json:"startNow,omitempty"`
	Language    string `json:"language,omitempty"`
}

// NativeMessageResponse representa a resposta enviada de volta para a extensão do navegador.
type NativeMessageResponse struct {
	Success    bool   `json:"success"`
	Captured   bool   `json:"captured"`
	Reason     string `json:"reason,omitempty"`
	DownloadID string `json:"downloadId,omitempty"`
	Version    string `json:"version,omitempty"`
	Language   string `json:"language,omitempty"`
}

// ReadNativeMessage lê uma mensagem no formato oficial WebExtension Native Messaging (uint32 LE + JSON).
func ReadNativeMessage(r io.Reader) (*NativeMessagePayload, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	if length == 0 || length > 10*1024*1024 {
		return nil, fmt.Errorf("tamanho de mensagem nativa inválido: %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	var msg NativeMessagePayload
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// WriteNativeMessage escreve uma resposta no formato oficial WebExtension Native Messaging (uint32 LE + JSON).
func WriteNativeMessage(w io.Writer, resp NativeMessageResponse) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	length := uint32(len(raw))
	if err := binary.Write(w, binary.LittleEndian, length); err != nil {
		return err
	}
	_, err = w.Write(raw)
	return err
}

// computeChromePathID reproduz o algoritmo crx_file::id_util::GenerateIdForPath do Chromium no Windows (SHA-256 de UTF-16LE mapeado para 'a'..'p').
func computeChromePathID(pathStr string) string {
	u16 := utf16.Encode([]rune(pathStr))
	raw := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.LittleEndian.PutUint16(raw[i*2:], v)
	}
	sum := sha256.Sum256(raw)
	out := make([]byte, 32)
	for i := 0; i < 16; i++ {
		out[i*2] = 'a' + ((sum[i] >> 4) & 0x0F)
		out[i*2+1] = 'a' + (sum[i] & 0x0F)
	}
	return string(out)
}

// DeriveUnpackedExtensionIDs gera os IDs determinísticos que o Chrome/Edge atribuem à pasta browser-extension.
func DeriveUnpackedExtensionIDs(extDir string) []string {
	clean := filepath.Clean(extDir)
	candidates := []string{clean}
	if len(clean) >= 2 && clean[1] == ':' {
		upperDrive := strings.ToUpper(clean[:1]) + clean[1:]
		lowerDrive := strings.ToLower(clean[:1]) + clean[1:]
		candidates = append(candidates, upperDrive, lowerDrive, upperDrive+`\`, lowerDrive+`\`)
	}
	seen := make(map[string]bool)
	var ids []string
	for _, c := range candidates {
		id := computeChromePathID(c)
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// InstallNativeMessagingHost gera os manifestos JSON e registra o host no Windows Registry para Chrome, Edge e Firefox.
func InstallNativeMessagingHost(hostExePath string, chromeExtID string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(hostExePath), 0o755); err != nil {
		return "", err
	}

	manifestDir := filepath.Dir(hostExePath)
	chromeManifestPath := filepath.Join(manifestDir, "com.rafatech.openidm.chrome.json")
	firefoxManifestPath := filepath.Join(manifestDir, "com.rafatech.openidm.firefox.json")

	originsMap := make(map[string]bool)
	var origins []string
	addOrigin := func(extID string) {
		extID = strings.TrimSpace(extID)
		extID = strings.TrimPrefix(extID, "chrome-extension://")
		extID = strings.TrimSuffix(extID, "/")
		if len(extID) != 32 {
			return
		}
		origin := fmt.Sprintf("chrome-extension://%s/", extID)
		if !originsMap[origin] {
			originsMap[origin] = true
			origins = append(origins, origin)
		}
	}

	if chromeExtID != "" {
		addOrigin(chromeExtID)
	}
	for _, id := range DeriveUnpackedExtensionIDs(filepath.Join(manifestDir, "browser-extension")) {
		addOrigin(id)
	}
	if wd, wdErr := os.Getwd(); wdErr == nil && wd != "" {
		for _, id := range DeriveUnpackedExtensionIDs(filepath.Join(wd, "browser-extension")) {
			addOrigin(id)
		}
	}
	if defInstall := DefaultInstallDir(); defInstall != "" {
		for _, id := range DeriveUnpackedExtensionIDs(filepath.Join(defInstall, "browser-extension")) {
			addOrigin(id)
		}
	}
	addOrigin("knldjmfmopnpolahpmmgbagdohdnhkik")

	// Preserva origens já registradas anteriormente no manifesto local
	if existingBytes, err := os.ReadFile(chromeManifestPath); err == nil {
		var existing struct {
			AllowedOrigins []string `json:"allowed_origins"`
		}
		if json.Unmarshal(existingBytes, &existing) == nil {
			for _, o := range existing.AllowedOrigins {
				addOrigin(o)
			}
		}
	}

	chromeManifest := map[string]any{
		"name":            NativeHostName,
		"description":     "OpenIDM Native Messaging Host",
		"path":            hostExePath,
		"type":            "stdio",
		"allowed_origins": origins,
	}

	firefoxManifest := map[string]any{
		"name":        NativeHostName,
		"description": "OpenIDM Native Messaging Host",
		"path":        hostExePath,
		"type":        "stdio",
		"allowed_extensions": []string{
			"openidm@rafatech.local",
		},
	}

	cBytes, _ := json.MarshalIndent(chromeManifest, "", "  ")
	fBytes, _ := json.MarshalIndent(firefoxManifest, "", "  ")

	if err := os.WriteFile(chromeManifestPath, cBytes, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(firefoxManifestPath, fBytes, 0o644); err != nil {
		return "", err
	}

	// Remove manifestos legados caso existam no mesmo diretório
	_ = os.Remove(filepath.Join(manifestDir, "com.rafatech.turbodm.chrome.json"))
	_ = os.Remove(filepath.Join(manifestDir, "com.rafatech.turbodm.firefox.json"))

	if runtime.GOOS == "windows" && !isTempDirPath(hostExePath) {
		legacyHost := "com.rafatech.turbodm"
		legacyKeys := []string{
			`HKCU\Software\Google\Chrome\NativeMessagingHosts\` + legacyHost,
			`HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + legacyHost,
			`HKCU\Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + legacyHost,
			`HKCU\Software\Chromium\NativeMessagingHosts\` + legacyHost,
			`HKCU\Software\Mozilla\NativeMessagingHosts\` + legacyHost,
		}
		for _, lk := range legacyKeys {
			_ = sysproc.HiddenCommand("reg", "delete", lk, "/f").Run()
		}

		regKeys := map[string]string{
			`HKCU\Software\Google\Chrome\NativeMessagingHosts\` + NativeHostName:               chromeManifestPath,
			`HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + NativeHostName:              chromeManifestPath,
			`HKCU\Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + NativeHostName: chromeManifestPath,
			`HKCU\Software\Chromium\NativeMessagingHosts\` + NativeHostName:                    chromeManifestPath,
			`HKCU\Software\Mozilla\NativeMessagingHosts\` + NativeHostName:                     firefoxManifestPath,
		}
		for key, val := range regKeys {
			_ = sysproc.HiddenCommand("reg", "add", key, "/ve", "/t", "REG_SZ", "/d", val, "/f").Run()
		}
	}

	return chromeManifestPath, nil
}

