package integration

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openidm/internal/engine"
	"openidm/internal/models"
	"openidm/internal/scheduler"
	"openidm/internal/store"
)

// TestShouldCaptureURL verifica filtros de extensão, tecla modificadora ALT e wildcards de site.
func TestShouldCaptureURL(t *testing.T) {
	settings := models.AppSettings{
		CaptureExtensions: []string{"zip", "exe", "mp4"},
		IgnoreModifierKey: "ALT",
	}
	rules := []models.SiteRule{
		{
			HostPattern:   "*.ignorado.com.br",
			IgnoreCapture: true,
		},
	}

	// 1. Extensão monitorada (.zip) em domínio normal -> Deve capturar.
	if ok, _ := ShouldCaptureURL("https://cdn.exemplo.com/pacote.zip", "", settings, rules); !ok {
		t.Fatalf("deveria capturar arquivo .zip")
	}

	// 2. Tecla modificadora ALT pressionada -> Deve ignorar captura.
	if ok, _ := ShouldCaptureURL("https://cdn.exemplo.com/pacote.zip", "ALT", settings, rules); ok {
		t.Fatalf("deveria ignorar captura quando a tecla ALT está pressionada")
	}

	// 3. Site na lista de exceções com wildcard (*.ignorado.com.br) -> Deve ignorar captura.
	if ok, _ := ShouldCaptureURL("https://sub.ignorado.com.br/instalador.exe", "", settings, rules); ok {
		t.Fatalf("deveria ignorar site que casa com wildcard *.ignorado.com.br")
	}

	// 4. Extensão comum de página (.html) -> Não deve capturar.
	if ok, _ := ShouldCaptureURL("https://exemplo.com/index.html", "", settings, rules); ok {
		t.Fatalf("não deveria capturar página .html")
	}

	// 5. URL sem extensão mas com fileName vindo de Content-Disposition/navegador -> Deve capturar.
	if ok, _ := ShouldCaptureDownload("https://api.exemplo.com/v1/export?token=abc", "C:\\Users\\teste\\Downloads\\backup.zip", "", "", settings, rules); !ok {
		t.Fatalf("deveria capturar quando fileName possui extensão monitorada mesmo em URL sem extensão")
	}

	// 6. Download hospedado em CDN externo mas iniciado a partir de página Referer na lista de exceções -> Deve ignorar.
	if ok, _ := ShouldCaptureDownload("https://cdn-externo.net/instalador.exe", "instalador.exe", "https://app.ignorado.com.br/downloads", "", settings, rules); ok {
		t.Fatalf("deveria ignorar captura quando o Referer casa com regra de exceção do site")
	}
}

// TestNativeMessagingFraming valida a codificação e decodificação binária (uint32 LE + JSON) do Native Messaging.
func TestNativeMessagingFraming(t *testing.T) {
	var buf bytes.Buffer

	resp := NativeMessageResponse{
		Success:    true,
		Captured:   true,
		DownloadID: "dl_123",
		Version:    "1.0.0",
	}
	if err := WriteNativeMessage(&buf, resp); err != nil {
		t.Fatalf("erro em WriteNativeMessage: %v", err)
	}

	// Como ReadNativeMessage lê o cabeçalho uint32 LE + JSON, testamos um payload completo.
	var inBuf bytes.Buffer
	_ = WriteNativeMessage(&inBuf, NativeMessageResponse{Success: true})
	msg, err := ReadNativeMessage(&inBuf)
	if err != nil || msg == nil {
		t.Fatalf("erro em ReadNativeMessage: %v", err)
	}
}

// TestBrowserCapturePromptsOrganization verifica que clicar em um link dispara o callback da tela de organização do app sem iniciar download silencioso.
func TestBrowserCapturePromptsOrganization(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.sqlite")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("erro ao criar store: %v", err)
	}
	defer st.Close()

	mgr, err := engine.NewManager(st)
	if err != nil {
		t.Fatalf("erro ao criar manager: %v", err)
	}
	sched := scheduler.NewScheduler(st, mgr)
	bridge := NewBridgeServer(st, mgr, sched, nil)

	var promptedMsg NativeMessagePayload
	var openCalled bool
	bridge.SetCapturePromptHandler(func(msg NativeMessagePayload) {
		promptedMsg = msg
	})
	bridge.SetOpenAppHandler(func() {
		openCalled = true
	})

	// 1. Dispara captura de link clicado
	body, _ := json.Marshal(NativeMessagePayload{
		Action:   "capture",
		URL:      "https://cdn.exemplo.com/instalador_v2.exe",
		FileName: "instalador_v2.exe",
		Referrer: "https://exemplo.com/downloads",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	bridge.handleBrowserCapture(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status esperado 200, obteve %d", rec.Code)
	}
	if promptedMsg.URL != "https://cdn.exemplo.com/instalador_v2.exe" {
		t.Fatalf("esperava que o link fosse roteado para a tela de organização do app, obteve %+v", promptedMsg)
	}
	// Verifica que nenhum download foi iniciado silenciosamente sem confirmação do usuário
	if len(mgr.ListDownloads()) != 0 {
		t.Fatalf("não deveria iniciar download silencioso antes da confirmação na tela de organização")
	}

	// 2. Testa comando open_app
	openBody, _ := json.Marshal(NativeMessagePayload{Action: "open_app"})
	openReq := httptest.NewRequest(http.MethodPost, "/api/capture", bytes.NewReader(openBody))
	openRec := httptest.NewRecorder()
	bridge.handleBrowserCapture(openRec, openReq)
	if !openCalled {
		t.Fatalf("esperava que SetOpenAppHandler fosse acionado em open_app")
	}

	// 3. Verifica classificação automática na organização por categoria
	cats, _ := st.ListCategories()
	fName, catID, saveDir := SuggestCategoryAndDir("https://cdn.exemplo.com/instalador_v2.exe", "", cats, "C:\\Downloads")
	if fName != "instalador_v2.exe" || catID != "programs" || saveDir == "" {
		t.Fatalf("classificação inesperada para .exe: fileName=%s catID=%s saveDir=%s", fName, catID, saveDir)
	}

	// 4. Testa criação do ícone 32x32 da bandeja do relógio (System Tray)
	hIcon := createOpenIDMIcon()
	if hIcon != 0 {
		_, _, _ = procDestroyIcon.Call(hIcon)
	}
}

// TestShouldCaptureBrowserDownloadAndCDNURLs verifica a interceptação imediata de downloads reais do Chrome (onCreated sem fileName, HuggingFace .safetensors, GitHub codeload e response-content-disposition).
func TestShouldCaptureBrowserDownloadAndCDNURLs(t *testing.T) {
	settings := models.AppSettings{
		CaptureExtensions: []string{"zip", "exe", "mp4"},
		IgnoreModifierKey: "ALT",
	}
	rules := []models.SiteRule{
		{
			HostPattern:   "*.ignorado.com.br",
			IgnoreCapture: true,
		},
	}

	// 1. No evento chrome.downloads.onCreated o fileName ainda é vazio ("") e a URL de CDN não tem extensão no path
	hfURL := "https://cas-bridge.xethub.hf.co/xet-bridge-us/69dc9a3d52797c523c6c91bc?X-Amz-Algorithm=AWS4-HMAC-SHA256&response-content-disposition=inline%3B+filename*%3DUTF-8%27%27qwen3vl_32b_minimax_h3_int8_convrot.safetensors%3B+filename%3D%22qwen3vl_32b_minimax_h3_int8_convrot.safetensors%22%3B"
	if ok, _ := ShouldCaptureBrowserDownload(hfURL, "", "https://huggingface.co/", "", settings, rules); !ok {
		t.Fatalf("deveria capturar download .safetensors do HuggingFace XetHub mesmo antes do fileName estar preenchido")
	}

	// 2. GitHub codeload URL sem extensão explícita no path
	ghURL := "https://codeload.github.com/wailsapp/wails/zip/refs/heads/master"
	if ok, _ := ShouldCaptureBrowserDownload(ghURL, "", "https://github.com/wailsapp/wails", "", settings, rules); !ok {
		t.Fatalf("deveria capturar download do GitHub codeload no evento onCreated")
	}

	// 3. Tecla ALT pressionada deve continuar ignorando a captura
	if ok, _ := ShouldCaptureBrowserDownload(hfURL, "", "https://huggingface.co/", "ALT", settings, rules); ok {
		t.Fatalf("deveria ignorar captura quando ALT está pressionado")
	}

	// 4. Site na lista de exceções deve continuar ignorando a captura
	if ok, _ := ShouldCaptureBrowserDownload(ghURL, "", "https://painel.ignorado.com.br/projeto", "", settings, rules); ok {
		t.Fatalf("deveria ignorar captura quando Referer está na lista de exceções")
	}

	// 5. Página HTML comum não deve ser capturada (nem quando customFileName termina em .html/.mhtml ao salvar página)
	if ok, _ := ShouldCaptureBrowserDownload("https://exemplo.com/artigo.html", "", "", "", settings, rules); ok {
		t.Fatalf("não deveria capturar página .html")
	}
	if ok, _ := ShouldCaptureBrowserDownload("https://exemplo.com/painel", "pagina_salva.html", "", "", settings, rules); ok {
		t.Fatalf("não deveria capturar salvamento de página .html mesmo com customFileName preenchido")
	}

	// 6. Verifica extração de nome de arquivo da query string response-content-disposition e do GitHub codeload
	if got := engine.ExtractFileNameFromURL(hfURL); got != "qwen3vl_32b_minimax_h3_int8_convrot.safetensors" {
		t.Fatalf("nome extraído incorreto para HuggingFace XetHub: obteve %q", got)
	}
	if got := engine.ExtractFileNameFromURL(ghURL); got != "wails-master.zip" {
		t.Fatalf("nome extraído incorreto para GitHub codeload: obteve %q", got)
	}

	// 7. Verifica derivação de IDs de extensão unpacked do Chrome
	ids := DeriveUnpackedExtensionIDs("d:\\RAFATECH PROJETOS\\APLICATIVOS\\DOWNLOAD-MANAGER\\browser-extension")
	if len(ids) == 0 {
		t.Fatalf("deveria gerar pelo menos um ID de extensão unpacked")
	}
	for _, id := range ids {
		if len(id) != 32 {
			t.Fatalf("ID de extensão Chrome inválido (esperado 32 chars): %q", id)
		}
	}
}

// TestGenerateOpenIDMICOAndInstallerPipeline valida a geração do ícone multi-resolução (.ico + PNGs da extensão),
// criação de atalhos .lnk, página interativa extension-setup.html e execução completa do instalador.
func TestGenerateOpenIDMICOAndInstallerPipeline(t *testing.T) {
	icoBytes, err := GenerateOpenIDMICO()
	if err != nil {
		t.Fatalf("erro em GenerateOpenIDMICO: %v", err)
	}
	if len(icoBytes) < 1024 {
		t.Fatalf("arquivo .ico muito pequeno: %d bytes", len(icoBytes))
	}
	if binary.LittleEndian.Uint16(icoBytes[0:2]) != 0 || binary.LittleEndian.Uint16(icoBytes[2:4]) != 1 {
		t.Fatalf("cabeçalho ICO inválido: %v", icoBytes[:6])
	}
	count := binary.LittleEndian.Uint16(icoBytes[4:6])
	if count != 6 {
		t.Fatalf("esperava 6 resoluções no .ico (256..16), obteve %d", count)
	}

	tempRoot := t.TempDir()
	srcDir := filepath.Join(tempRoot, "source")
	installDir := filepath.Join(tempRoot, "installed")
	if err := os.MkdirAll(filepath.Join(srcDir, "browser-extension"), 0o755); err != nil {
		t.Fatalf("erro ao criar pasta source: %v", err)
	}
	_ = os.WriteFile(filepath.Join(srcDir, "openidm.exe"), []byte("MZ-FAKE-APP"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "openidm-native-host.exe"), []byte("MZ-FAKE-HOST"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "browser-extension", "manifest.json"), []byte(`{"manifest_version":3}`), 0o644)

	var stepsSeen int
	res, err := RunInstallation(InstallOptions{
		InstallDir:            installDir,
		SourceDir:             srcDir,
		StartWithWindows:      true,
		StartInTray:           true,
		CreateDesktopShortcut: true,
		CreateStartMenu:       true,
		RegisterUninstaller:   true,
		InstallBrowserExt:     true,
		LaunchBrowserExtSetup: false,
		LaunchAppAfterInstall: false,
	}, func(step, total int, message string) {
		stepsSeen++
	})
	if err != nil {
		t.Fatalf("erro em RunInstallation: %v", err)
	}
	if !res.Success || stepsSeen != 7 {
		t.Fatalf("esperava Success=true e 7 etapas de progresso, obteve Success=%v stepsSeen=%d", res.Success, stepsSeen)
	}

	// Verifica que openidm.ico, PNGs da extensão, extension-setup.html, manifestos e atalhos foram criados
	for _, mustExist := range []string{
		res.IconPath,
		res.AppExePath,
		res.NativeHostExePath,
		res.ExtensionSetupHTML,
		res.ChromeManifestPath,
		res.FirefoxManifestPath,
		filepath.Join(res.ExtensionDir, "icons", "icon16.png"),
		filepath.Join(res.ExtensionDir, "icons", "icon32.png"),
		filepath.Join(res.ExtensionDir, "icons", "icon48.png"),
		filepath.Join(res.ExtensionDir, "icons", "icon128.png"),
		res.DesktopShortcutPath,
	} {
		if mustExist == "" || !fileExists(mustExist) {
			t.Fatalf("artefato obrigatório da instalação não encontrado: %q", mustExist)
		}
	}

	htmlData, _ := os.ReadFile(res.ExtensionSetupHTML)
	if !strings.Contains(string(htmlData), "browser-extension") || !strings.Contains(string(htmlData), "en-US") {
		t.Fatalf("extension-setup.html não contém o caminho da extensão ou tradução en-US")
	}
}

// TestBridgeHealthAndLanguageSettings valida que /api/health expõe o idioma ativo para a extensão MV3
// e que POST /api/settings normaliza o idioma e aciona SetSettingsUpdatedHandler.
func TestBridgeHealthAndLanguageSettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_bridge_lang.sqlite")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("erro ao criar store: %v", err)
	}
	defer st.Close()

	mgr, err := engine.NewManager(st)
	if err != nil {
		t.Fatalf("erro ao criar manager: %v", err)
	}
	sched := scheduler.NewScheduler(st, mgr)
	bridge := NewBridgeServer(st, mgr, sched, nil)

	var callbackCfg models.AppSettings
	bridge.SetSettingsUpdatedHandler(func(cfg models.AppSettings) {
		callbackCfg = cfg
	})

	// 1. /api/health inicial deve retornar language="pt-BR"
	req1 := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec1 := httptest.NewRecorder()
	bridge.handleHealth(rec1, req1)
	var health1 map[string]any
	_ = json.Unmarshal(rec1.Body.Bytes(), &health1)
	if health1["language"] != models.LangPtBR {
		t.Fatalf("esperava language='pt-BR' em /api/health inicial, obteve %v", health1["language"])
	}

	// 2. Atualiza idioma via POST /api/settings com "en" -> deve normalizar para "en-US"
	cfg, _ := st.GetSettings()
	cfg.Language = "en"
	body, _ := json.Marshal(cfg)
	req2 := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewReader(body))
	rec2 := httptest.NewRecorder()
	bridge.handleSettings(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("esperava HTTP 200 em POST /api/settings, obteve %d", rec2.Code)
	}
	if callbackCfg.Language != models.LangEnUS {
		t.Fatalf("esperava callback com Language='en-US', obteve %q", callbackCfg.Language)
	}

	// 3. /api/health agora deve retornar language="en-US"
	req3 := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec3 := httptest.NewRecorder()
	bridge.handleHealth(rec3, req3)
	var health3 map[string]any
	_ = json.Unmarshal(rec3.Body.Bytes(), &health3)
	if health3["language"] != models.LangEnUS {
		t.Fatalf("esperava language='en-US' em /api/health após atualização, obteve %v", health3["language"])
	}

	// 4. POST /api/language altera o idioma para "pt-BR" e GET /api/language reflete a mudança
	req4 := httptest.NewRequest(http.MethodPost, "/api/language", strings.NewReader(`{"language":"pt"}`))
	rec4 := httptest.NewRecorder()
	bridge.handleLanguage(rec4, req4)
	if rec4.Code != http.StatusOK || callbackCfg.Language != models.LangPtBR {
		t.Fatalf("esperava HTTP 200 e callback Language='pt-BR' em POST /api/language, obteve code=%d lang=%q", rec4.Code, callbackCfg.Language)
	}

	// 5. POST /api/capture com action="set_language" altera para "en-US" e ping retorna language="en-US"
	req5 := httptest.NewRequest(http.MethodPost, "/api/capture", strings.NewReader(`{"action":"set_language","language":"en-US"}`))
	rec5 := httptest.NewRecorder()
	bridge.handleBrowserCapture(rec5, req5)
	var capResp NativeMessageResponse
	_ = json.Unmarshal(rec5.Body.Bytes(), &capResp)
	if !capResp.Success || capResp.Language != models.LangEnUS {
		t.Fatalf("esperava Success=true e Language='en-US' em action=set_language, obteve %+v", capResp)
	}

	req6 := httptest.NewRequest(http.MethodPost, "/api/capture", strings.NewReader(`{"action":"ping","fileName":"ext-test"}`))
	rec6 := httptest.NewRecorder()
	bridge.handleBrowserCapture(rec6, req6)
	var pingResp NativeMessageResponse
	_ = json.Unmarshal(rec6.Body.Bytes(), &pingResp)
	if pingResp.Language != models.LangEnUS {
		t.Fatalf("esperava Language='en-US' no retorno de ping da extensão, obteve %q", pingResp.Language)
	}
}





