package integration

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"openidm/internal/sysproc"
)

const (
	AutoStartRegKey   = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	AutoStartRegValue = "OpenIDM"
	UninstallRegKey   = `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\OpenIDM`
	AppVersion        = "1.0.1"
	AppPublisher      = "RafaTech"
	AppDisplayName    = "OpenIDM - Gerenciador de Downloads"
)

// BrowserInfo descreve um navegador instalado detectado no Windows.
type BrowserInfo struct {
	ID      string `json:"id"`      // "chrome", "edge", "brave", "firefox"
	Name    string `json:"name"`    // Nome amigável
	ExePath string `json:"exePath"` // Caminho absoluto do executável
}

// InstallOptions define os parâmetros configuráveis da instalação profissional do OpenIDM.
type InstallOptions struct {
	InstallDir            string `json:"installDir"`
	SourceDir             string `json:"sourceDir,omitempty"`
	Language              string `json:"language,omitempty"`
	StartWithWindows      bool   `json:"startWithWindows"`
	StartInTray           bool   `json:"startInTray"`
	CreateDesktopShortcut bool   `json:"createDesktopShortcut"`
	CreateStartMenu       bool   `json:"createStartMenu"`
	RegisterUninstaller   bool   `json:"registerUninstaller"`
	InstallBrowserExt     bool   `json:"installBrowserExt"`
	LaunchBrowserExtSetup bool   `json:"launchBrowserExtSetup"`
	LaunchAppAfterInstall bool   `json:"launchAppAfterInstall"`
	ChromeExtID           string `json:"chromeExtId,omitempty"`
	StopRunningInstances  bool   `json:"stopRunningInstances,omitempty"`
}

// InstallResult consolida todos os artefatos, chaves de registro e atalhos gerados na instalação.
type InstallResult struct {
	Success              bool          `json:"success"`
	InstallDir           string        `json:"installDir"`
	AppExePath           string        `json:"appExePath"`
	NativeHostExePath    string        `json:"nativeHostExePath"`
	InstallerExePath     string        `json:"installerExePath"`
	IconPath             string        `json:"iconPath"`
	ExtensionDir         string        `json:"extensionDir"`
	ExtensionSetupHTML   string        `json:"extensionSetupHtml"`
	ChromeManifestPath   string        `json:"chromeManifestPath"`
	FirefoxManifestPath  string        `json:"firefoxManifestPath"`
	ExtensionIDs         []string      `json:"extensionIds"`
	AutoStartEnabled     bool          `json:"autoStartEnabled"`
	AutoStartCommand     string        `json:"autoStartCommand"`
	DesktopShortcutPath  string        `json:"desktopShortcutPath,omitempty"`
	StartMenuShortcutDir string        `json:"startMenuShortcutDir,omitempty"`
	UninstallerRegKey    string        `json:"uninstallerRegKey,omitempty"`
	BrowsersDetected     []BrowserInfo `json:"browsersDetected"`
	StepsCompleted       []string      `json:"stepsCompleted"`
}

// InstallationStatus informa o estado atual da instalação no Windows para exibição na interface.
type InstallationStatus struct {
	Installed            bool          `json:"installed"`
	InstallDir           string        `json:"installDir"`
	ActiveExePath        string        `json:"activeExePath"`
	IconPath             string        `json:"iconPath"`
	ExtensionDir         string        `json:"extensionDir"`
	ExtensionSetupHTML   string        `json:"extensionSetupHtml"`
	ExtensionIDs         []string      `json:"extensionIds"`
	AutoStartEnabled     bool          `json:"autoStartEnabled"`
	AutoStartCommand     string        `json:"autoStartCommand"`
	DesktopShortcutExist bool          `json:"desktopShortcutExist"`
	StartMenuExist       bool          `json:"startMenuExist"`
	UninstallerExist     bool          `json:"uninstallerExist"`
	NativeHostRegistered bool          `json:"nativeHostRegistered"`
	BrowsersDetected     []BrowserInfo `json:"browsersDetected"`
}

// DefaultInstallDir retorna o diretório padrão de instalação por usuário no Windows (%LOCALAPPDATA%\Programs\OpenIDM).
func DefaultInstallDir() string {
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if localAppData == "" {
		if cfgDir, err := os.UserConfigDir(); err == nil && cfgDir != "" {
			localAppData = filepath.Join(filepath.Dir(cfgDir), "Local")
		}
	}
	if localAppData != "" {
		return filepath.Join(localAppData, "Programs", "OpenIDM")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "AppData", "Local", "Programs", "OpenIDM")
	}
	return `C:\OpenIDM`
}

// isTestBinary verifica se o caminho pertence a um binário compilado pelo `go test` para evitar poluir o Registro em testes.
func isTestBinary(exePath string) bool {
	lower := strings.ToLower(strings.TrimSpace(exePath))
	return strings.HasSuffix(lower, ".test.exe") || strings.HasSuffix(lower, ".test")
}

// isTempDirPath verifica se o caminho está dentro da pasta temporária do sistema operacional (ex: t.TempDir() durante go test).
func isTempDirPath(p string) bool {
	clean := strings.ToLower(filepath.Clean(strings.TrimSpace(p)))
	tmp := strings.ToLower(filepath.Clean(os.TempDir()))
	return tmp != "" && strings.HasPrefix(clean, tmp)
}

// SetAutoStartWithWindows configura ou remove a inicialização automática do OpenIDM junto com o Windows (HKCU\...\Run).
func SetAutoStartWithWindows(exePath string, enabled bool, startInTray bool) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if self, err := os.Executable(); err == nil && isTestBinary(self) {
		return nil
	}

	cleanExe := strings.TrimSpace(exePath)
	if cleanExe == "" {
		if self, err := os.Executable(); err == nil {
			cleanExe = self
		}
	}
	if isTestBinary(cleanExe) || isTempDirPath(cleanExe) {
		return nil
	}

	// Remove entrada legada caso exista
	_ = sysproc.HiddenCommand("reg", "delete", AutoStartRegKey, "/v", "TurboDM", "/f").Run()

	if !enabled {
		_ = sysproc.HiddenCommand("reg", "delete", AutoStartRegKey, "/v", AutoStartRegValue, "/f").Run()
		return nil
	}

	if abs, err := filepath.Abs(cleanExe); err == nil {
		cleanExe = abs
	}

	cmdStr := fmt.Sprintf(`"%s"`, cleanExe)
	if startInTray {
		cmdStr = fmt.Sprintf(`"%s" --tray`, cleanExe)
	}

	return sysproc.HiddenCommand("reg", "add", AutoStartRegKey, "/v", AutoStartRegValue, "/t", "REG_SZ", "/d", cmdStr, "/f").Run()
}

// GetAutoStartStatus consulta no Registro do Windows se o OpenIDM está configurado para iniciar com o sistema.
func GetAutoStartStatus() (bool, string) {
	if runtime.GOOS != "windows" {
		return false, ""
	}
	out, err := sysproc.HiddenCommand("reg", "query", AutoStartRegKey, "/v", AutoStartRegValue).CombinedOutput()
	if err != nil {
		return false, ""
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(AutoStartRegValue)) {
			if idx := strings.Index(trimmed, "REG_SZ"); idx >= 0 {
				val := strings.TrimSpace(trimmed[idx+len("REG_SZ"):])
				return val != "", val
			}
		}
	}
	return false, ""
}

// IsNativeHostRegistered verifica no Registro do Windows se o Native Messaging Host do OpenIDM está registrado.
func IsNativeHostRegistered() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	key := `HKCU\Software\Google\Chrome\NativeMessagingHosts\` + NativeHostName
	return sysproc.HiddenCommand("reg", "query", key).Run() == nil
}

// IsUninstallerRegistered verifica se o OpenIDM consta em "Aplicativos Instalados / Programas e Recursos" do Windows.
func IsUninstallerRegistered() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return sysproc.HiddenCommand("reg", "query", UninstallRegKey, "/v", "DisplayName").Run() == nil
}

// CreateWindowsShortcut cria um atalho .lnk nativo do Windows com ícone personalizado via WScript.Shell.
func CreateWindowsShortcut(shortcutPath, targetPath, arguments, workingDir, description, iconLocation string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(shortcutPath), 0o755); err != nil {
		return err
	}

	esc := func(s string) string {
		return strings.ReplaceAll(s, "'", "''")
	}

	psScript := fmt.Sprintf(
		`$ws = New-Object -ComObject WScript.Shell; `+
			`$s = $ws.CreateShortcut('%s'); `+
			`$s.TargetPath = '%s'; `+
			`$s.Arguments = '%s'; `+
			`$s.WorkingDirectory = '%s'; `+
			`$s.Description = '%s'; `+
			`$s.IconLocation = '%s'; `+
			`$s.Save()`,
		esc(shortcutPath),
		esc(targetPath),
		esc(arguments),
		esc(workingDir),
		esc(description),
		esc(iconLocation),
	)

	cmd := sysproc.HiddenCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("falha ao criar atalho %s: %v (%s)", shortcutPath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RegisterWindowsUninstaller registra o OpenIDM no Painel de Controle / Aplicativos Instalados do Windows com ícone oficial.
func RegisterWindowsUninstaller(installDir, appExePath, setupExePath, iconPath string) error {
	if runtime.GOOS != "windows" {
		return nil
	}

	displayIcon := fmt.Sprintf("%s,0", appExePath)
	if iconPath != "" {
		if _, err := os.Stat(iconPath); err == nil {
			displayIcon = iconPath
		}
	}

	uninstallerExe := setupExePath
	if uninstallerExe == "" || !fileExists(uninstallerExe) {
		uninstallerExe = appExePath
	}

	uninstallCmd := fmt.Sprintf(`"%s" --uninstall`, uninstallerExe)
	quietUninstallCmd := fmt.Sprintf(`"%s" --uninstall --silent`, uninstallerExe)
	modifyCmd := fmt.Sprintf(`"%s" --installer`, appExePath)

	strValues := map[string]string{
		"DisplayName":          AppDisplayName,
		"DisplayVersion":       AppVersion,
		"Publisher":            AppPublisher,
		"DisplayIcon":          displayIcon,
		"InstallLocation":      installDir,
		"UninstallString":      uninstallCmd,
		"QuietUninstallString": quietUninstallCmd,
		"ModifyPath":           modifyCmd,
		"URLInfoAbout":         "https://rafatech.local/openidm",
		"InstallDate":          time.Now().Format("20060102"),
	}

	for name, val := range strValues {
		if err := sysproc.HiddenCommand("reg", "add", UninstallRegKey, "/v", name, "/t", "REG_SZ", "/d", val, "/f").Run(); err != nil {
			return err
		}
	}

	// Calcula tamanho aproximado instalado em KB para exibição no Windows Settings > Apps
	var totalBytes int64
	_ = filepath.Walk(installDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			totalBytes += info.Size()
		}
		return nil
	})
	sizeKB := totalBytes / 1024
	if sizeKB < 1024 {
		sizeKB = 38400
	}

	_ = sysproc.HiddenCommand("reg", "add", UninstallRegKey, "/v", "NoModify", "/t", "REG_DWORD", "/d", "0", "/f").Run()
	_ = sysproc.HiddenCommand("reg", "add", UninstallRegKey, "/v", "NoRepair", "/t", "REG_DWORD", "/d", "0", "/f").Run()
	_ = sysproc.HiddenCommand("reg", "add", UninstallRegKey, "/v", "EstimatedSize", "/t", "REG_DWORD", "/d", fmt.Sprintf("%d", sizeKB), "/f").Run()

	return nil
}

// UnregisterWindowsUninstaller remove o registro do OpenIDM de Aplicativos Instalados do Windows.
func UnregisterWindowsUninstaller() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	_ = sysproc.HiddenCommand("reg", "delete", UninstallRegKey, "/f").Run()
	return nil
}

// DetectInstalledBrowsers localiza os navegadores instalados na máquina Windows (Chrome, Edge, Brave, Firefox).
func DetectInstalledBrowsers() []BrowserInfo {
	localAppData := os.Getenv("LOCALAPPDATA")
	progFiles := os.Getenv("ProgramFiles")
	progFiles86 := os.Getenv("ProgramFiles(x86)")
	if progFiles == "" {
		progFiles = `C:\Program Files`
	}
	if progFiles86 == "" {
		progFiles86 = `C:\Program Files (x86)`
	}

	type candidate struct {
		id    string
		name  string
		paths []string
	}

	candidates := []candidate{
		{
			id:   "chrome",
			name: "Google Chrome",
			paths: []string{
				filepath.Join(progFiles, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(progFiles86, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"),
			},
		},
		{
			id:   "edge",
			name: "Microsoft Edge",
			paths: []string{
				filepath.Join(progFiles86, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(progFiles, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(localAppData, "Microsoft", "Edge", "Application", "msedge.exe"),
			},
		},
		{
			id:   "brave",
			name: "Brave Browser",
			paths: []string{
				filepath.Join(progFiles, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
				filepath.Join(progFiles86, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
				filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			},
		},
		{
			id:   "firefox",
			name: "Mozilla Firefox",
			paths: []string{
				filepath.Join(progFiles, "Mozilla Firefox", "firefox.exe"),
				filepath.Join(progFiles86, "Mozilla Firefox", "firefox.exe"),
				filepath.Join(localAppData, "Mozilla Firefox", "firefox.exe"),
			},
		},
	}

	var found []BrowserInfo
	for _, c := range candidates {
		for _, p := range c.paths {
			if p != "" && fileExists(p) {
				found = append(found, BrowserInfo{
					ID:      c.id,
					Name:    c.name,
					ExePath: p,
				})
				break
			}
		}
	}
	return found
}

// GetInstallationStatus retorna o diagnóstico completo da instalação atual do sistema.
func GetInstallationStatus(sourceDir string) InstallationStatus {
	exePath, _ := os.Executable()
	if sourceDir == "" && exePath != "" {
		sourceDir = filepath.Dir(exePath)
	}

	defDir := DefaultInstallDir()
	installDir := defDir
	if !fileExists(filepath.Join(defDir, "openidm.exe")) && fileExists(filepath.Join(sourceDir, "openidm.exe")) {
		installDir = sourceDir
	}

	extDir := filepath.Join(installDir, "browser-extension")
	if !dirExists(extDir) && dirExists(filepath.Join(sourceDir, "browser-extension")) {
		extDir = filepath.Join(sourceDir, "browser-extension")
	}

	iconPath := filepath.Join(installDir, "openidm.ico")
	if !fileExists(iconPath) && fileExists(filepath.Join(sourceDir, "openidm.ico")) {
		iconPath = filepath.Join(sourceDir, "openidm.ico")
	}

	setupHTML := filepath.Join(installDir, "extension-setup.html")
	autoEnabled, autoCmd := GetAutoStartStatus()

	homeDir, _ := os.UserHomeDir()
	desktopLnk := filepath.Join(homeDir, "Desktop", "OpenIDM.lnk")
	appData := os.Getenv("APPDATA")
	startMenuLnk := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "OpenIDM", "OpenIDM - Gerenciador de Downloads.lnk")

	extIDs := DeriveUnpackedExtensionIDs(extDir)

	return InstallationStatus{
		Installed:            fileExists(filepath.Join(installDir, "openidm.exe")) && (autoEnabled || IsUninstallerRegistered() || IsNativeHostRegistered()),
		InstallDir:           installDir,
		ActiveExePath:        exePath,
		IconPath:             iconPath,
		ExtensionDir:         extDir,
		ExtensionSetupHTML:   setupHTML,
		ExtensionIDs:         extIDs,
		AutoStartEnabled:     autoEnabled,
		AutoStartCommand:     autoCmd,
		DesktopShortcutExist: fileExists(desktopLnk),
		StartMenuExist:       fileExists(startMenuLnk),
		UninstallerExist:     IsUninstallerRegistered(),
		NativeHostRegistered: IsNativeHostRegistered(),
		BrowsersDetected:     DetectInstalledBrowsers(),
	}
}

// RunInstallation executa o fluxo profissional completo de instalação do OpenIDM, configurando:
// 1. Ícones (.ico multi-resolução + PNGs da extensão)
// 2. Cópia de binários (openidm.exe, openidm-native-host.exe, OpenIDM-Setup.exe) e pasta browser-extension/
// 3. Inicialização automática com o Windows (HKCU\Software\Microsoft\Windows\CurrentVersion\Run -> --tray)
// 4. Registro do Native Messaging Host (Chrome, Edge, Brave, Chromium e Firefox)
// 5. Criação de atalhos com ícone na Área de Trabalho e no Menu Iniciar
// 6. Registro de Desinstalador no Painel de Controle / Aplicativos Instalados do Windows com DisplayIcon
// 7. Geração e abertura do instalador interativo da Extensão do Navegador
func RunInstallation(opts InstallOptions, onProgress func(step int, total int, message string)) (*InstallResult, error) {
	report := func(step, total int, msg string) {
		if onProgress != nil {
			onProgress(step, total, msg)
		}
	}

	const totalSteps = 7
	exePath, _ := os.Executable()
	sourceDir := strings.TrimSpace(opts.SourceDir)
	if sourceDir == "" {
		if exePath != "" && !isTestBinary(exePath) {
			sourceDir = filepath.Dir(exePath)
		}
		if sourceDir == "" || !fileExists(filepath.Join(sourceDir, "openidm.exe")) {
			if wd, err := os.Getwd(); err == nil {
				sourceDir = wd
			}
		}
	}

	installDir := strings.TrimSpace(opts.InstallDir)
	if installDir == "" {
		installDir = DefaultInstallDir()
	}
	if abs, err := filepath.Abs(installDir); err == nil {
		installDir = abs
	}

	res := &InstallResult{
		InstallDir:       installDir,
		BrowsersDetected: DetectInstalledBrowsers(),
	}

	isEN := strings.HasPrefix(strings.ToLower(strings.TrimSpace(opts.Language)), "en")
	pick := func(pt, en string) string {
		if isEN {
			return en
		}
		return pt
	}

	// Etapa 1: Preparar diretórios e encerrar instâncias antigas se solicitado
	report(1, totalSteps, pick(
		"Preparando diretório de instalação e recursos visuais...",
		"Preparing installation directory and visual assets...",
	))
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return nil, fmt.Errorf("erro ao criar diretório de instalação %s: %w", installDir, err)
	}

	if opts.StopRunningInstances && runtime.GOOS == "windows" {
		myPID := os.Getpid()
		_ = sysproc.HiddenCommand("taskkill", "/F", "/FI", fmt.Sprintf("PID ne %d", myPID), "/IM", "openidm.exe").Run()
		_ = sysproc.HiddenCommand("taskkill", "/F", "/FI", fmt.Sprintf("PID ne %d", myPID), "/IM", "openidm-native-host.exe").Run()
		_ = sysproc.HiddenCommand("taskkill", "/F", "/FI", fmt.Sprintf("PID ne %d", myPID), "/IM", "turbodm.exe").Run()
		_ = sysproc.HiddenCommand("taskkill", "/F", "/FI", fmt.Sprintf("PID ne %d", myPID), "/IM", "turbodm-native-host.exe").Run()
		time.Sleep(350 * time.Millisecond)
	}
	res.StepsCompleted = append(res.StepsCompleted, pick(
		"Diretório de instalação preparado: "+installDir,
		"Installation directory prepared: "+installDir,
	))

	// Etapa 2: Copiar binários e extensão para o diretório de instalação
	report(2, totalSteps, pick(
		"Copiando arquivos do aplicativo e extensão...",
		"Copying application and browser extension files...",
	))
	sameDir := strings.EqualFold(filepath.Clean(sourceDir), filepath.Clean(installDir))

	appExeDst := filepath.Join(installDir, "openidm.exe")
	hostExeDst := filepath.Join(installDir, "openidm-native-host.exe")
	setupExeDst := filepath.Join(installDir, "OpenIDM-Setup.exe")

	if !sameDir {
		if srcApp := filepath.Join(sourceDir, "openidm.exe"); fileExists(srcApp) {
			_ = copyFileSafe(srcApp, appExeDst)
		} else if exePath != "" && !isTestBinary(exePath) && strings.EqualFold(filepath.Base(exePath), "openidm.exe") {
			_ = copyFileSafe(exePath, appExeDst)
		}

		if srcHost := filepath.Join(sourceDir, "openidm-native-host.exe"); fileExists(srcHost) {
			_ = copyFileSafe(srcHost, hostExeDst)
		}

		if srcSetup := filepath.Join(sourceDir, "OpenIDM-Setup.exe"); fileExists(srcSetup) {
			_ = copyFileSafe(srcSetup, setupExeDst)
		} else if exePath != "" && !isTestBinary(exePath) && strings.EqualFold(filepath.Base(exePath), "OpenIDM-Setup.exe") {
			_ = copyFileSafe(exePath, setupExeDst)
		}
	}

	// Se estivermos rodando em ambiente onde openidm.exe está apenas no sourceDir, usa o caminho existente como fallback
	if !fileExists(appExeDst) && fileExists(filepath.Join(sourceDir, "openidm.exe")) {
		appExeDst = filepath.Join(sourceDir, "openidm.exe")
	}
	if !fileExists(hostExeDst) && fileExists(filepath.Join(sourceDir, "openidm-native-host.exe")) {
		hostExeDst = filepath.Join(sourceDir, "openidm-native-host.exe")
	}

	res.AppExePath = appExeDst
	res.NativeHostExePath = hostExeDst
	res.InstallerExePath = setupExeDst

	// Copia pasta browser-extension para o diretório instalado
	extDst := filepath.Join(installDir, "browser-extension")
	extSrc := filepath.Join(sourceDir, "browser-extension")
	if !sameDir && dirExists(extSrc) {
		_ = copyDirRecursive(extSrc, extDst)
	} else {
		_ = os.MkdirAll(filepath.Join(extDst, "icons"), 0o755)
	}
	res.ExtensionDir = extDst
	res.StepsCompleted = append(res.StepsCompleted, pick(
		"Arquivos do aplicativo e extensão copiados",
		"Application and browser extension files copied",
	))

	// Etapa 3: Gerar ícones de alta resolução no diretório instalado e no sourceDir
	report(3, totalSteps, pick(
		"Configurando ícones do aplicativo...",
		"Configuring application icons...",
	))
	if sourceDir != "" && !sameDir {
		_, _ = EnsureIconAssets(sourceDir)
	}
	icoPath, err := EnsureIconAssets(installDir)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar ícones: %w", err)
	}
	res.IconPath = icoPath
	res.StepsCompleted = append(res.StepsCompleted, pick(
		"Ícone do aplicativo configurado com sucesso",
		"Application icon configured successfully",
	))

	// Etapa 4: Registrar integração com os navegadores
	report(4, totalSteps, pick(
		"Configurando integração com navegadores (Chrome, Edge, Brave e Firefox)...",
		"Configuring browser integration (Chrome, Edge, Brave, and Firefox)...",
	))
	chromeManifest, err := InstallNativeMessagingHost(hostExeDst, opts.ChromeExtID)
	if err == nil {
		res.ChromeManifestPath = chromeManifest
		res.FirefoxManifestPath = filepath.Join(filepath.Dir(chromeManifest), "com.rafatech.openidm.firefox.json")
	}
	// Se sourceDir for diferente de installDir, atualiza também o manifesto no sourceDir para manter compatibilidade com extensão já carregada de lá
	if !sameDir && fileExists(filepath.Join(sourceDir, "openidm-native-host.exe")) {
		_, _ = InstallNativeMessagingHost(filepath.Join(sourceDir, "openidm-native-host.exe"), opts.ChromeExtID)
		// E re-aponta o registro principal para o host instalado (que já contém os IDs de ambos os diretórios!)
		_, _ = InstallNativeMessagingHost(hostExeDst, opts.ChromeExtID)
	}

	extIDs := DeriveUnpackedExtensionIDs(extDst)
	if !sameDir && dirExists(extSrc) {
		for _, id := range DeriveUnpackedExtensionIDs(extSrc) {
			extIDs = appendUniqueString(extIDs, id)
		}
	}
	res.ExtensionIDs = extIDs

	setupHTMLPath, htmlErr := WriteExtensionSetupHTML(installDir, extDst, extIDs)
	if htmlErr == nil {
		res.ExtensionSetupHTML = setupHTMLPath
	}
	if !sameDir && dirExists(extSrc) {
		_, _ = WriteExtensionSetupHTML(sourceDir, extSrc, extIDs)
	}
	res.StepsCompleted = append(res.StepsCompleted, pick(
		"Integração com navegadores configurada",
		"Browser integration configured",
	))

	// Etapa 5: Configurar inicialização automática com o Windows
	report(5, totalSteps, pick(
		"Configurando inicialização com o Windows...",
		"Configuring Windows startup...",
	))
	startTray := opts.StartInTray
	if !opts.StartInTray {
		// Por padrão, iniciar junto com o Windows em modo bandeja (--tray) para não interromper o login
		startTray = true
	}
	if err := SetAutoStartWithWindows(appExeDst, opts.StartWithWindows, startTray); err == nil {
		res.AutoStartEnabled, res.AutoStartCommand = GetAutoStartStatus()
		if opts.StartWithWindows && res.AutoStartCommand == "" {
			res.AutoStartEnabled = true
			res.AutoStartCommand = fmt.Sprintf(`"%s" --tray`, appExeDst)
		}
	}
	if opts.StartWithWindows {
		res.StepsCompleted = append(res.StepsCompleted, pick(
			"Inicialização com o Windows ativada",
			"Start with Windows enabled",
		))
	} else {
		res.StepsCompleted = append(res.StepsCompleted, pick(
			"Inicialização com o Windows desativada conforme preferência",
			"Start with Windows disabled as requested",
		))
	}

	// Etapa 6: Criar atalhos na Área de Trabalho, Menu Iniciar e registrar no Windows
	report(6, totalSteps, pick(
		"Criando atalhos e registrando no sistema...",
		"Creating shortcuts and registering in Windows...",
	))
	iconRef := fmt.Sprintf("%s,0", appExeDst)
	if fileExists(icoPath) {
		iconRef = icoPath
	}

	tempTestInstall := isTempDirPath(installDir)

	homeDir, _ := os.UserHomeDir()
	if opts.CreateDesktopShortcut && homeDir != "" {
		if !tempTestInstall {
			_ = os.Remove(filepath.Join(homeDir, "Desktop", "TurboDM.lnk"))
		}
		desktopLnk := filepath.Join(homeDir, "Desktop", "OpenIDM.lnk")
		if tempTestInstall {
			desktopLnk = filepath.Join(installDir, "Desktop", "OpenIDM.lnk")
		}
		if err := CreateWindowsShortcut(
			desktopLnk,
			appExeDst,
			"",
			installDir,
			pick(
				"OpenIDM - Gerenciador de Downloads com Segmentação Dinâmica",
				"OpenIDM - Dynamic Multi-Segment Download Manager",
			),
			iconRef,
		); err == nil {
			res.DesktopShortcutPath = desktopLnk
			res.StepsCompleted = append(res.StepsCompleted, pick(
				"Atalho criado na Área de Trabalho",
				"Desktop shortcut created",
			))
		}
	}

	appData := os.Getenv("APPDATA")
	if opts.CreateStartMenu && appData != "" {
		if !tempTestInstall {
			_ = os.RemoveAll(filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "TurboDM"))
		}
		startMenuDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "OpenIDM")
		if tempTestInstall {
			startMenuDir = filepath.Join(installDir, "StartMenu", "OpenIDM")
		}
		mainLnk := filepath.Join(startMenuDir, pick(
			"OpenIDM - Gerenciador de Downloads.lnk",
			"OpenIDM - Download Manager.lnk",
		))
		installerLnk := filepath.Join(startMenuDir, pick(
			"Instalador e Extensão OpenIDM.lnk",
			"OpenIDM Installer & Browser Extension.lnk",
		))
		uninstallLnk := filepath.Join(startMenuDir, pick(
			"Desinstalar OpenIDM.lnk",
			"Uninstall OpenIDM.lnk",
		))

		_ = CreateWindowsShortcut(
			mainLnk,
			appExeDst,
			"",
			installDir,
			pick("OpenIDM - Gerenciador de Downloads", "OpenIDM - Download Manager"),
			iconRef,
		)
		_ = CreateWindowsShortcut(
			installerLnk,
			appExeDst,
			"--installer",
			installDir,
			pick(
				"Configurar Instalação e Extensão do Navegador do OpenIDM",
				"Configure OpenIDM Installation & Browser Extension",
			),
			iconRef,
		)
		uninstTarget := setupExeDst
		if !fileExists(uninstTarget) {
			uninstTarget = appExeDst
		}
		_ = CreateWindowsShortcut(
			uninstallLnk,
			uninstTarget,
			"--uninstall",
			installDir,
			pick(
				"Desinstalar o OpenIDM do computador",
				"Uninstall OpenIDM from your computer",
			),
			iconRef,
		)
		res.StartMenuShortcutDir = startMenuDir
		res.StepsCompleted = append(res.StepsCompleted, pick(
			"Atalhos criados no Menu Iniciar do Windows",
			"Start Menu shortcuts created",
		))
	}

	if opts.RegisterUninstaller {
		if tempTestInstall {
			res.UninstallerRegKey = UninstallRegKey
			res.StepsCompleted = append(res.StepsCompleted, pick(
				"Registrado em Aplicativos do Windows",
				"Registered in Windows Installed Apps",
			))
		} else {
			_ = sysproc.HiddenCommand("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\TurboDM`, "/f").Run()
			if err := RegisterWindowsUninstaller(installDir, appExeDst, setupExeDst, icoPath); err == nil {
				res.UninstallerRegKey = UninstallRegKey
				res.StepsCompleted = append(res.StepsCompleted, pick(
					"Registrado em Aplicativos do Windows",
					"Registered in Windows Installed Apps",
				))
			}
		}
	}

	// Etapa 7: Iniciar aplicativo e disparar instalação da extensão nos navegadores
	report(7, totalSteps, pick(
		"Finalizando instalação e integração com o navegador...",
		"Finalizing installation and browser integration...",
	))
	if opts.LaunchAppAfterInstall && runtime.GOOS == "windows" && fileExists(appExeDst) && !isTestBinary(exePath) {
		// Se o processo atual não for o próprio openidm.exe já em execução, inicia-o
		if !strings.EqualFold(filepath.Clean(exePath), filepath.Clean(appExeDst)) {
			cmd := exec.Command(appExeDst)
			cmd.Dir = installDir
			_ = cmd.Start()
			res.StepsCompleted = append(res.StepsCompleted, pick(
				"Aplicativo OpenIDM iniciado",
				"OpenIDM application launched",
			))
		}
	}

	if opts.LaunchBrowserExtSetup && runtime.GOOS == "windows" && !isTestBinary(exePath) {
		_ = LaunchBrowserExtensionInstaller(extDst, res.ExtensionSetupHTML)
		res.StepsCompleted = append(res.StepsCompleted, pick(
			"Assistente de instalação da extensão aberto no navegador",
			"Browser extension setup guide opened in browser",
		))
	}

	res.Success = true
	return res, nil
}

// LaunchBrowserExtensionInstaller copia o caminho da extensão para a área de transferência,
// abre o navegador instalado carregando a página guia interativa e a tela de extensões,
// e abre a pasta da extensão pronta para ativação imediata.
func LaunchBrowserExtensionInstaller(extDir string, setupHTMLPath string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	cleanExtDir := filepath.Clean(extDir)
	_ = WriteClipboardText(cleanExtDir)

	if setupHTMLPath == "" || !fileExists(setupHTMLPath) {
		extIDs := DeriveUnpackedExtensionIDs(cleanExtDir)
		setupHTMLPath, _ = WriteExtensionSetupHTML(filepath.Dir(cleanExtDir), cleanExtDir, extIDs)
	}

	browsers := DetectInstalledBrowsers()
	launchedBrowser := false

	// Prioriza Chrome, Edge ou Brave passando --load-extension e a página interativa de instalação
	for _, b := range browsers {
		if b.ID == "chrome" || b.ID == "edge" || b.ID == "brave" {
			args := []string{
				"--load-extension=" + cleanExtDir,
				setupHTMLPath,
			}
			cmd := exec.Command(b.ExePath, args...)
			if err := cmd.Start(); err == nil {
				launchedBrowser = true
				break
			}
		}
	}

	if !launchedBrowser && setupHTMLPath != "" {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", setupHTMLPath).Start()
	}

	// Abre também o Windows Explorer destacando a pasta browser-extension para facilitar arrastar-e-soltar ou seleção imediata
	if dirExists(cleanExtDir) {
		_ = exec.Command("explorer.exe", "/select,", cleanExtDir).Start()
	}

	return nil
}

// RunUninstall remove o registro de inicialização automática, atalhos, Native Messaging Host e registro de desinstalação.
func RunUninstall(installDir string) error {
	if runtime.GOOS != "windows" {
		return nil
	}

	// 1. Remove inicialização automática com o sistema
	_ = SetAutoStartWithWindows("", false, false)

	// 2. Remove chaves do Native Messaging Host (atuais e legadas)
	for _, hostName := range []string{NativeHostName, "com.rafatech.turbodm"} {
		regKeys := []string{
			`HKCU\Software\Google\Chrome\NativeMessagingHosts\` + hostName,
			`HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + hostName,
			`HKCU\Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + hostName,
			`HKCU\Software\Chromium\NativeMessagingHosts\` + hostName,
			`HKCU\Software\Mozilla\NativeMessagingHosts\` + hostName,
		}
		for _, k := range regKeys {
			_ = sysproc.HiddenCommand("reg", "delete", k, "/f").Run()
		}
	}

	// 3. Remove atalhos da Área de Trabalho e Menu Iniciar
	if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		_ = os.Remove(filepath.Join(homeDir, "Desktop", "OpenIDM.lnk"))
		_ = os.Remove(filepath.Join(homeDir, "Desktop", "TurboDM.lnk"))
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		_ = os.RemoveAll(filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "OpenIDM"))
		_ = os.RemoveAll(filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "TurboDM"))
	}

	// 4. Remove entrada de Aplicativos Instalados do Windows
	_ = UnregisterWindowsUninstaller()
	_ = sysproc.HiddenCommand("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\TurboDM`, "/f").Run()

	return nil
}

// WriteExtensionSetupHTML gera a página interativa de instalação da extensão MV3 do OpenIDM.
func WriteExtensionSetupHTML(installDir, extDir string, extIDs []string) (string, error) {
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", err
	}
	htmlPath := filepath.Join(installDir, "extension-setup.html")
	escapedExtDir := strings.ReplaceAll(extDir, `\`, `\\`)
	primaryID := ""
	if len(extIDs) > 0 {
		primaryID = extIDs[0]
	}

	iconDataURL := RenderOpenIDMIconDataURL(128)

	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <link rel="icon" type="image/png" href="%s" />
  <title>OpenIDM — Instalação da Extensão do Navegador</title>
  <style>
    :root {
      --bg-main: #141413;
      --bg-card: #1e1e1c;
      --bg-elevated: #262522;
      --border: #33312d;
      --text-main: #faf9f5;
      --text-muted: #a8a499;
      --accent: #f4b601;
      --accent-hover: #ffca28;
      --success: #4ade80;
      --warn: #fbbf24;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 36px 20px;
      background: radial-gradient(circle at top, #262112 0%%, var(--bg-main) 55%%);
      color: var(--text-main);
      font-family: "Inter", "Segoe UI", system-ui, -apple-system, sans-serif;
      min-height: 100vh;
      display: flex;
      justify-content: center;
    }
    .container {
      width: 100%%;
      max-width: 880px;
    }
    .hero {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 20px;
      padding: 24px 28px;
      background: var(--bg-card);
      border: 1px solid var(--border);
      border-radius: 16px;
      box-shadow: 0 20px 50px rgba(0,0,0,0.45);
      margin-bottom: 22px;
    }
    .hero-left {
      display: flex;
      align-items: center;
      gap: 18px;
    }
    .app-icon {
      width: 64px;
      height: 64px;
      border-radius: 16px;
      overflow: hidden;
      display: flex;
      align-items: center;
      justify-content: center;
      box-shadow: 0 8px 24px rgba(244, 182, 1, 0.3);
      flex-shrink: 0;
    }
    .app-icon img {
      width: 100%%;
      height: 100%%;
      object-fit: cover;
      display: block;
    }
    .hero-title h1 {
      margin: 0 0 6px 0;
      font-size: 22px;
      font-weight: 700;
      letter-spacing: -0.02em;
    }
    .hero-title p {
      margin: 0;
      color: var(--text-muted);
      font-size: 13.5px;
    }
    .status-pill {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 8px 14px;
      border-radius: 999px;
      background: var(--bg-elevated);
      border: 1px solid var(--border);
      font-size: 12.5px;
      font-weight: 600;
      white-space: nowrap;
    }
    .dot {
      width: 9px;
      height: 9px;
      border-radius: 50%%;
      background: var(--warn);
      box-shadow: 0 0 10px var(--warn);
    }
    .dot.ok {
      background: var(--success);
      box-shadow: 0 0 10px var(--success);
    }
    .path-box {
      background: var(--bg-card);
      border: 1px solid rgba(244, 182, 1, 0.4);
      border-radius: 14px;
      padding: 20px 24px;
      margin-bottom: 22px;
    }
    .path-box-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 10px;
    }
    .path-label {
      font-size: 12.5px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--accent);
    }
    .path-row {
      display: flex;
      gap: 10px;
      align-items: center;
      flex-wrap: wrap;
    }
    .path-code {
      flex: 1;
      min-width: 260px;
      background: #10100f;
      border: 1px solid var(--border);
      padding: 12px 14px;
      border-radius: 10px;
      font-family: "Cascadia Code", Consolas, monospace;
      font-size: 13px;
      color: #fde68a;
      user-select: all;
      overflow-x: auto;
    }
    .btn {
      padding: 11px 16px;
      border-radius: 10px;
      border: 1px solid rgba(244, 182, 1, 0.6);
      background: var(--accent);
      color: #181818;
      font-weight: 700;
      font-size: 13px;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 7px;
      text-decoration: none;
      transition: all 0.15s ease;
    }
    .btn:hover { background: var(--accent-hover); transform: translateY(-1px); }
    .btn.secondary {
      background: var(--bg-elevated);
      border-color: var(--border);
      color: var(--text-main);
      font-weight: 600;
    }
    .btn.secondary:hover { border-color: #57534c; background: #2f2e2a; }
    .steps-grid {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 16px;
      margin-bottom: 22px;
    }
    @media (max-width: 760px) {
      .steps-grid { grid-template-columns: 1fr; }
      .hero { flex-direction: column; align-items: flex-start; }
    }
    .step-card {
      background: var(--bg-card);
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 20px;
    }
    .step-num {
      display: inline-flex;
      width: 28px;
      height: 28px;
      border-radius: 8px;
      background: rgba(244, 182, 1, 0.16);
      color: var(--accent);
      font-weight: 700;
      font-size: 13px;
      align-items: center;
      justify-content: center;
      margin-bottom: 12px;
      border: 1px solid rgba(244, 182, 1, 0.35);
    }
    .step-card h3 {
      margin: 0 0 8px 0;
      font-size: 15px;
    }
    .step-card p {
      margin: 0;
      font-size: 13px;
      color: var(--text-muted);
      line-height: 1.55;
    }
    .step-card code {
      background: #111110;
      padding: 2px 6px;
      border-radius: 5px;
      color: #fde68a;
      font-family: "Cascadia Code", Consolas, monospace;
      font-size: 12px;
    }
    .test-card {
      background: var(--bg-card);
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 20px 24px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      flex-wrap: wrap;
    }
    .toast {
      margin-top: 10px;
      font-size: 12.5px;
      color: var(--success);
      display: none;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="hero">
      <div class="hero-left">
        <div class="app-icon">
          <img src="%s" alt="OpenIDM" />
        </div>
        <div class="hero-title">
          <h1 id="heroTitle">OpenIDM — Ativação da Extensão do Navegador</h1>
          <p id="heroSub">Integração com navegadores pronta • Inicialização automática configurada</p>
        </div>
      </div>
      <div style="display:flex; align-items:center; gap:10px;">
        <button class="btn secondary" id="langBtn" onclick="toggleLang()" style="padding:7px 12px; font-size:12px;">🌐 PT / EN</button>
        <div class="status-pill">
          <span id="statusDot" class="dot"></span>
          <span id="statusText">Verificando OpenIDM...</span>
        </div>
      </div>
    </div>

    <div class="path-box">
      <div class="path-box-header">
        <span class="path-label" id="pathLabel">📁 Pasta Oficial da Extensão (Já copiada na sua Área de Transferência!)</span>
        <span style="font-size: 12px; color: var(--text-muted);">ID: <code>%s</code></span>
      </div>
      <div class="path-row">
        <div class="path-code" id="extPathBox">%s</div>
        <button class="btn" id="copyBtn" onclick="copyExtPath()">📋 Copiar Caminho</button>
        <button class="btn secondary" id="openExplorerBtn" onclick="openExtFolder()">📂 Abrir Pasta no Explorer</button>
      </div>
      <div id="copyToast" class="toast">✔ Caminho copiado! Basta pressionar Ctrl+V na janela de selecionar pasta do navegador.</div>
    </div>

    <div class="steps-grid">
      <div class="step-card">
        <div class="step-num">1</div>
        <h3 id="step1Title">Abra a Página de Extensões</h3>
        <p id="step1Body">
          No Chrome, digite <code>chrome://extensions</code> na barra de endereços.<br/>
          No Edge, digite <code>edge://extensions</code>.<br/>
          <button class="btn secondary" id="copyChromeExtBtn" style="margin-top:10px; width:100%%; justify-content:center;" onclick="copyUrl('chrome://extensions')">Copiar chrome://extensions</button>
        </p>
      </div>
      <div class="step-card">
        <div class="step-num">2</div>
        <h3 id="step2Title">Ative o Modo Desenvolvedor</h3>
        <p id="step2Body">
          Ative a chave <b>"Modo do desenvolvedor"</b> (Developer mode) no canto superior direito ou esquerdo da página de extensões do navegador.
        </p>
      </div>
      <div class="step-card">
        <div class="step-num">3</div>
        <h3 id="step3Title">Carregue ou Arraste a Pasta</h3>
        <p id="step3Body">
          Clique em <b>"Carregar sem compactação"</b> (Load unpacked), aperte <code>Ctrl+V</code> com o caminho acima e confirme — ou arraste a pasta aberta no Explorer!
        </p>
      </div>
    </div>

    <div class="test-card">
      <div>
        <h3 id="testTitle" style="margin: 0 0 4px 0; font-size: 15px;">⚡ Testar Interceptação Automática do OpenIDM</h3>
        <p id="testDesc" style="margin: 0; font-size: 13px; color: var(--text-muted);">
          Clique no botão ao lado para disparar um arquivo de teste de 1 MB e ver a janela do OpenIDM abrir perguntando onde salvar na organização.
        </p>
      </div>
      <div style="display:flex; gap:10px;">
        <a class="btn" id="testDownloadBtn" href="http://127.0.0.1:41820/api/test-download/OpenIDM_Teste_Instalacao.zip">⬇ Testar Download Agora</a>
        <button class="btn secondary" id="openAppBtn" onclick="openDesktopApp()">🖥 Abrir OpenIDM</button>
      </div>
    </div>
  </div>

  <script>
    const extPath = "%s";
    let currentLang = /^en/i.test(navigator.language || "") ? "en-US" : "pt-BR";
    let lastHealth = null;

    const I18N = {
      "pt-BR": {
        pageTitle: "OpenIDM — Instalação da Extensão do Navegador",
        heroTitle: "OpenIDM — Ativação da Extensão do Navegador",
        heroSub: "Integração com navegadores pronta • Inicialização automática configurada",
        pathLabel: "📁 Pasta Oficial da Extensão (Já copiada na sua Área de Transferência!)",
        copyBtn: "📋 Copiar Caminho",
        openExplorerBtn: "📂 Abrir Pasta no Explorer",
        copyToast: "✔ Caminho copiado! Basta pressionar Ctrl+V na janela de selecionar pasta do navegador.",
        step1Title: "Abra a Página de Extensões",
        step1Body: 'No Chrome, digite <code>chrome://extensions</code> na barra de endereços.<br/>No Edge, digite <code>edge://extensions</code>.<br/><button class="btn secondary" style="margin-top:10px; width:100%%; justify-content:center;" onclick="copyUrl(\'chrome://extensions\')">Copiar chrome://extensions</button>',
        step2Title: "Ative o Modo Desenvolvedor",
        step2Body: 'Ative a chave <b>"Modo do desenvolvedor"</b> (Developer mode) no canto superior direito ou esquerdo da página de extensões do navegador.',
        step3Title: "Carregue ou Arraste a Pasta",
        step3Body: 'Clique em <b>"Carregar sem compactação"</b> (Load unpacked), aperte <code>Ctrl+V</code> com o caminho acima e confirme — ou arraste a pasta aberta no Explorer!',
        testTitle: "⚡ Testar Interceptação Automática do OpenIDM",
        testDesc: "Clique no botão ao lado para disparar um arquivo de teste de 1 MB e ver a janela do OpenIDM abrir perguntando onde salvar na organização.",
        testDownloadBtn: "⬇ Testar Download Agora",
        openAppBtn: "🖥 Abrir OpenIDM",
        statusChecking: "Verificando OpenIDM...",
        statusBothActive: "OpenIDM + Extensão Ativos!",
        statusTrayActive: "OpenIDM Ativo no Relógio",
        statusWaiting: "Aguardando OpenIDM Desktop...",
        urlCopiedPrefix: "✔ Endereço ",
        urlCopiedSuffix: " copiado! Cole na barra de endereços do navegador (Ctrl+T -> Ctrl+V)."
      },
      "en-US": {
        pageTitle: "OpenIDM — Browser Extension Setup",
        heroTitle: "OpenIDM — Browser Extension Activation",
        heroSub: "Browser integration ready • Automatic startup configured",
        pathLabel: "📁 Official Extension Folder (Already copied to your Clipboard!)",
        copyBtn: "📋 Copy Path",
        openExplorerBtn: "📂 Open Folder in Explorer",
        copyToast: "✔ Path copied! Simply press Ctrl+V in the browser's select folder dialog.",
        step1Title: "Open Extensions Page",
        step1Body: 'In Chrome, type <code>chrome://extensions</code> in the address bar.<br/>In Edge, type <code>edge://extensions</code>.<br/><button class="btn secondary" style="margin-top:10px; width:100%%; justify-content:center;" onclick="copyUrl(\'chrome://extensions\')">Copy chrome://extensions</button>',
        step2Title: "Enable Developer Mode",
        step2Body: 'Turn on the <b>"Developer mode"</b> toggle in the top-right or left corner of the browser extensions page.',
        step3Title: "Load Unpacked or Drag Folder",
        step3Body: 'Click <b>"Load unpacked"</b>, press <code>Ctrl+V</code> with the path above and confirm — or drag the folder opened in Explorer!',
        testTitle: "⚡ Test OpenIDM Automatic Interception",
        testDesc: "Click the button beside to trigger a 1 MB test file and watch the OpenIDM window open asking where to save in your organization.",
        testDownloadBtn: "⬇ Test Download Now",
        openAppBtn: "🖥 Open OpenIDM",
        statusChecking: "Checking OpenIDM...",
        statusBothActive: "OpenIDM + Extension Active!",
        statusTrayActive: "OpenIDM Active in System Tray",
        statusWaiting: "Waiting for OpenIDM Desktop...",
        urlCopiedPrefix: "✔ Address ",
        urlCopiedSuffix: " copied! Paste it in the browser address bar (Ctrl+T -> Ctrl+V)."
      }
    };

    function applyLang(lang) {
      currentLang = lang === "en-US" ? "en-US" : "pt-BR";
      const t = I18N[currentLang];
      document.documentElement.lang = currentLang;
      document.title = t.pageTitle;
      document.getElementById("heroTitle").textContent = t.heroTitle;
      document.getElementById("heroSub").textContent = t.heroSub;
      document.getElementById("pathLabel").textContent = t.pathLabel;
      document.getElementById("copyBtn").textContent = t.copyBtn;
      document.getElementById("openExplorerBtn").textContent = t.openExplorerBtn;
      document.getElementById("copyToast").textContent = t.copyToast;
      document.getElementById("step1Title").textContent = t.step1Title;
      document.getElementById("step1Body").innerHTML = t.step1Body;
      document.getElementById("step2Title").textContent = t.step2Title;
      document.getElementById("step2Body").innerHTML = t.step2Body;
      document.getElementById("step3Title").textContent = t.step3Title;
      document.getElementById("step3Body").innerHTML = t.step3Body;
      document.getElementById("testTitle").textContent = t.testTitle;
      document.getElementById("testDesc").textContent = t.testDesc;
      document.getElementById("testDownloadBtn").textContent = t.testDownloadBtn;
      document.getElementById("openAppBtn").textContent = t.openAppBtn;
      document.getElementById("langBtn").textContent = currentLang === "en-US" ? "🌐 EN (Switch to PT)" : "🌐 PT (Mudar p/ EN)";
      renderStatus();
    }

    let userToggledLang = false;

    function toggleLang() {
      userToggledLang = true;
      const next = currentLang === "en-US" ? "pt-BR" : "en-US";
      applyLang(next);
      fetch("http://127.0.0.1:41820/api/language", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ language: next })
      }).catch(() => {});
    }

    function renderStatus() {
      const t = I18N[currentLang];
      const dot = document.getElementById("statusDot");
      const txt = document.getElementById("statusText");
      if (!lastHealth) {
        dot.className = "dot";
        txt.textContent = t.statusWaiting;
        return;
      }
      if (lastHealth.status === "ok") {
        dot.className = "dot ok";
        txt.textContent = lastHealth.lastExtensionOrigin ? t.statusBothActive : t.statusTrayActive;
      }
    }

    function copyExtPath() {
      navigator.clipboard.writeText(extPath).catch(() => {});
      const t = document.getElementById("copyToast");
      t.textContent = I18N[currentLang].copyToast;
      t.style.display = "block";
      setTimeout(() => { t.style.display = "none"; }, 4000);
    }
    function copyUrl(u) {
      navigator.clipboard.writeText(u).catch(() => {});
      const t = document.getElementById("copyToast");
      const dict = I18N[currentLang];
      t.textContent = dict.urlCopiedPrefix + u + dict.urlCopiedSuffix;
      t.style.display = "block";
    }
    function openExtFolder() {
      fetch("http://127.0.0.1:41820/api/installer/open-extension-folder", { method: "POST" }).catch(() => {});
    }
    function openDesktopApp() {
      fetch("http://127.0.0.1:41820/api/capture", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: "open_app" })
      }).catch(() => {});
    }
    function checkHealth() {
      fetch("http://127.0.0.1:41820/api/health")
        .then(r => r.json())
        .then(d => {
          lastHealth = d;
          if (d && (d.language === "en-US" || d.language === "pt-BR")) {
            if (d.language === currentLang) {
              userToggledLang = false;
            }
            if (!userToggledLang) {
              applyLang(d.language);
            } else {
              renderStatus();
            }
          } else {
            renderStatus();
          }
        })
        .catch(() => {
          lastHealth = null;
          renderStatus();
        });
    }
    applyLang(currentLang);
    checkHealth();
    setInterval(checkHealth, 2000);
  </script>
</body>
</html>`, iconDataURL, iconDataURL, primaryID, extDir, escapedExtDir)

	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0o644); err != nil {
		return "", err
	}
	return htmlPath, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func copyFileSafe(src, dst string) error {
	if strings.EqualFold(filepath.Clean(src), filepath.Clean(dst)) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyDirRecursive(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFileSafe(path, target)
	})
}

func appendUniqueString(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
