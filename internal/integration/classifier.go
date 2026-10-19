package integration

import (
	"net/url"
	"path/filepath"
	"strings"

	"openidm/internal/engine"
	"openidm/internal/models"
)

// ShouldCaptureURL avalia se uma URL clicada no navegador ou copiada no clipboard deve ser capturada,
// respeitando tecla modificadora (ALT), lista de exceções por site (*.exemplo.com) e extensões monitoradas.
func ShouldCaptureURL(
	rawURL string,
	modifierPressed string,
	settings models.AppSettings,
	siteRules []models.SiteRule,
) (bool, string) {
	return ShouldCaptureDownload(rawURL, "", "", modifierPressed, settings, siteRules)
}

// ShouldCaptureDownload avalia URL, nome de arquivo informado pelo navegador (ex: Content-Disposition) e domínio referrer.
func ShouldCaptureDownload(
	rawURL string,
	customFileName string,
	referrer string,
	modifierPressed string,
	settings models.AppSettings,
	siteRules []models.SiteRule,
) (bool, string) {
	cleanURL := engine.NormalizeDownloadURL(strings.Trim(strings.TrimSpace(rawURL), "\"'<>"))
	u, err := url.Parse(cleanURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ftp") || u.Host == "" {
		return false, "protocolo não suportado"
	}

	// 1. Se o usuário segurou a tecla de ignorar captura (ex: ALT), deixa o navegador baixar normalmente.
	if settings.IgnoreModifierKey != "" && strings.EqualFold(modifierPressed, settings.IgnoreModifierKey) {
		return false, "tecla modificadora pressionada para ignorar captura"
	}

	// 2. Verifica exceções por site com suporte a wildcards (*.exemplo.com) tanto na URL quanto na página de origem (referrer).
	host := strings.ToLower(u.Hostname())
	var refHost string
	if strings.TrimSpace(referrer) != "" {
		if refURL, rErr := url.Parse(strings.TrimSpace(referrer)); rErr == nil {
			refHost = strings.ToLower(refURL.Hostname())
		}
	}
	for _, r := range siteRules {
		if r.IgnoreCapture {
			if engine.MatchHostWildcard(r.HostPattern, host) || (refHost != "" && engine.MatchHostWildcard(r.HostPattern, refHost)) {
				return false, "site na lista de exceções (" + r.HostPattern + ")"
			}
		}
	}

	// 3. Verifica se a extensão do arquivo (pelo nome sugerido ou pela URL) pertence à lista de captura automática.
	fileName := engine.SanitizeDownloadFileName(customFileName)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	if ext == "" {
		fileName = engine.ExtractFileNameFromURL(cleanURL)
		ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	}

	if ext != "" {
		for _, allowed := range settings.CaptureExtensions {
			if strings.EqualFold(strings.TrimPrefix(allowed, "."), ext) {
				return true, "extensão monitorada (" + ext + ")"
			}
		}
	}

	// 4. Reconhece padrões conhecidos de links diretos de download sem extensão aparente no path.
	if isKnownDirectDownloadPattern(u, host) {
		return true, "padrão de link direto de download reconhecido"
	}

	if ext == "" {
		return false, "sem extensão de arquivo identificável"
	}

	return false, "extensão não monitorada (" + ext + ")"
}

// isKnownDirectDownloadPattern identifica URLs de download direto que frequentemente não exibem extensão no final do path.
func isKnownDirectDownloadPattern(u *url.URL, host string) bool {
	if u == nil {
		return false
	}
	lowerPath := strings.ToLower(u.Path)
	lowerQuery := strings.ToLower(u.RawQuery)

	if host == "codeload.github.com" || host == "drive.usercontent.google.com" || host == "objects.githubusercontent.com" {
		return true
	}
	if strings.Contains(lowerPath, "/api/test-download/") ||
		strings.Contains(lowerPath, "/releases/download/") ||
		(host == "github.com" && strings.Contains(lowerPath, "/archive/")) {
		return true
	}
	if (host == "huggingface.co" || strings.HasSuffix(host, ".huggingface.co")) && strings.Contains(lowerPath, "/resolve/") {
		return true
	}
	if (host == "sourceforge.net" || strings.HasSuffix(host, ".sourceforge.net")) && strings.Contains(lowerPath, "/files/") && strings.HasSuffix(lowerPath, "/download") {
		return true
	}
	if (host == "mediafire.com" || strings.HasSuffix(host, ".mediafire.com")) && strings.Contains(lowerPath, "/file/") {
		return true
	}
	if strings.Contains(lowerQuery, "download=true") ||
		strings.Contains(lowerQuery, "download=1") ||
		strings.Contains(lowerQuery, "dl=1") ||
		strings.Contains(lowerQuery, "export=download") ||
		strings.Contains(lowerQuery, "response-content-disposition=") ||
		strings.Contains(lowerQuery, "rscd=") ||
		strings.Contains(lowerQuery, "content-disposition=") {
		return true
	}
	return false
}

// ShouldCaptureBrowserDownload avalia um download já iniciado ou interceptado pela extensão do navegador
// (chrome.downloads.onCreated / onDeterminingFilename / onChanged / clique em link de download).
// Como o navegador já confirmou que se trata de um download (e no onCreated o filename pode ainda estar vazio
// ou a URL ser um redirecionamento de CDN como HuggingFace/GitHub/Google Drive), captura qualquer protocolo
// suportado exceto quando bloqueado pela tecla modificadora (ALT), lista de exceções do site ou página .html.
func ShouldCaptureBrowserDownload(
	rawURL string,
	customFileName string,
	referrer string,
	modifierPressed string,
	settings models.AppSettings,
	siteRules []models.SiteRule,
) (bool, string) {
	cleanURL := engine.NormalizeDownloadURL(strings.Trim(strings.TrimSpace(rawURL), "\"'<>"))
	u, err := url.Parse(cleanURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ftp") || u.Host == "" {
		return false, "protocolo não suportado"
	}

	// 1. Se o usuário segurou a tecla de ignorar captura (ex: ALT), deixa o navegador baixar normalmente.
	if settings.IgnoreModifierKey != "" && strings.EqualFold(modifierPressed, settings.IgnoreModifierKey) {
		return false, "tecla modificadora pressionada para ignorar captura"
	}

	// 2. Verifica exceções por site com suporte a wildcards (*.exemplo.com) tanto na URL quanto na página de origem (referrer).
	host := strings.ToLower(u.Hostname())
	var refHost string
	if strings.TrimSpace(referrer) != "" {
		if refURL, rErr := url.Parse(strings.TrimSpace(referrer)); rErr == nil {
			refHost = strings.ToLower(refURL.Hostname())
		}
	}
	for _, r := range siteRules {
		if r.IgnoreCapture {
			if engine.MatchHostWildcard(r.HostPattern, host) || (refHost != "" && engine.MatchHostWildcard(r.HostPattern, refHost)) {
				return false, "site na lista de exceções (" + r.HostPattern + ")"
			}
		}
	}

	// 3. Evita capturar páginas web estáticas (.html/.htm/.mhtml/.xhtml) mesmo quando o navegador informa o nome ao salvar página.
	cleanCustom := engine.SanitizeDownloadFileName(customFileName)
	customExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(cleanCustom)), ".")
	pathExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(u.Path)), ".")
	if customExt == "html" || customExt == "htm" || customExt == "mhtml" || customExt == "xhtml" {
		return false, "página web comum (." + customExt + ")"
	}
	if cleanCustom == "" && (pathExt == "html" || pathExt == "htm" || pathExt == "mhtml" || pathExt == "xhtml") {
		return false, "página web comum (." + pathExt + ")"
	}

	return true, "download interceptado pelo navegador"
}

// SuggestCategoryAndDir sugere a categoria e pasta padrão com base no nome do arquivo ou URL.
func SuggestCategoryAndDir(rawURL, customFileName string, categories []models.Category, defaultDir string) (string, string, string) {
	cleanURL := engine.NormalizeDownloadURL(strings.Trim(strings.TrimSpace(rawURL), "\"'<>"))
	fileName := engine.SanitizeDownloadFileName(customFileName)
	if fileName == "" {
		fileName = engine.ExtractFileNameFromURL(cleanURL)
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")

	for _, c := range categories {
		for _, candidate := range c.Extensions {
			if strings.EqualFold(candidate, ext) {
				return fileName, c.ID, c.DefaultDir
			}
		}
	}

	for _, c := range categories {
		if c.ID == "general" {
			return fileName, c.ID, c.DefaultDir
		}
	}
	return fileName, "general", defaultDir
}
