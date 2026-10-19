package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openidm/internal/engine"
	"openidm/internal/integration"
	"openidm/internal/models"
)

// TestProbeAndClassifyURL_RedirectAndContentDisposition verifica que links capturados sem extensão
// (ex: /download?id=99 -> 302 -> /redirect2 -> Content-Disposition: filename="TurboSetup.exe")
// são reclassificados corretamente na categoria "programs" (Programas) mesmo quando initialCapture.FileName já veio preenchido.
func TestProbeAndClassifyURL_RedirectAndContentDisposition(t *testing.T) {
	payload := []byte("MZ-FAKE-INSTALLER-BINARY-CONTENT-1234567890")

	mux := http.NewServeMux()
	mux.HandleFunc("/step1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/step2?token=xyz", http.StatusFound)
	})
	mux.HandleFunc("/step2", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final-download", http.StatusFound)
	})
	mux.HandleFunc("/final-download", func(w http.ResponseWriter, r *http.Request) {
		// Simula servidor que só envia Content-Disposition no GET (ou ambos) e suporta Range
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Disposition", `attachment; filename="TurboSetup_v2.exe"`)
		if r.Header.Get("Range") == "bytes=0-0" {
			w.Header().Set("Content-Range", "bytes 0-0/43")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[:1])
			return
		}
		w.Header().Set("Content-Length", "43")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dbPath := filepath.Join(t.TempDir(), "test_app.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	rawURL := srv.URL + "/step1?id=99"

	// 1. Captura inicial do clique (antes do probe HTTP): nome provisório é "step1" e categoria "general"
	initialPrompt := app.PromptCaptureURL(rawURL, "", "https://exemplo.com", "browser")
	if initialPrompt.FileName != "step1" || initialPrompt.CategoryID != "general" {
		t.Fatalf("prompt inicial inesperado: %+v", initialPrompt)
	}

	// 2. Simula exatamente a chamada que AddUrlModal.tsx faz passando initialPrompt.FileName ("step1")
	probed, err := app.ProbeAndClassifyURL(rawURL, initialPrompt.FileName)
	if err != nil {
		t.Fatalf("erro em ProbeAndClassifyURL: %v", err)
	}
	if !probed.Probed {
		t.Fatalf("esperava Probed=true")
	}
	if probed.FileName != "TurboSetup_v2.exe" {
		t.Fatalf("esperava FileName='TurboSetup_v2.exe' vindo do Content-Disposition pós-302, obteve '%s'", probed.FileName)
	}
	if probed.CategoryID != "programs" {
		t.Fatalf("esperava CategoryID='programs', obteve '%s'", probed.CategoryID)
	}
	if !strings.HasSuffix(strings.ToLower(probed.SaveDir), "programas") {
		t.Fatalf("esperava SaveDir apontando para pasta Programas, obteve '%s'", probed.SaveDir)
	}
	if probed.TotalSize != 43 || !probed.AcceptRanges {
		t.Fatalf("metadados de tamanho/Range incorretos: size=%d range=%v", probed.TotalSize, probed.AcceptRanges)
	}
}

// TestProbeAndClassifyURL_RedirectURLWithoutContentDisposition verifica que quando uma URL sem extensão
// redireciona via 302 para uma URL de CDN com extensão (.zip) sem cabeçalho Content-Disposition,
// o nome do arquivo e a categoria ("compressed") são extraídos da URL final pós-redirecionamento.
func TestProbeAndClassifyURL_RedirectURLWithoutContentDisposition(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/cdn/files/pacote_oficial.zip", http.StatusFound)
	})
	mux.HandleFunc("/cdn/files/pacote_oficial.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dbPath := filepath.Join(t.TempDir(), "test_redirect.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	rawURL := srv.URL + "/get?file=123"
	initial := app.PromptCaptureURL(rawURL, "", "", "browser")
	probed, err := app.ProbeAndClassifyURL(rawURL, initial.FileName)
	if err != nil {
		t.Fatalf("erro em ProbeAndClassifyURL: %v", err)
	}
	if probed.FileName != "pacote_oficial.zip" {
		t.Fatalf("esperava 'pacote_oficial.zip' da URL final pós-302, obteve '%s'", probed.FileName)
	}
	if probed.CategoryID != "compressed" {
		t.Fatalf("esperava CategoryID='compressed', obteve '%s'", probed.CategoryID)
	}
}

// TestExtractFileNameFromRootURLOnWindows garante que URLs raiz retornem "download.bin" e não "_".
func TestExtractFileNameFromRootURLOnWindows(t *testing.T) {
	for _, u := range []string{"https://exemplo.com", "https://exemplo.com/", "https://exemplo.com///"} {
		got := engine.ExtractFileNameFromURL(u)
		if got != "download.bin" {
			t.Fatalf("ExtractFileNameFromURL(%q) = %q, esperava 'download.bin'", u, got)
		}
	}
}

// TestBeforeCloseHidesToTrayUntilQuit garante que clicar no X oculta na bandeja do relógio sem encerrar o processo.
func TestBeforeCloseHidesToTrayUntilQuit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_close.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	if prevent := app.BeforeClose(context.Background()); !prevent {
		t.Fatalf("BeforeClose deveria retornar true (impedir fechamento e ocultar no relógio) quando quitting=false")
	}
	app.quitting = true
	if prevent := app.BeforeClose(context.Background()); prevent {
		t.Fatalf("BeforeClose deveria retornar false (permitir encerramento) quando quitting=true")
	}
}

// TestPromptCaptureURLDeduplicationAndUpdate garante que GetPendingCapture não apaga o estado de deduplicação
// e que um segundo evento com nome resolvido atualiza a categoria e nome do arquivo.
func TestPromptCaptureURLDeduplicationAndUpdate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_dedup.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	// 1. Primeiro evento (ex: CLICK_CAPTURE com download="custom_name.zip")
	p1 := app.PromptCaptureURL("https://exemplo.com/api/export", "custom_name.zip", "https://exemplo.com", "browser")
	if p1.FileName != "custom_name.zip" || p1.CategoryID != "compressed" {
		t.Fatalf("p1 inesperado: %+v", p1)
	}

	// Frontend consome GetPendingCapture imediatamente
	if got := app.GetPendingCapture(); got == nil || got.FileName != "custom_name.zip" {
		t.Fatalf("GetPendingCapture deveria retornar p1, obteve %+v", got)
	}

	// 2. Segundo evento 20ms depois (ex: onCreated com fileName="") NÃO deve sobrescrever custom_name.zip por "export"
	p2 := app.PromptCaptureURL("https://exemplo.com/api/export", "", "https://exemplo.com", "browser")
	if p2.FileName != "custom_name.zip" || p2.CategoryID != "compressed" {
		t.Fatalf("deduplicação falhou após GetPendingCapture: FileName foi sobrescrito para %q (cat=%q)", p2.FileName, p2.CategoryID)
	}

	// 3. Se o primeiro evento veio sem nome (onCreated) e o segundo traz o nome resolvido (onDeterminingFilename), deve atualizar!
	p3 := app.PromptCaptureURL("https://exemplo.com/api/report", "", "https://exemplo.com", "browser")
	if p3.FileName != "report" || p3.CategoryID != "general" {
		t.Fatalf("p3 inesperado: %+v", p3)
	}
	_ = app.GetPendingCapture()

	p4 := app.PromptCaptureURL("https://exemplo.com/api/report", "relatorio_2026.pdf", "https://exemplo.com", "browser")
	if p4.FileName != "relatorio_2026.pdf" || p4.CategoryID != "documents" {
		t.Fatalf("esperava atualização para relatorio_2026.pdf / documents, obteve %+v", p4)
	}

	// 4. Verifica normalização de links /blob/ do HuggingFace e GitHub
	hfNorm := engine.NormalizeDownloadURL("https://huggingface.co/Comfy-Org/MiniMax-H3/blob/main/text_encoders/model.safetensors")
	if hfNorm != "https://huggingface.co/Comfy-Org/MiniMax-H3/resolve/main/text_encoders/model.safetensors" {
		t.Fatalf("NormalizeDownloadURL HuggingFace incorreto: %s", hfNorm)
	}
	ghNorm := engine.NormalizeDownloadURL("https://github.com/owner/repo/blob/main/release.zip")
	if ghNorm != "https://github.com/owner/repo/raw/main/release.zip" {
		t.Fatalf("NormalizeDownloadURL GitHub incorreto: %s", ghNorm)
	}
}

// TestProbeAndClassifyURL_CDNBlobRedirectPreservesExtension garante que redirecionamentos 302 para hashes sem extensão
// (ex: /blobs/69dc9a3d52797c523c6c91bc) não sobrescrevam o nome válido extraído da URL original.
func TestProbeAndClassifyURL_CDNBlobRedirectPreservesExtension(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/resolve/main/instalador_oficial.exe", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/blobs/69dc9a3d52797c523c6c91bc", http.StatusFound)
	})
	mux.HandleFunc("/blobs/69dc9a3d52797c523c6c91bc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", "2048")
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dbPath := filepath.Join(t.TempDir(), "test_cdn_hash.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	rawURL := srv.URL + "/resolve/main/instalador_oficial.exe"
	initial := app.PromptCaptureURL(rawURL, "", "", "browser")
	if initial.FileName != "instalador_oficial.exe" || initial.CategoryID != "programs" {
		t.Fatalf("classificação inicial incorreta: %+v", initial)
	}

	probed, err := app.ProbeAndClassifyURL(rawURL, initial.FileName)
	if err != nil {
		t.Fatalf("erro em ProbeAndClassifyURL: %v", err)
	}
	if probed.FileName != "instalador_oficial.exe" {
		t.Fatalf("o hash CDN sem extensão sobrescreveu o nome do arquivo! esperava 'instalador_oficial.exe', obteve %q", probed.FileName)
	}
	if probed.CategoryID != "programs" {
		t.Fatalf("esperava CategoryID='programs', obteve %q", probed.CategoryID)
	}
}

// TestNormalizeAndExtractSpecialProviders valida normalização de Google Drive, Dropbox e SourceForge.
func TestNormalizeAndExtractSpecialProviders(t *testing.T) {
	gdrive := engine.NormalizeDownloadURL("https://drive.google.com/file/d/1AbCdEfGhIjKlMnOpQrStUvWxYz/view?usp=sharing")
	if !strings.Contains(gdrive, "drive.usercontent.google.com/download") || !strings.Contains(gdrive, "id=1AbCdEfGhIjKlMnOpQrStUvWxYz") {
		t.Fatalf("normalização Google Drive inesperada: %s", gdrive)
	}

	dropbox := engine.NormalizeDownloadURL("https://www.dropbox.com/s/xyz123/pacote.zip?dl=0")
	if !strings.Contains(dropbox, "dl=1") {
		t.Fatalf("normalização Dropbox inesperada: %s", dropbox)
	}

	sfName := engine.ExtractFileNameFromURL("https://sourceforge.net/projects/sevenzip/files/7-Zip/24.08/7z2408-x64.exe/download")
	if sfName != "7z2408-x64.exe" {
		t.Fatalf("extração SourceForge inesperada: esperava '7z2408-x64.exe', obteve %q", sfName)
	}

	gdriveOpen := engine.NormalizeDownloadURL("https://drive.google.com/open?id=1ZyXwVuTsRqPoNmLkJiHgFeDcBa")
	if !strings.Contains(gdriveOpen, "drive.usercontent.google.com/download") || !strings.Contains(gdriveOpen, "id=1ZyXwVuTsRqPoNmLkJiHgFeDcBa") {
		t.Fatalf("normalização Google Drive /open?id= inesperada: %s", gdriveOpen)
	}

	dropboxScl := engine.NormalizeDownloadURL("https://www.dropbox.com/scl/fi/abc123/relatorio.pdf?rlkey=xyz")
	if !strings.Contains(dropboxScl, "dl=1") {
		t.Fatalf("normalização Dropbox /scl/fi/ sem dl=0 inesperada: %s", dropboxScl)
	}

	mfName := engine.ExtractFileNameFromURL("https://www.mediafire.com/file/abc123xyz/Pacote_Mod_v3.rar/file")
	if mfName != "Pacote_Mod_v3.rar" {
		t.Fatalf("extração MediaFire inesperada: esperava 'Pacote_Mod_v3.rar', obteve %q", mfName)
	}

	ghArch := engine.ExtractFileNameFromURL("https://github.com/wailsapp/wails/archive/refs/heads/master.zip")
	if ghArch != "wails-master.zip" {
		t.Fatalf("extração GitHub archive inesperada: esperava 'wails-master.zip', obteve %q", ghArch)
	}
}

// TestProbeAndClassifyURL_DeadOr404ServerReturnsProbedFalse garante que servidores retornando HTTP 404/500
// não sejam marcados como Probed=true ("Dinâmico / Stream") e preencham ProbeError.
func TestProbeAndClassifyURL_DeadOr404ServerReturnsProbedFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	dbPath := filepath.Join(t.TempDir(), "test_404.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	probed, err := app.ProbeAndClassifyURL(srv.URL+"/inexistente.zip", "")
	if err != nil {
		t.Fatalf("ProbeAndClassifyURL não deve retornar erro fatal Go: %v", err)
	}
	if probed.Probed {
		t.Fatalf("esperava Probed=false para link HTTP 404, mas obteve Probed=true")
	}
	if !strings.Contains(probed.ProbeError, "404") {
		t.Fatalf("esperava ProbeError contendo '404', obteve %q", probed.ProbeError)
	}
}

// TestPromptCaptureURL_PreservesValidExtensionAndClassifiesSafetensors garante que arquivos .safetensors
// sejam classificados em "programs" e que um segundo evento com hash de blob sem extensão não sobrescreva o nome válido.
func TestPromptCaptureURL_PreservesValidExtensionAndClassifiesSafetensors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_safetensors.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())
	app.ensureExtendedCaptureSettings()

	rawURL := "https://huggingface.co/Comfy-Org/MiniMax-H3/resolve/main/qwen3vl_32b.safetensors"
	p1 := app.PromptCaptureURL(rawURL, "qwen3vl_32b.safetensors", "https://huggingface.co/", "browser")
	if p1.FileName != "qwen3vl_32b.safetensors" || p1.CategoryID != "programs" {
		t.Fatalf("esperava qwen3vl_32b.safetensors na categoria 'programs', obteve FileName=%q CategoryID=%q", p1.FileName, p1.CategoryID)
	}

	// Simula segundo evento disparado pelo navegador com hash de blob sem extensão para a mesma URL ou mesmo referrer
	p2 := app.PromptCaptureURL(rawURL, "69dc9a3d52797c523c6c91bc", "https://huggingface.co/", "browser")
	if p2.FileName != "qwen3vl_32b.safetensors" || p2.CategoryID != "programs" {
		t.Fatalf("hash sem extensão sobrescreveu nome válido! obteve FileName=%q CategoryID=%q", p2.FileName, p2.CategoryID)
	}

	p3 := app.PromptCaptureURL("https://cdn-lfs.hf.co/repos/ab/cd/69dc9a3d52797c523c6c91bc", "69dc9a3d52797c523c6c91bc", "https://huggingface.co/", "browser")
	if p3.FileName != "qwen3vl_32b.safetensors" || p3.CategoryID != "programs" {
		t.Fatalf("redirecionamento CDN sem extensão no mesmo referrer deveria preservar 'qwen3vl_32b.safetensors', obteve FileName=%q CategoryID=%q", p3.FileName, p3.CategoryID)
	}
	queue := app.GetPendingCaptureQueue()
	if len(queue) != 1 || queue[0].URL != "https://cdn-lfs.hf.co/repos/ab/cd/69dc9a3d52797c523c6c91bc" {
		t.Fatalf("esperava que o redirecionamento CDN substituísse a entrada na fila sem duplicar, obteve len=%d queue=%+v", len(queue), queue)
	}
}

// TestRunDownloadTask_ScriptURLReclassifiesSaveDirAndSendsReferer verifica que um download iniciado com URL de script
// (download.php -> categoria "general" e pasta ".../Geral") que exige cabeçalho Referer e retorna Content-Disposition
// "instalador_real.exe" atualiza tanto CategoryID ("programs") quanto SaveDir (".../Programas") e conclui o download.
func TestRunDownloadTask_ScriptURLReclassifiesSaveDirAndSendsReferer(t *testing.T) {
	payload := []byte("MZ-EXECUTABLE-BINARY-DATA-WITH-REFERER-PROTECTION-1234567890")
	expectedRef := "https://portal.exemplo.com/pagina-download"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != expectedRef {
			http.Error(w, "403 Forbidden - Referer ausente", http.StatusForbidden)
			return
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Disposition", `attachment; filename="instalador_real.exe"`)
		if r.Header.Get("Range") == "bytes=0-0" {
			w.Header().Set("Content-Range", "bytes 0-0/60")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[:1])
			return
		}
		w.Header().Set("Content-Length", "60")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload)
		}
	}))
	defer srv.Close()

	dbPath := filepath.Join(t.TempDir(), "test_script_dl.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	// Isola as pastas de categorias dentro do TempDir do teste
	tmpDownloads := t.TempDir()
	cats, _ := app.GetCategories()
	for _, c := range cats {
		c.DefaultDir = filepath.Join(tmpDownloads, c.Slug)
		_ = app.SaveCategory(c)
	}

	scriptURL := srv.URL + "/download.php?id=777"
	dl, err := app.AddDownload(models.AddDownloadRequest{
		URL:            scriptURL,
		Referrer:       expectedRef,
		MaxConnections: 2,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro em AddDownload: %v", err)
	}

	// Inicialmente, "download.php" cai na categoria "general"
	if dl.CategoryID != "general" {
		t.Fatalf("esperava categoria inicial 'general' para download.php, obteve %q", dl.CategoryID)
	}

	// Aguarda conclusão do download e verifica se foi reclassificado para "programs" e salvo na pasta "programs"
	var finalDl *models.Download
	for i := 0; i < 60; i++ {
		cur, _ := app.GetDownload(dl.ID)
		if cur != nil && (cur.Status == models.StatusCompleted || cur.Status == models.StatusError) {
			finalDl = cur
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if finalDl == nil || finalDl.Status != models.StatusCompleted {
		t.Fatalf("download não concluiu com sucesso: %+v", finalDl)
	}
	if finalDl.FileName != "instalador_real.exe" {
		t.Fatalf("esperava FileName='instalador_real.exe', obteve %q", finalDl.FileName)
	}
	if finalDl.CategoryID != "programs" {
		t.Fatalf("esperava CategoryID='programs' após Content-Disposition, obteve %q", finalDl.CategoryID)
	}
	expectedDir := filepath.Join(tmpDownloads, "programs")
	if finalDl.SaveDir != expectedDir {
		t.Fatalf("SaveDir não foi atualizado da pasta 'general' para 'programs'! esperava %q, obteve %q", expectedDir, finalDl.SaveDir)
	}
}

// TestAppInstallerAndAutoStartSettings verifica os métodos do instalador expostos na fachada App (ShouldOpenInstaller, RunSystemInstaller e StartWithWindows).
func TestAppInstallerAndAutoStartSettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_installer_app.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	cfg, err := app.GetSettings()
	if err != nil {
		t.Fatalf("erro em GetSettings: %v", err)
	}
	if !cfg.StartWithWindows {
		t.Fatalf("esperava StartWithWindows=true por padrão no banco recém-criado")
	}

	app.SetOpenInstallerOnStartup(true)
	if !app.ShouldOpenInstaller() {
		t.Fatalf("ShouldOpenInstaller deveria retornar true na primeira consulta")
	}
	if app.ShouldOpenInstaller() {
		t.Fatalf("ShouldOpenInstaller deveria limpar a flag após consumo")
	}

	targetInstall := filepath.Join(t.TempDir(), "OpenIDM_Installed")
	res, err := app.RunSystemInstaller(integration.InstallOptions{
		InstallDir:            targetInstall,
		StartWithWindows:      false,
		StartInTray:           true,
		CreateDesktopShortcut: true,
		CreateStartMenu:       true,
		RegisterUninstaller:   true,
		InstallBrowserExt:     true,
		LaunchBrowserExtSetup: false,
		LaunchAppAfterInstall: false,
	})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("erro em RunSystemInstaller: res=%+v err=%v", res, err)
	}

	updatedCfg, _ := app.GetSettings()
	if updatedCfg.StartWithWindows {
		t.Fatalf("esperava que RunSystemInstaller sincronizasse StartWithWindows=false no SQLite")
	}
}

// TestStandaloneCardModeAndMultipleDownloadsList valida que a captura de links abre apenas o Card Solto ("card"),
// empilha capturas múltiplas e junta todos os downloads em uma lista no mesmo card mantendo o app principal no relógio.
func TestStandaloneCardModeAndMultipleDownloadsList(t *testing.T) {
	payload1 := []byte("CONTEUDO-ARQUIVO-1-PARA-TESTE-CARD-SOLTO-1234567890")
	payload2 := []byte("CONTEUDO-ARQUIVO-2-PARA-TESTE-CARD-SOLTO-ABCDEFGHIJ")

	mux := http.NewServeMux()
	mux.HandleFunc("/file1.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", "51")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload1)
		}
	})
	mux.HandleFunc("/file2.exe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", "51")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload2)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dbPath := filepath.Join(t.TempDir(), "test_card_mode.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	saveDir := t.TempDir()

	// 1. Ao capturar o primeiro link, a janela deve mudar para o modo "card" (Card Solto) sem abrir o modo "full".
	url1 := srv.URL + "/file1.zip"
	p1 := app.PromptCaptureURL(url1, "file1.zip", "https://exemplo.com", "browser")
	if p1 == nil || app.GetWindowMode() != "card" {
		t.Fatalf("esperava modo 'card' ao capturar link, obteve %q", app.GetWindowMode())
	}

	// 2. Se o usuário clicar em um segundo link antes mesmo de confirmar o primeiro, ambos ficam na fila do card.
	url2 := srv.URL + "/file2.exe"
	p2 := app.PromptCaptureURL(url2, "file2.exe", "https://exemplo.com", "browser")
	if p2 == nil || app.GetWindowMode() != "card" {
		t.Fatalf("esperava modo 'card' no segundo link, obteve %q", app.GetWindowMode())
	}
	queue := app.GetPendingCaptureQueue()
	if len(queue) != 2 {
		t.Fatalf("esperava 2 links na fila de captura do card, obteve %d", len(queue))
	}

	// 3. Inicia o primeiro e o segundo download pelo card e verifica se ambos se juntam na lista do mesmo Card Solto.
	dl1, err := app.AddDownload(models.AddDownloadRequest{
		URL:            url1,
		FileName:       "file1.zip",
		SaveDir:        saveDir,
		CategoryID:     "compressed",
		MaxConnections: 2,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro ao adicionar dl1: %v", err)
	}
	if len(app.GetPendingCaptureQueue()) != 1 {
		t.Fatalf("esperava que restasse 1 link pendente na fila do card após confirmar dl1, obteve %d", len(app.GetPendingCaptureQueue()))
	}

	dl2, err := app.AddDownload(models.AddDownloadRequest{
		URL:            url2,
		FileName:       "file2.exe",
		SaveDir:        saveDir,
		CategoryID:     "programs",
		MaxConnections: 2,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro ao adicionar dl2: %v", err)
	}
	if len(app.GetPendingCaptureQueue()) != 0 {
		t.Fatalf("esperava fila de captura vazia após confirmar dl2, obteve %d", len(app.GetPendingCaptureQueue()))
	}

	// Verifica que os dois downloads estão juntos na lista do Card Solto e que o modo permanece "card".
	cardList := app.GetCardDownloads()
	if len(cardList) != 2 {
		t.Fatalf("esperava 2 downloads juntos na lista do Card Solto, obteve %d", len(cardList))
	}
	if app.GetWindowMode() != "card" {
		t.Fatalf("esperava que o app principal continuasse minimizado com janela em modo 'card', obteve %q", app.GetWindowMode())
	}

	// 4. Aguarda conclusão de ambos e valida que continuam visíveis na lista do card até o usuário limpar/dispensar.
	for i := 0; i < 60; i++ {
		c1, _ := app.GetDownload(dl1.ID)
		c2, _ := app.GetDownload(dl2.ID)
		if c1 != nil && c2 != nil && c1.Status == models.StatusCompleted && c2.Status == models.StatusCompleted {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	cardAfterComplete := app.GetCardDownloads()
	if len(cardAfterComplete) != 2 {
		t.Fatalf("esperava que ambos os downloads concluídos permanecessem na lista do card para acesso rápido, obteve %d", len(cardAfterComplete))
	}

	// 5. Testa remoção individual e limpeza de concluídos da lista do card sem excluir do histórico principal.
	app.DismissCardDownload(dl1.ID)
	if len(app.GetCardDownloads()) != 1 {
		t.Fatalf("esperava 1 item no card após DismissCardDownload, obteve %d", len(app.GetCardDownloads()))
	}
	app.ClearCompletedFromCard()
	if len(app.GetCardDownloads()) != 0 {
		t.Fatalf("esperava 0 itens no card após ClearCompletedFromCard, obteve %d", len(app.GetCardDownloads()))
	}
	if len(app.GetDownloads()) != 2 {
		t.Fatalf("ClearCompletedFromCard não deveria apagar downloads do histórico principal, obteve %d", len(app.GetDownloads()))
	}

	// 6. Testa transição explícita entre Card Solto, Ocultação na Bandeja e App Completo e limites de ResizeStandaloneCard.
	app.ResizeStandaloneCard(100, 100)
	if app.cardWidth < 500 || app.cardHeight < 200 {
		t.Fatalf("ResizeStandaloneCard deveria aplicar limites mínimos retangulares seguros (500x200), obteve %dx%d", app.cardWidth, app.cardHeight)
	}
	app.CloseStandaloneCard()
	if !app.windowHidden {
		t.Fatalf("esperava windowHidden=true após CloseStandaloneCard")
	}
	// Redimensionamento em background enquanto oculto no relógio deve atualizar dimensões sem tirar windowHidden=true
	app.ResizeStandaloneCard(680, 430)
	if !app.windowHidden || app.cardWidth != 680 || app.cardHeight != 430 {
		t.Fatalf("ResizeStandaloneCard com card oculto no relógio deveria preservar windowHidden=true e gravar 680x430, obteve hidden=%v %dx%d", app.windowHidden, app.cardWidth, app.cardHeight)
	}
	app.OpenFullAppFromCard()
	if app.GetWindowMode() != "full" || app.windowHidden {
		t.Fatalf("esperava modo 'full' e windowHidden=false após OpenFullAppFromCard, obteve mode=%q hidden=%v", app.GetWindowMode(), app.windowHidden)
	}
	app.MinimizeToCard()
	if app.GetWindowMode() != "card" || app.windowHidden {
		t.Fatalf("esperava modo 'card' e windowHidden=false após MinimizeToCard, obteve mode=%q hidden=%v", app.GetWindowMode(), app.windowHidden)
	}
}

// TestCardPositionPersistenceAndSpeedStability valida que a posição (X, Y) do card reduzido é salva como preferência
// em AppSettings (SQLite), preservada mesmo após UpdateSettings, e que AdvanceSegment não zera a velocidade em leituras rápidas (< 200ms).
func TestCardPositionPersistenceAndSpeedStability(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_card_pos_speed.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	// 1. Verifica que inicialmente não há posição salva, e após SaveCardPosition ela persiste no banco SQLite
	pos0 := app.GetCardPosition()
	if saved, _ := pos0["saved"].(bool); saved {
		t.Fatalf("esperava saved=false inicialmente, obteve %+v", pos0)
	}

	app.SaveCardPosition(42, 315)
	pos1 := app.GetCardPosition()
	if saved, _ := pos1["saved"].(bool); !saved || pos1["x"] != 42 || pos1["y"] != 315 {
		t.Fatalf("esperava posição salva (42, 315), obteve %+v", pos1)
	}

	// Recria instância do App no mesmo banco para garantir que a preferência persiste entre reinicializações
	app.Shutdown(context.Background())
	app2, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao reabrir App: %v", err)
	}
	defer app2.Shutdown(context.Background())

	pos2 := app2.GetCardPosition()
	if saved, _ := pos2["saved"].(bool); !saved || pos2["x"] != 42 || pos2["y"] != 315 {
		t.Fatalf("esperava que a preferência de posição (42, 315) persistisse ao reabrir o banco, obteve %+v", pos2)
	}

	// Garante que salvar configurações gerais na modal de Opções não apaga a posição salva do card
	cfg, err := app2.GetSettings()
	if err != nil {
		t.Fatalf("erro em GetSettings: %v", err)
	}
	cfg.CardPosSaved = false
	cfg.CardPosX = 0
	cfg.CardPosY = 0
	if err := app2.SaveSettings(cfg); err != nil {
		t.Fatalf("erro em SaveSettings: %v", err)
	}
	pos3 := app2.GetCardPosition()
	if saved, _ := pos3["saved"].(bool); !saved || pos3["x"] != 42 || pos3["y"] != 315 {
		t.Fatalf("SaveSettings não deveria apagar a posição salva do card, obteve %+v", pos3)
	}

	// 2. Valida que AdvanceSegment não zera SpeedBps quando speedBps == 0 (leituras intermediárias < 200ms)
	table := engine.NewSegmentTable("dl-speed-test", 1024*1024, 256*1024, nil)
	seg, ok := table.AcquireNextSegment(1, false)
	if !ok || seg == nil {
		t.Fatalf("esperava adquirir segmento inicial")
	}
	table.AdvanceSegment(seg.Start, 32768, 5_000_000) // 5 MB/s medidos na janela de 200ms
	_, _, totalSpeed1, _, _ := table.Snapshot()
	if totalSpeed1 != 5_000_000 {
		t.Fatalf("esperava totalSpeed=5000000, obteve %d", totalSpeed1)
	}
	// Simula leitura rápida subsequente (< 200ms) onde speedBps é passado como 0
	table.AdvanceSegment(seg.Start, 32768, 0)
	_, _, totalSpeed2, _, _ := table.Snapshot()
	if totalSpeed2 != 5_000_000 {
		t.Fatalf("AdvanceSegment com speedBps=0 zerou a velocidade! esperava 5000000 preservado, obteve %d", totalSpeed2)
	}
	// Ao concluir o segmento, a velocidade dele deve ser limpa para 0
	table.CompleteSegment(seg.Start)
	_, _, totalSpeed3, _, _ := table.Snapshot()
	if totalSpeed3 != 0 {
		t.Fatalf("esperava totalSpeed=0 após conclusão do segmento, obteve %d", totalSpeed3)
	}

	// 3. Valida exclusão direta (ícone 🗑) de um download na lista do Card Reduzido
	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("PARCIAL-ANTES-DE-EXCLUIR"))
			time.Sleep(300 * time.Millisecond)
		}
	}))
	defer slowSrv.Close()

	dlActive, err := app2.AddDownload(models.AddDownloadRequest{
		URL:            slowSrv.URL + "/arquivo_em_andamento.bin",
		FileName:       "arquivo_em_andamento.bin",
		SaveDir:        t.TempDir(),
		CategoryID:     "general",
		MaxConnections: 1,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro ao iniciar download ativo para exclusão direta: %v", err)
	}
	if len(app2.GetCardDownloads()) != 1 {
		t.Fatalf("esperava 1 download na lista do card antes de excluir, obteve %d", len(app2.GetCardDownloads()))
	}
	if err := app2.DeleteDownload(dlActive.ID, true); err != nil {
		t.Fatalf("erro em DeleteDownload direto: %v", err)
	}
	if len(app2.GetCardDownloads()) != 0 {
		t.Fatalf("esperava 0 downloads na lista do card após exclusão direta, obteve %d", len(app2.GetCardDownloads()))
	}
	if len(app2.GetDownloads()) != 0 {
		t.Fatalf("esperava 0 downloads no banco após exclusão direta, obteve %d", len(app2.GetDownloads()))
	}
}

// TestRedirectDeduplicationTrayFullAppAndCleanDelete valida:
// 1) Deduplicação de redirecionamento 302 rápido com o mesmo nome de arquivo válido (ex: HuggingFace -> CDN) em um único item na fila.
// 2) Clicar no ícone da bandeja (ShowWindow) abre o App Completo ("full") e limpa pendingCapture para não sequestrar a tela de volta ao card.
// 3) DeleteDownload em download ativo não emite status intermediário "cancelled" ("Parado") antes de remover.
func TestRedirectDeduplicationTrayFullAppAndCleanDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_redirect_tray_del.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	// 1. Simula clique no link original (content_script) seguido de redirecionamento 302 para CDN (downloads.onCreated) com mesmo nome válido
	urlOrigin := "https://huggingface.co/Comfy-Org/model/resolve/main/model-00002-of-00048.safetensors"
	urlCDN := "https://cas-bridge.xethub.hf.co/xet-bridge-us/69dc9a3d52797c523c6c91bc?token=abc"
	ref := "https://huggingface.co/Comfy-Org/model/tree/main"

	app.PromptCaptureURL(urlOrigin, "model-00002-of-00048.safetensors", ref, "browser")
	p2 := app.PromptCaptureURL(urlCDN, "model-00002-of-00048.safetensors", ref, "browser")

	queue := app.GetPendingCaptureQueue()
	if len(queue) != 1 {
		t.Fatalf("esperava 1 único item na fila após redirecionamento 302 do mesmo arquivo, obteve %d: %+v", len(queue), queue)
	}
	if queue[0].URL != urlCDN || p2.FileName != "model-00002-of-00048.safetensors" {
		t.Fatalf("esperava URL substituída pelo CDN mantendo nome válido, obteve %+v", queue[0])
	}

	// 2. Simula clique no ícone do relógio (ShowWindow): deve abrir modo "full" e limpar pendingCapture
	app.ShowWindow()
	if app.GetWindowMode() != "full" {
		t.Fatalf("esperava windowMode='full' após ShowWindow, obteve %q", app.GetWindowMode())
	}
	if pending := app.GetPendingCapture(); pending != nil {
		t.Fatalf("ShowWindow deveria limpar pendingCapture para não forçar retorno ao modo card, mas obteve %+v", pending)
	}

	// 3. Verifica que DeleteDownload em um download ativo não emite evento com status "cancelled"
	var sawCancelled bool
	app.manager.OnUpdate(func(d models.Download) {
		if d.Status == models.StatusCancelled {
			sawCancelled = true
		}
	})

	blockCh := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "500000")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("DADOS-EM-STREAM"))
			<-blockCh
		}
	}))
	defer func() {
		close(blockCh)
		srv.Close()
	}()

	dl, err := app.AddDownload(models.AddDownloadRequest{
		URL:            srv.URL + "/teste_delete_ativo.bin",
		FileName:       "teste_delete_ativo.bin",
		SaveDir:        t.TempDir(),
		CategoryID:     "general",
		MaxConnections: 1,
		StartNow:       true,
	})
	if err != nil {
		t.Fatalf("erro em AddDownload: %v", err)
	}

	time.Sleep(120 * time.Millisecond)
	if err := app.DeleteDownload(dl.ID, true); err != nil {
		t.Fatalf("erro em DeleteDownload: %v", err)
	}
	if sawCancelled {
		t.Fatalf("DeleteDownload não deveria emitir status intermediário 'cancelled' (Parado) ao excluir diretamente")
	}
}

// TestLanguageSupportAndPersistence valida normalização, persistência no SQLite, sincronização com a bandeja
// do sistema (SystemTray) e geração bilíngue (PT-BR / EN-US) da página de ativação da extensão na v1.0.1.
func TestLanguageSupportAndPersistence(t *testing.T) {
	// 1. Valida normalização de códigos de idioma (casos de borda, vazios e variantes)
	cases := map[string]string{
		"":       models.LangPtBR,
		"   ":    models.LangPtBR,
		"pt":     models.LangPtBR,
		"pt-BR":  models.LangPtBR,
		"PT_br":  models.LangPtBR,
		"en":     models.LangEnUS,
		"en-US":  models.LangEnUS,
		"EN-gb":  models.LangEnUS,
		"es-ES":  models.LangPtBR,
		"fr":     models.LangPtBR,
	}
	for input, want := range cases {
		if got := models.NormalizeLanguage(input); got != want {
			t.Fatalf("NormalizeLanguage(%q) = %q, esperava %q", input, got, want)
		}
	}

	// 2. Valida idioma padrão "pt-BR" em novo banco SQLite e troca dinâmica via SetLanguage("en-US")
	dbPath := filepath.Join(t.TempDir(), "test_lang.sqlite")
	app, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao criar App: %v", err)
	}
	defer app.Shutdown(context.Background())

	tray := integration.NewSystemTray(integration.TrayCallbacks{})
	app.tray = tray

	if got := app.GetLanguage(); got != models.LangPtBR {
		t.Fatalf("esperava idioma inicial %q, obteve %q", models.LangPtBR, got)
	}

	updatedLang, err := app.SetLanguage("en")
	if err != nil {
		t.Fatalf("erro em SetLanguage('en'): %v", err)
	}
	if updatedLang != models.LangEnUS || app.GetLanguage() != models.LangEnUS {
		t.Fatalf("esperava idioma atualizado para %q, obteve %q", models.LangEnUS, updatedLang)
	}
	if tray.GetLanguage() != models.LangEnUS {
		t.Fatalf("esperava SystemTray sincronizado para %q, obteve %q", models.LangEnUS, tray.GetLanguage())
	}

	// 3. Reabre o banco para confirmar persistência entre sessões
	app.Shutdown(context.Background())
	app2, err := NewApp(dbPath, nil)
	if err != nil {
		t.Fatalf("erro ao reabrir App: %v", err)
	}
	defer app2.Shutdown(context.Background())

	if got := app2.GetLanguage(); got != models.LangEnUS {
		t.Fatalf("esperava que idioma %q persistisse após reabrir banco, obteve %q", models.LangEnUS, got)
	}

	// 4. Verifica localização automática das categorias padrão no CapturePrompt quando o idioma é en-US
	promptZip := app2.PromptCaptureURL("https://example.com/archive.zip", "", "", "manual")
	if promptZip.CategoryName != "Compressed" {
		t.Fatalf("esperava CategoryName='Compressed' em en-US, obteve %q", promptZip.CategoryName)
	}
	promptMp3 := app2.PromptCaptureURL("https://example.com/song.mp3", "", "", "manual")
	if promptMp3.CategoryName != "Music" {
		t.Fatalf("esperava CategoryName='Music' em en-US, obteve %q", promptMp3.CategoryName)
	}
	promptMp4 := app2.PromptCaptureURL("https://example.com/movie.mp4", "", "", "manual")
	if promptMp4.CategoryName != "Video" {
		t.Fatalf("esperava CategoryName='Video' em en-US, obteve %q", promptMp4.CategoryName)
	}
	promptBin := app2.PromptCaptureURL("https://example.com/file.unknown", "", "", "manual")
	if promptBin.CategoryName != "General / Other" {
		t.Fatalf("esperava CategoryName='General / Other' em en-US, obteve %q", promptBin.CategoryName)
	}
	if gotCustom := localizeDefaultCategoryName("compressed", "Meus Arquivos", true); gotCustom != "Meus Arquivos" {
		t.Fatalf("esperava preservar nome customizado 'Meus Arquivos', obteve %q", gotCustom)
	}

	// 5. Verifica que RunSystemInstaller respeita o idioma en-US nas mensagens de etapas concluídas
	tempInstall := filepath.Join(t.TempDir(), "install_en")
	instRes, err := app2.RunSystemInstaller(integration.InstallOptions{
		InstallDir:            tempInstall,
		Language:              "en-US",
		StartWithWindows:      false,
		CreateDesktopShortcut: false,
		CreateStartMenu:       false,
		RegisterUninstaller:   true,
		InstallBrowserExt:     true,
	})
	if err != nil {
		t.Fatalf("erro em RunSystemInstaller(en-US): %v", err)
	}
	joinedSteps := strings.Join(instRes.StepsCompleted, " | ")
	if !strings.Contains(joinedSteps, "Installation directory prepared") || !strings.Contains(joinedSteps, "Browser integration configured") {
		t.Fatalf("esperava etapas do instalador em inglês quando Language='en-US', obteve: %s", joinedSteps)
	}

	// 6. Verifica geração do guia HTML bilíngue da extensão
	extDir := t.TempDir()
	htmlPath, err := integration.WriteExtensionSetupHTML(extDir, filepath.Join(extDir, "browser-extension"), []string{"Google Chrome", "Microsoft Edge"})
	if err != nil || htmlPath == "" {
		t.Fatalf("erro em WriteExtensionSetupHTML: %v", err)
	}
}


