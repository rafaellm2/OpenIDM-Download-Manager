package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"openidm/internal/sysproc"
)

const (
	// DefaultGitHubRepo define o repositório oficial do OpenIDM no GitHub.
	DefaultGitHubRepo = "rafaellm2/OpenIDM-Download-Manager"
	// DefaultGitHubReleasesAPI aponta para o endpoint oficial da última release publicada no GitHub.
	DefaultGitHubReleasesAPI = "https://api.github.com/repos/" + DefaultGitHubRepo + "/releases/latest"
	// DefaultGitHubReleasesPage aponta para a página web de releases do repositório.
	DefaultGitHubReleasesPage = "https://github.com/" + DefaultGitHubRepo + "/releases"
)

// UpdateInfo reúne os metadados de verificação de nova versão no GitHub Releases.
type UpdateInfo struct {
	Available      bool   `json:"available"`
	Postponed      bool   `json:"postponed"`
	PostponedUntil int64  `json:"postponedUntil,omitempty"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseName    string `json:"releaseName"`
	ReleaseNotes   string `json:"releaseNotes"`
	ReleaseURL     string `json:"releaseUrl"`
	DownloadURL    string `json:"downloadUrl"`
	AssetName      string `json:"assetName"`
	AssetSize      int64  `json:"assetSize"`
	PublishedAt    string `json:"publishedAt,omitempty"`
	CheckedAt      string `json:"checkedAt"`
}

// githubReleasePayload representa o JSON retornado pela API do GitHub Releases.
type githubReleasePayload struct {
	TagName     string               `json:"tag_name"`
	Name        string               `json:"name"`
	Body        string               `json:"body"`
	HTMLURL     string               `json:"html_url"`
	Draft       bool                 `json:"draft"`
	Prerelease  bool                 `json:"prerelease"`
	PublishedAt string               `json:"published_at"`
	Assets      []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CompareVersions compara duas strings SemVer (ex: "v1.2.0" vs "1.0.0").
// Retorna 1 se v1 > v2, -1 se v1 < v2 e 0 se forem equivalentes.
func CompareVersions(v1, v2 string) int {
	p1 := parseVersionParts(v1)
	p2 := parseVersionParts(v2)
	maxLen := len(p1)
	if len(p2) > maxLen {
		maxLen = len(p2)
	}
	for i := 0; i < maxLen; i++ {
		var a, b int
		if i < len(p1) {
			a = p1[i]
		}
		if i < len(p2) {
			b = p2[i]
		}
		if a > b {
			return 1
		}
		if a < b {
			return -1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	clean := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(v), "v"), "V"))
	if idx := strings.IndexAny(clean, "-+"); idx >= 0 {
		clean = clean[:idx]
	}
	if clean == "" {
		return []int{0, 0, 0}
	}
	rawParts := strings.Split(clean, ".")
	out := make([]int, 0, len(rawParts))
	for _, p := range rawParts {
		var digits strings.Builder
		for _, r := range p {
			if r >= '0' && r <= '9' {
				digits.WriteRune(r)
			} else {
				break
			}
		}
		n, _ := strconv.Atoi(digits.String())
		out = append(out, n)
	}
	for len(out) < 3 {
		out = append(out, 0)
	}
	return out
}

// CheckForUpdate consulta o GitHub Releases e verifica se há uma versão mais recente que currentVersion.
func CheckForUpdate(ctx context.Context, apiURL string, currentVersion string, postponedUntilUnix int64) (*UpdateInfo, error) {
	if strings.TrimSpace(apiURL) == "" {
		apiURL = DefaultGitHubReleasesAPI
	}
	if strings.TrimSpace(currentVersion) == "" {
		currentVersion = AppVersion
	}

	reqCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "OpenIDM-Updater/"+currentVersion)

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar servidor de atualizações: %w", err)
	}
	defer resp.Body.Close()

	nowStr := time.Now().Format(time.RFC3339)
	postponed := postponedUntilUnix > time.Now().Unix()

	// Caso o repositório ainda não possua release publicada (404), retorna que a versão atual já é a mais recente.
	if resp.StatusCode == http.StatusNotFound {
		return &UpdateInfo{
			Available:      false,
			Postponed:      postponed,
			PostponedUntil: postponedUntilUnix,
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
			ReleaseName:    "OpenIDM v" + currentVersion,
			ReleaseURL:     DefaultGitHubReleasesPage,
			CheckedAt:      nowStr,
		}, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("servidor GitHub retornou HTTP %d", resp.StatusCode)
	}

	var rel githubReleasePayload
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("resposta inválida da API de releases: %w", err)
	}

	latestVer := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v"), "V")
	if latestVer == "" {
		latestVer = currentVersion
	}

	isNewer := !rel.Draft && CompareVersions(latestVer, currentVersion) > 0
	assetURL, assetName, assetSize := selectBestReleaseAsset(rel.Assets)
	releasePage := strings.TrimSpace(rel.HTMLURL)
	if releasePage == "" {
		releasePage = DefaultGitHubReleasesPage
	}

	return &UpdateInfo{
		Available:      isNewer,
		Postponed:      postponed,
		PostponedUntil: postponedUntilUnix,
		CurrentVersion: currentVersion,
		LatestVersion:  latestVer,
		ReleaseName:    strings.TrimSpace(rel.Name),
		ReleaseNotes:   strings.TrimSpace(rel.Body),
		ReleaseURL:     releasePage,
		DownloadURL:    assetURL,
		AssetName:      assetName,
		AssetSize:      assetSize,
		PublishedAt:    rel.PublishedAt,
		CheckedAt:      nowStr,
	}, nil
}

// selectBestReleaseAsset escolhe o instalador Windows (.exe) prioritário dentre os arquivos anexados à release.
func selectBestReleaseAsset(assets []githubReleaseAsset) (string, string, int64) {
	if len(assets) == 0 {
		return "", "", 0
	}
	// 1ª prioridade: Instalador oficial OpenIDM-Setup*.exe
	for _, a := range assets {
		lower := strings.ToLower(strings.TrimSpace(a.Name))
		if strings.HasSuffix(lower, ".exe") && strings.Contains(lower, "setup") {
			return a.BrowserDownloadURL, a.Name, a.Size
		}
	}
	// 2ª prioridade: Qualquer executável Windows (.exe)
	for _, a := range assets {
		lower := strings.ToLower(strings.TrimSpace(a.Name))
		if strings.HasSuffix(lower, ".exe") && !strings.Contains(lower, "native-host") {
			return a.BrowserDownloadURL, a.Name, a.Size
		}
	}
	// 3ª prioridade: Pacote .msi ou .zip
	for _, a := range assets {
		lower := strings.ToLower(strings.TrimSpace(a.Name))
		if strings.HasSuffix(lower, ".msi") || strings.HasSuffix(lower, ".zip") {
			return a.BrowserDownloadURL, a.Name, a.Size
		}
	}
	return assets[0].BrowserDownloadURL, assets[0].Name, assets[0].Size
}

// DownloadUpdateAsset baixa o instalador da nova release para a pasta temporária do sistema reportando progresso.
func DownloadUpdateAsset(
	ctx context.Context,
	downloadURL string,
	assetName string,
	onProgress func(percent int, downloaded int64, total int64),
) (string, error) {
	cleanURL := strings.TrimSpace(downloadURL)
	if cleanURL == "" {
		return "", errors.New("URL de download da atualização vazia")
	}
	cleanName := filepath.Base(strings.TrimSpace(assetName))
	if cleanName == "" || cleanName == "." || cleanName == "/" || cleanName == "\\" {
		cleanName = "OpenIDM-Setup-Update.exe"
	}

	updateDir := filepath.Join(os.TempDir(), "OpenIDM-Update")
	if err := os.MkdirAll(updateDir, 0o755); err != nil {
		return "", fmt.Errorf("erro ao criar diretório temporário de atualização: %w", err)
	}

	targetPath := filepath.Join(updateDir, cleanName)
	partPath := targetPath + ".part"
	_ = os.Remove(partPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cleanURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "OpenIDM-Updater/"+AppVersion)

	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro ao baixar pacote de atualização: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("falha no download da atualização: HTTP %d", resp.StatusCode)
	}

	outFile, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 64*1024)
	lastReport := time.Now()

	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := outFile.Write(buf[:n]); wErr != nil {
				_ = outFile.Close()
				return "", wErr
			}
			downloaded += int64(n)
			if onProgress != nil && time.Since(lastReport) >= 120*time.Millisecond {
				lastReport = time.Now()
				pct := 0
				if total > 0 {
					pct = int((downloaded * 100) / total)
				}
				onProgress(pct, downloaded, total)
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			_ = outFile.Close()
			return "", rErr
		}
	}

	_ = outFile.Sync()
	_ = outFile.Close()

	if onProgress != nil {
		onProgress(100, downloaded, total)
	}

	_ = os.Remove(targetPath)
	if err := os.Rename(partPath, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

// LaunchDownloadedUpdate executa o pacote baixado para atualizar e reiniciar o OpenIDM automaticamente.
func LaunchDownloadedUpdate(downloadedPath string, installDir string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if strings.TrimSpace(installDir) == "" {
		st := GetInstallationStatus("")
		installDir = st.InstallDir
	}
	lowerName := strings.ToLower(filepath.Base(downloadedPath))

	// Caso 1: Instalador OpenIDM-Setup*.exe -> suporta flags de atualização silenciosa com reinício automático.
	if strings.HasSuffix(lowerName, ".exe") && strings.Contains(lowerName, "setup") {
		args := []string{
			"--silent",
			"--stop-running",
			"--launch-app",
		}
		if strings.TrimSpace(installDir) != "" {
			args = append(args, "--install-dir", installDir)
		}
		cmd := exec.Command(downloadedPath, args...)
		cmd.Dir = filepath.Dir(downloadedPath)
		return cmd.Start()
	}

	// Caso 2: Executável direto openidm.exe -> aguarda o encerramento do processo atual, substitui e reinicia.
	if strings.HasSuffix(lowerName, ".exe") {
		dstExe := filepath.Join(installDir, "openidm.exe")
		escSrc := strings.ReplaceAll(downloadedPath, "'", "''")
		escDst := strings.ReplaceAll(dstExe, "'", "''")
		psCmd := fmt.Sprintf(
			`Start-Sleep -Milliseconds 900; Copy-Item -Path '%s' -Destination '%s' -Force; Start-Process -FilePath '%s'`,
			escSrc, escDst, escDst,
		)
		return sysproc.HiddenCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", psCmd).Start()
	}

	// Caso 3: Outros formatos (.zip / .msi) -> abre no shell padrão do Windows.
	return sysproc.HiddenCommand("cmd", "/c", "start", "", downloadedPath).Start()
}
