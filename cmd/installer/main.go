package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"openidm/internal/integration"
)

func main() {
	silentFlag := flag.Bool("silent", false, "Executa a instalação silenciosa completa sem abrir janela interativa")
	uninstallFlag := flag.Bool("uninstall", false, "Remove a instalação, atalhos, auto-start e Native Host do OpenIDM")
	genAssetsFlag := flag.Bool("gen-assets", false, "Gera openidm.ico e ícones PNG da extensão no diretório atual")
	installDirFlag := flag.String("install-dir", "", "Diretório de destino da instalação (padrão: %LOCALAPPDATA%\\Programs\\OpenIDM)")
	sourceDirFlag := flag.String("source-dir", "", "Diretório de origem dos binários")
	startWithWinFlag := flag.Bool("autostart", true, "Configura para iniciar automaticamente junto com o Windows")
	launchExtFlag := flag.Bool("launch-ext", false, "Inicia a instalação da extensão no navegador ao concluir")
	launchAppFlag := flag.Bool("launch-app", false, "Inicia o OpenIDM após concluir a instalação silenciosa")
	stopRunningFlag := flag.Bool("stop-running", false, "Encerra instâncias em execução do openidm.exe antes de copiar")
	flag.Parse()

	if *genAssetsFlag {
		wd, _ := os.Getwd()
		icoPath, err := integration.EnsureIconAssets(wd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao gerar ícones: %v\n", err)
			os.Exit(1)
		}
		// Também grava em build/windows/icon.ico (Wails e go-winres) e build/appicon.png
		if icoBytes, err := os.ReadFile(icoPath); err == nil {
			_ = os.MkdirAll(filepath.Join(wd, "build", "windows"), 0o755)
			_ = os.WriteFile(filepath.Join(wd, "build", "windows", "icon.ico"), icoBytes, 0o644)
		}
		if png256, err := integration.RenderOpenIDMIconPNG(256); err == nil {
			_ = os.WriteFile(filepath.Join(wd, "build", "appicon.png"), png256, 0o644)
		}
		_, _ = integration.WriteExtensionSetupHTML(wd, filepath.Join(wd, "browser-extension"), nil)
		fmt.Printf("Ícones gerados com sucesso: %s\n", icoPath)
		return
	}

	if *uninstallFlag {
		if err := integration.RunUninstall(*installDirFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao desinstalar OpenIDM: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("OpenIDM desinstalado com sucesso.")
		return
	}

	// Instalador de release: extrai os arquivos embutidos para uma pasta temporária e usa-a como origem.
	if *sourceDirFlag == "" && hasEmbeddedPayload() {
		payloadDir, err := extractEmbeddedPayload()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao extrair os arquivos do instalador: %v\n", err)
			os.Exit(1)
		}
		defer os.RemoveAll(payloadDir)
		*sourceDirFlag = payloadDir
	}

	if *silentFlag {
		res, err := integration.RunInstallation(integration.InstallOptions{
			InstallDir:            *installDirFlag,
			SourceDir:             *sourceDirFlag,
			StartWithWindows:      *startWithWinFlag,
			StartInTray:           true,
			CreateDesktopShortcut: true,
			CreateStartMenu:       true,
			RegisterUninstaller:   true,
			InstallBrowserExt:     true,
			LaunchBrowserExtSetup: *launchExtFlag,
			LaunchAppAfterInstall: *launchAppFlag,
			StopRunningInstances:  *stopRunningFlag,
		}, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Falha na instalação: %v\n", err)
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
		return
	}

	// Modo Interativo GUI: Inicia o Assistente Profissional de Instalação (Setup Wizard)
	runInteractiveSetupWizard(*installDirFlag, *sourceDirFlag)
}

func runInteractiveSetupWizard(initialInstallDir, sourceDir string) {
	exePath, _ := os.Executable()
	if sourceDir == "" && exePath != "" {
		sourceDir = filepath.Dir(exePath)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		// Fallback direto: abre openidm.exe --installer se disponível
		appExe := filepath.Join(sourceDir, "openidm.exe")
		if _, stErr := os.Stat(appExe); stErr == nil {
			_ = exec.Command(appExe, "--installer").Start()
			return
		}
		return
	}
	defer listener.Close()

	addr := listener.Addr().String()
	wizardURL := "http://" + addr + "/"

	var progressMu sync.Mutex
	curStep := 0
	curTotal := 7
	curMessage := "Pronto para instalar"
	doneCh := make(chan struct{})
	var closeOnce sync.Once

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(setupWizardHTML))
	})

	mux.HandleFunc("/api/icon.png", func(w http.ResponseWriter, r *http.Request) {
		if pngBytes, err := integration.RenderOpenIDMIconPNG(128); err == nil {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		st := integration.GetInstallationStatus(sourceDir)
		if initialInstallDir != "" {
			st.InstallDir = initialInstallDir
		} else if st.InstallDir == sourceDir {
			st.InstallDir = integration.DefaultInstallDir()
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(st)
	})

	mux.HandleFunc("/api/progress", func(w http.ResponseWriter, r *http.Request) {
		progressMu.Lock()
		resp := map[string]any{
			"step":    curStep,
			"total":   curTotal,
			"message": curMessage,
		}
		progressMu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/install", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var opts integration.InstallOptions
		_ = json.NewDecoder(r.Body).Decode(&opts)
		if opts.SourceDir == "" {
			opts.SourceDir = sourceDir
		}
		opts.StopRunningInstances = true

		res, err := integration.RunInstallation(opts, func(step, total int, message string) {
			progressMu.Lock()
			curStep = step
			curTotal = total
			curMessage = message
			progressMu.Unlock()
		})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(res)
	})

	mux.HandleFunc("/api/launch-extension", func(w http.ResponseWriter, r *http.Request) {
		st := integration.GetInstallationStatus(sourceDir)
		_ = integration.LaunchBrowserExtensionInstaller(st.ExtensionDir, st.ExtensionSetupHTML)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/open-ext-folder", func(w http.ResponseWriter, r *http.Request) {
		st := integration.GetInstallationStatus(sourceDir)
		_ = integration.WriteClipboardText(st.ExtensionDir)
		_ = exec.Command("explorer.exe", st.ExtensionDir).Start()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"extensionDir": st.ExtensionDir})
	})

	mux.HandleFunc("/api/close", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		go func() {
			time.Sleep(250 * time.Millisecond)
			closeOnce.Do(func() { close(doneCh) })
		}()
	})

	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(listener)
	}()

	// Abre janela dedicada em modo App (Edge ou Chrome) com perfil temporário isolado para respeitar tamanho 860x660
	browsers := integration.DetectInstalledBrowsers()
	var appCmd *exec.Cmd
	tempProfile, _ := os.MkdirTemp("", "openidm-setup-ui-*")
	defer func() {
		if tempProfile != "" {
			_ = os.RemoveAll(tempProfile)
		}
	}()

	for _, b := range browsers {
		if b.ID == "edge" || b.ID == "chrome" || b.ID == "brave" {
			args := []string{
				"--app=" + wizardURL,
				"--window-size=860,670",
				"--no-first-run",
				"--no-default-browser-check",
			}
			if tempProfile != "" {
				args = append(args, "--user-data-dir="+tempProfile)
			}
			cmd := exec.Command(b.ExePath, args...)
			if err := cmd.Start(); err == nil {
				appCmd = cmd
				break
			}
		}
	}

	if appCmd == nil {
		// Fallback para navegador padrão do sistema
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", wizardURL).Start()
		select {
		case <-doneCh:
		case <-time.After(10 * time.Minute):
		}
		return
	}

	go func() {
		_ = appCmd.Wait()
		closeOnce.Do(func() { close(doneCh) })
	}()

	<-doneCh
	_ = srv.Close()
}

const setupWizardHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8" />
  <meta name="theme-color" content="#141413" />
  <link rel="icon" type="image/png" href="/api/icon.png" />
  <title>Instalador Oficial — OpenIDM (v1.0.1)</title>
  <style>
    :root {
      --bg: #141413;
      --card: #1e1e1c;
      --elevated: #262522;
      --border: #32312d;
      --text: #faf9f5;
      --muted: #a39f94;
      --accent: #f4b601;
      --accent-hover: #ffc519;
      --success: #4ade80;
    }
    * { box-sizing: border-box; user-select: none; }
    body {
      margin: 0;
      padding: 24px 28px;
      background: radial-gradient(circle at top right, #262115 0%, var(--bg) 60%);
      color: var(--text);
      font-family: "Inter", "Segoe UI", system-ui, sans-serif;
      height: 100vh;
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      overflow-y: auto;
    }
    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding-bottom: 18px;
      border-bottom: 1px solid var(--border);
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 16px;
    }
    .logo {
      width: 54px;
      height: 54px;
      border-radius: 14px;
      overflow: hidden;
      display: flex;
      align-items: center;
      justify-content: center;
      box-shadow: 0 8px 22px rgba(244,182,1,0.35);
    }
    .logo img {
      width: 100%;
      height: 100%;
      object-fit: cover;
    }
    .brand h1 {
      margin: 0 0 4px 0;
      font-size: 20px;
      font-weight: 700;
    }
    .brand p {
      margin: 0;
      font-size: 12.5px;
      color: var(--muted);
    }
    .badge {
      padding: 6px 12px;
      border-radius: 999px;
      background: var(--card);
      border: 1px solid var(--border);
      font-size: 11.5px;
      color: var(--accent);
      font-weight: 600;
    }
    .panel {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 18px 20px;
      margin-top: 16px;
    }
    .panel h3 {
      margin: 0 0 12px 0;
      font-size: 14px;
      color: var(--text);
    }
    .field-label {
      font-size: 12px;
      color: var(--muted);
      margin-bottom: 6px;
      display: block;
    }
    input[type="text"] {
      width: 100%;
      padding: 10px 12px;
      border-radius: 9px;
      border: 1px solid var(--border);
      background: #121210;
      color: var(--text);
      font-family: "Cascadia Code", Consolas, monospace;
      font-size: 12.5px;
      outline: none;
      user-select: text;
    }
    input[type="text"]:focus { border-color: var(--accent); }
    .check-item {
      display: flex;
      align-items: flex-start;
      gap: 12px;
      padding: 9px 0;
      font-size: 13px;
      cursor: pointer;
      line-height: 1.4;
    }
    .check-item input[type="checkbox"] {
      width: 17px;
      height: 17px;
      margin: 2px 0 0 0;
      accent-color: var(--accent);
      flex-shrink: 0;
      cursor: pointer;
    }
    .check-item .check-text {
      display: flex;
      flex-direction: column;
      gap: 2px;
    }
    .check-sub {
      display: block;
      font-size: 12px;
      color: var(--muted);
      line-height: 1.35;
    }
    .progress-box {
      display: none;
      background: #181816;
      border: 1px solid rgba(244,182,1,0.45);
      border-radius: 12px;
      padding: 16px 18px;
      margin-top: 14px;
    }
    .progress-bar-bg {
      width: 100%;
      height: 9px;
      background: var(--elevated);
      border-radius: 999px;
      overflow: hidden;
      margin: 8px 0 10px 0;
    }
    .progress-bar-fill {
      width: 0%;
      height: 100%;
      background: linear-gradient(90deg, var(--accent), var(--success));
      transition: width 0.25s ease;
    }
    .steps-list {
      font-size: 12px;
      color: #d6d3c9;
      line-height: 1.6;
      max-height: 130px;
      overflow-y: auto;
    }
    .footer {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding-top: 16px;
      border-top: 1px solid var(--border);
      margin-top: 16px;
    }
    .btn {
      padding: 11px 20px;
      border-radius: 10px;
      border: 1px solid rgba(244,182,1,0.5);
      background: var(--accent);
      color: #181818;
      font-weight: 700;
      font-size: 13px;
      cursor: pointer;
      transition: all 0.15s ease;
    }
    .btn:hover:not(:disabled) { background: var(--accent-hover); }
    .btn:disabled { opacity: 0.6; cursor: wait; }
    .btn.secondary {
      background: var(--elevated);
      border-color: var(--border);
      color: var(--text);
    }
    .btn.secondary:hover { background: #302f2b; }
  </style>
</head>
<body>
  <div>
    <div class="header">
      <div class="brand">
        <div class="logo">
          <img src="/api/icon.png" alt="OpenIDM" />
        </div>
        <div>
          <h1 id="headerTitle">Instalador Oficial — OpenIDM</h1>
          <p id="headerSub">Gerenciador de Downloads de Alta Velocidade para Windows</p>
        </div>
      </div>
      <div style="display:flex; align-items:center; gap:8px;">
        <button class="btn secondary" id="langBtn" onclick="toggleLang()" style="padding:5px 10px; font-size:11.5px; border-radius:999px;">🌐 PT / EN</button>
        <span class="badge">v1.0.1</span>
      </div>
    </div>

    <div class="panel">
      <label class="field-label" id="dirLabel">Pasta de Destino da Instalação no Windows:</label>
      <input type="text" id="installDir" value="" />
    </div>

    <div class="panel">
      <h3 id="optionsTitle">Opções de Instalação</h3>

      <label class="check-item">
        <input type="checkbox" id="chkAutostart" checked />
        <span class="check-text">
          <b id="optAutoTitle">Iniciar o OpenIDM automaticamente com o Windows</b>
          <span class="check-sub" id="optAutoSub">Mantém o aplicativo ativo em segundo plano perto do relógio</span>
        </span>
      </label>

      <label class="check-item">
        <input type="checkbox" id="chkExt" checked />
        <span class="check-text">
          <b id="optExtTitle">Integrar ao navegador (Chrome, Edge, Brave e Firefox)</b>
          <span class="check-sub" id="optExtSub">Captura downloads automaticamente ao clicar em links no navegador</span>
        </span>
      </label>

      <label class="check-item">
        <input type="checkbox" id="chkShortcuts" checked />
        <span class="check-text">
          <b id="optShortTitle">Criar atalhos na Área de Trabalho e no Menu Iniciar</b>
          <span class="check-sub" id="optShortSub">Facilita o acesso rápido ao aplicativo e ao desinstalador</span>
        </span>
      </label>

      <label class="check-item">
        <input type="checkbox" id="chkLaunchApp" checked />
        <span class="check-text">
          <b id="optLaunchTitle">Abrir o OpenIDM após concluir a instalação</b>
        </span>
      </label>
    </div>

    <div class="progress-box" id="progressBox">
      <div style="display:flex; justify-content:space-between; font-size:12.5px; font-weight:600;">
        <span id="progressText">Instalando...</span>
        <span id="progressPct" style="color:var(--accent);">0%</span>
      </div>
      <div class="progress-bar-bg">
        <div class="progress-bar-fill" id="progressFill"></div>
      </div>
      <div class="steps-list" id="stepsList"></div>
    </div>
  </div>

  <div class="footer">
    <button class="btn secondary" id="cancelBtn" onclick="closeSetup()">Cancelar / Fechar</button>
    <div style="display:flex; gap:10px;">
      <button class="btn secondary" id="extBtn" style="display:none;" onclick="openExtSetup()">🌐 Abrir Guia da Extensão</button>
      <button class="btn" id="installBtn" onclick="startInstall()">Instalar OpenIDM</button>
    </div>
  </div>

  <script>
    let currentLang = /^en/i.test(navigator.language || "") ? "en-US" : "pt-BR";
    let installState = "idle"; // "idle" | "installing" | "done" | "error"

    const I18N = {
      "pt-BR": {
        docTitle: "Instalador Oficial — OpenIDM (v1.0.1)",
        headerTitle: "Instalador Oficial — OpenIDM",
        headerSub: "Gerenciador de Downloads de Alta Velocidade para Windows",
        dirLabel: "Pasta de Destino da Instalação no Windows:",
        optionsTitle: "Opções de Instalação",
        optAutoTitle: "Iniciar o OpenIDM automaticamente com o Windows",
        optAutoSub: "Mantém o aplicativo ativo em segundo plano perto do relógio",
        optExtTitle: "Integrar ao navegador (Chrome, Edge, Brave e Firefox)",
        optExtSub: "Captura downloads automaticamente ao clicar em links no navegador",
        optShortTitle: "Criar atalhos na Área de Trabalho e no Menu Iniciar",
        optShortSub: "Facilita o acesso rápido ao aplicativo e ao desinstalador",
        optLaunchTitle: "Abrir o OpenIDM após concluir a instalação",
        cancelBtn: "Cancelar / Fechar",
        extBtn: "🌐 Abrir Guia da Extensão",
        installBtn: "Instalar OpenIDM",
        installingBtn: "⏳ Instalando...",
        doneBtn: "✔ Concluído",
        retryBtn: "Tentar Novamente",
        configuringMsg: "Configurando arquivos e registro do Windows...",
        successMsg: "✔ OpenIDM instalado com sucesso!",
        errorPrefix: "Erro: ",
        failPrefix: "Falha: "
      },
      "en-US": {
        docTitle: "Official Installer — OpenIDM (v1.0.1)",
        headerTitle: "Official Installer — OpenIDM",
        headerSub: "High-Speed Multi-Segment Download Manager for Windows",
        dirLabel: "Windows Installation Destination Folder:",
        optionsTitle: "Installation Options",
        optAutoTitle: "Launch OpenIDM automatically with Windows",
        optAutoSub: "Keeps the app running in the background in the system tray",
        optExtTitle: "Integrate with browsers (Chrome, Edge, Brave & Firefox)",
        optExtSub: "Automatically intercepts downloads when clicking links in the browser",
        optShortTitle: "Create Desktop and Start Menu shortcuts",
        optShortSub: "Provides quick access to the application and uninstaller",
        optLaunchTitle: "Launch OpenIDM after installation completes",
        cancelBtn: "Cancel / Close",
        extBtn: "🌐 Open Extension Guide",
        installBtn: "Install OpenIDM",
        installingBtn: "⏳ Installing...",
        doneBtn: "✔ Completed",
        retryBtn: "Try Again",
        configuringMsg: "Configuring files and Windows registry...",
        successMsg: "✔ OpenIDM installed successfully!",
        errorPrefix: "Error: ",
        failPrefix: "Failure: "
      }
    };

    function applyLang(lang) {
      currentLang = lang === "en-US" ? "en-US" : "pt-BR";
      const t = I18N[currentLang];
      document.documentElement.lang = currentLang;
      document.title = t.docTitle;
      document.getElementById("headerTitle").textContent = t.headerTitle;
      document.getElementById("headerSub").textContent = t.headerSub;
      document.getElementById("dirLabel").textContent = t.dirLabel;
      document.getElementById("optionsTitle").textContent = t.optionsTitle;
      document.getElementById("optAutoTitle").textContent = t.optAutoTitle;
      document.getElementById("optAutoSub").textContent = t.optAutoSub;
      document.getElementById("optExtTitle").textContent = t.optExtTitle;
      document.getElementById("optExtSub").textContent = t.optExtSub;
      document.getElementById("optShortTitle").textContent = t.optShortTitle;
      document.getElementById("optShortSub").textContent = t.optShortSub;
      document.getElementById("optLaunchTitle").textContent = t.optLaunchTitle;
      document.getElementById("cancelBtn").textContent = t.cancelBtn;
      document.getElementById("extBtn").textContent = t.extBtn;
      document.getElementById("langBtn").textContent = currentLang === "en-US" ? "🌐 EN (PT)" : "🌐 PT (EN)";
      const installBtn = document.getElementById("installBtn");
      if (installState === "idle") installBtn.textContent = t.installBtn;
      else if (installState === "installing") installBtn.textContent = t.installingBtn;
      else if (installState === "done") installBtn.textContent = t.doneBtn;
      else if (installState === "error") installBtn.textContent = t.retryBtn;
    }

    function toggleLang() {
      applyLang(currentLang === "en-US" ? "pt-BR" : "en-US");
    }

    applyLang(currentLang);

    fetch("/api/status")
      .then(r => r.json())
      .then(st => {
        if (st && st.installDir) {
          document.getElementById("installDir").value = st.installDir;
        }
      });

    function startInstall() {
      const t = I18N[currentLang];
      const btn = document.getElementById("installBtn");
      const box = document.getElementById("progressBox");
      const fill = document.getElementById("progressFill");
      const txt = document.getElementById("progressText");
      const pct = document.getElementById("progressPct");
      const list = document.getElementById("stepsList");

      installState = "installing";
      btn.disabled = true;
      btn.textContent = t.installingBtn;
      box.style.display = "block";
      fill.style.width = "15%";
      txt.textContent = t.configuringMsg;
      pct.textContent = "15%";
      list.innerHTML = "";

      const poll = setInterval(() => {
        fetch("/api/progress")
          .then(r => r.json())
          .then(p => {
            if (p && p.step) {
              const pr = Math.min(95, Math.round((p.step / (p.total || 7)) * 100));
              fill.style.width = pr + "%";
              pct.textContent = pr + "%";
              txt.textContent = p.message || t.installingBtn;
            }
          })
          .catch(() => {});
      }, 250);

      const shortcuts = document.getElementById("chkShortcuts").checked;
      const ext = document.getElementById("chkExt").checked;

      fetch("/api/install", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          installDir: document.getElementById("installDir").value.trim(),
          language: currentLang,
          startWithWindows: document.getElementById("chkAutostart").checked,
          startInTray: true,
          createDesktopShortcut: shortcuts,
          createStartMenu: shortcuts,
          registerUninstaller: shortcuts,
          installBrowserExt: ext,
          launchBrowserExtSetup: ext,
          launchAppAfterInstall: document.getElementById("chkLaunchApp").checked
        })
      })
        .then(r => r.json())
        .then(res => {
          clearInterval(poll);
          const curT = I18N[currentLang];
          if (res.error) {
            installState = "error";
            txt.textContent = curT.errorPrefix + res.error;
            btn.disabled = false;
            btn.textContent = curT.retryBtn;
            return;
          }
          installState = "done";
          fill.style.width = "100%";
          pct.textContent = "100%";
          txt.textContent = curT.successMsg;
          btn.textContent = curT.doneBtn;
          document.getElementById("extBtn").style.display = "inline-block";
          if (res.stepsCompleted) {
            list.innerHTML = res.stepsCompleted.map(s => "<div>✔ " + s + "</div>").join("");
          }
        })
        .catch(err => {
          clearInterval(poll);
          installState = "error";
          txt.textContent = I18N[currentLang].failPrefix + err;
          btn.disabled = false;
          btn.textContent = I18N[currentLang].retryBtn;
        });
    }

    function openExtSetup() {
      fetch("/api/launch-extension", { method: "POST" });
    }

    function closeSetup() {
      fetch("/api/close", { method: "POST" }).finally(() => window.close());
    }
  </script>
</body>
</html>`
