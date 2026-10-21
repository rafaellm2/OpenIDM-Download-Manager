package integration

import (
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"openidm/internal/engine"
	"openidm/internal/models"
)

// Lista de domínios de plataformas de vídeo/streaming e redes sociais que não devem ser capturados como download direto.
var ignoredStreamingHosts = []string{
	"youtube.com",
	"youtu.be",
	"googlevideo.com",
	"ytimg.com",
	"twitch.tv",
	"ttvnw.net",
	"vimeo.com",
	"dailymotion.com",
	"tiktok.com",
	"instagram.com",
	"facebook.com",
	"fb.watch",
	"twitter.com",
	"x.com",
	"netflix.com",
	"primevideo.com",
	"disneyplus.com",
	"spotify.com",
}

// Extensões de páginas web, scripts e arquivos de código que nunca devem acionar captura automática no clipboard/navegação.
var nonDownloadWebExtensions = map[string]bool{
	"html":  true,
	"htm":   true,
	"mhtml": true,
	"xhtml": true,
	"php":   true,
	"asp":   true,
	"aspx":  true,
	"jsp":   true,
	"cgi":   true,
	"do":    true,
	"js":    true,
	"mjs":   true,
	"cjs":   true,
	"ts":    true,
	"tsx":   true,
	"jsx":   true,
	"css":   true,
	"scss":  true,
	"json":  true,
	"xml":   true,
	"map":   true,
	"txt":   true,
	"md":    true,
}

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

	host := strings.ToLower(u.Hostname())
	var refHost string
	if strings.TrimSpace(referrer) != "" {
		if refURL, rErr := url.Parse(strings.TrimSpace(referrer)); rErr == nil {
			refHost = strings.ToLower(refURL.Hostname())
		}
	}

	// 2. Ignora servidores locais (localhost, 127.0.0.1, ::1, IPs de rede local) exceto a rota interna de teste de instalação.
	if isLocalOrLoopbackHost(host, u.Path) || (refHost != "" && isLocalOrLoopbackHost(refHost, u.Path)) {
		return false, "endereço local (localhost/LAN) ignorado"
	}

	// 3. Ignora links de sites de vídeo/streaming (YouTube, Twitch, Vimeo, TikTok, etc.).
	if isStreamingPlatformHost(host) || (refHost != "" && isStreamingPlatformHost(refHost)) {
		return false, "plataforma de streaming/vídeo ignorada"
	}

	// 4. Verifica exceções por site com suporte a wildcards (*.exemplo.com) tanto na URL quanto na página de origem (referrer).
	for _, r := range siteRules {
		if r.IgnoreCapture {
			if engine.MatchHostWildcard(r.HostPattern, host) || (refHost != "" && engine.MatchHostWildcard(r.HostPattern, refHost)) {
				return false, "site na lista de exceções (" + r.HostPattern + ")"
			}
		}
	}

	// 5. Ignora páginas de navegação de repositórios (tree, commit, issues, pull, wiki) que não são arquivos brutos.
	if customFileName == "" && isWebNavigationPath(u, host) {
		return false, "página de navegação web ignorada"
	}

	// 6. Verifica a extensão real do arquivo sem considerar o fallback genérico "download.bin" de URLs sem caminho.
	fileName := engine.SanitizeDownloadFileName(customFileName)
	if strings.EqualFold(fileName, "download.bin") {
		fileName = ""
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	if ext == "" {
		extracted := engine.ExtractFileNameFromURL(cleanURL)
		// Se ExtractFileNameFromURL retornou o fallback "download.bin", a URL é apenas um site/rota sem extensão real.
		if !strings.EqualFold(extracted, "download.bin") {
			fileName = extracted
			ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
		}
	}

	// Ignora extensões típicas de páginas web, scripts ou arquivos TypeScript (.ts, .html, .js, .md, .txt).
	if ext != "" && nonDownloadWebExtensions[ext] {
		return false, "extensão de página/código web ignorada (." + ext + ")"
	}

	if ext != "" {
		for _, allowed := range settings.CaptureExtensions {
			cleanAllowed := strings.ToLower(strings.TrimPrefix(allowed, "."))
			if nonDownloadWebExtensions[cleanAllowed] || cleanAllowed == "bin" {
				continue
			}
			if strings.EqualFold(cleanAllowed, ext) {
				return true, "extensão monitorada (" + ext + ")"
			}
		}
	}

	// 7. Reconhece padrões conhecidos de links diretos de download sem extensão aparente no path.
	if isKnownDirectDownloadPattern(u, host) {
		return true, "padrão de link direto de download reconhecido"
	}

	if ext == "" {
		return false, "sem extensão de arquivo identificável"
	}

	return false, "extensão não monitorada (" + ext + ")"
}

// isLocalOrLoopbackHost identifica se o host é localhost, loopback (127.0.0.1, ::1) ou IP privado de rede local,
// preservando apenas o endpoint explícito de teste (/api/test-download/).
func isLocalOrLoopbackHost(host string, urlPath string) bool {
	if strings.Contains(strings.ToLower(urlPath), "/api/test-download/") {
		return false
	}
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	if h == "localhost" || h == "0.0.0.0" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
	}
	return false
}

// isStreamingPlatformHost verifica se o domínio pertence a plataformas de vídeo/streaming onde links copiados são páginas e não arquivos.
func isStreamingPlatformHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	for _, domain := range ignoredStreamingHosts {
		if h == domain || strings.HasSuffix(h, "."+domain) {
			return true
		}
	}
	return false
}

// isWebNavigationPath detecta rotas comuns de páginas web (ex: GitHub/HuggingFace tree, issues, pull, watch, embed).
func isWebNavigationPath(u *url.URL, host string) bool {
	if u == nil {
		return false
	}
	lowerPath := strings.ToLower(u.Path)
	if lowerPath == "" || lowerPath == "/" {
		return u.RawQuery == "" || !isKnownDirectDownloadPattern(u, host)
	}
	navSegments := []string{
		"/tree/", "/commit/", "/commits/", "/issues/", "/pull/", "/pulls/",
		"/discussions/", "/wiki/", "/watch", "/embed/", "/shorts/", "/live/",
	}
	for _, seg := range navSegments {
		if strings.Contains(lowerPath, seg) {
			return true
		}
	}
	return false
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

	host := strings.ToLower(u.Hostname())
	var refHost string
	if strings.TrimSpace(referrer) != "" {
		if refURL, rErr := url.Parse(strings.TrimSpace(referrer)); rErr == nil {
			refHost = strings.ToLower(refURL.Hostname())
		}
	}

	// 2. Ignora downloads originados em servidores locais (localhost / 127.0.0.1 / LAN), exceto teste oficial do OpenIDM.
	if isLocalOrLoopbackHost(host, u.Path) || (refHost != "" && isLocalOrLoopbackHost(refHost, u.Path)) {
		return false, "endereço local (localhost/LAN) ignorado"
	}

	// 3. Ignora plataformas de vídeo/streaming.
	if isStreamingPlatformHost(host) || (refHost != "" && isStreamingPlatformHost(refHost)) {
		return false, "plataforma de streaming/vídeo ignorada"
	}

	// 4. Verifica exceções por site com suporte a wildcards (*.exemplo.com) tanto na URL quanto na página de origem (referrer).
	for _, r := range siteRules {
		if r.IgnoreCapture {
			if engine.MatchHostWildcard(r.HostPattern, host) || (refHost != "" && engine.MatchHostWildcard(r.HostPattern, refHost)) {
				return false, "site na lista de exceções (" + r.HostPattern + ")"
			}
		}
	}

	// 5. Evita capturar páginas web estáticas ou arquivos de código (.html/.htm/.mhtml/.xhtml/.js/.ts).
	cleanCustom := engine.SanitizeDownloadFileName(customFileName)
	customExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(cleanCustom)), ".")
	pathExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(u.Path)), ".")
	if customExt != "" && nonDownloadWebExtensions[customExt] {
		return false, "página/arquivo web comum (." + customExt + ")"
	}
	if cleanCustom == "" && pathExt != "" && nonDownloadWebExtensions[pathExt] {
		return false, "página/arquivo web comum (." + pathExt + ")"
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
