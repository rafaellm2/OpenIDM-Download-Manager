package engine

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"openidm/internal/models"

	"golang.org/x/net/proxy"
)

var (
	rePACProxy = regexp.MustCompile(`(?i)\bPROXY\s+([a-zA-Z0-9.\-]+:\d+)`)
	rePACSocks = regexp.MustCompile(`(?i)\bSOCKS5?\s+([a-zA-Z0-9.\-]+:\d+)`)
)

// BuildHTTPClient constrói um http.Client otimizado para múltiplos segmentos paralelos
// com Keep-Alive persistente (reuso de conexões TCP/TLS sem novo handshake).
func BuildHTTPClient(settings models.AppSettings, maxConns int) *http.Client {
	if maxConns <= 0 {
		maxConns = 32
	}

	dialer := &net.Dialer{
		Timeout:   20 * time.Second,
		KeepAlive: 60 * time.Second,
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     false, // HTTP/1.1 dedicado por worker garante conexões TCP dedicadas concorrentes
		MaxIdleConns:          maxConns * 2,
		MaxIdleConnsPerHost:   maxConns * 2,
		MaxConnsPerHost:       maxConns * 2,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		DisableKeepAlives:     false, // Essencial para reutilizar conexões após conclusão de um segmento
		DisableCompression:    true,  // Essencial para Range bytes exatos sem transparência gzip
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	// Configura Proxy conforme opções do usuário (Sistema, HTTP/HTTPS, SOCKS5 ou PAC).
	switch settings.ProxyType {
	case "system":
		transport.Proxy = http.ProxyFromEnvironment
	case "http":
		if settings.ProxyHost != "" && settings.ProxyPort > 0 {
			var proxyURL *url.URL
			if settings.ProxyUser != "" {
				proxyURL = &url.URL{
					Scheme: "http",
					User:   url.UserPassword(settings.ProxyUser, settings.ProxyPass),
					Host:   fmt.Sprintf("%s:%d", settings.ProxyHost, settings.ProxyPort),
				}
			} else {
				proxyURL = &url.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s:%d", settings.ProxyHost, settings.ProxyPort),
				}
			}
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	case "socks5":
		if settings.ProxyHost != "" && settings.ProxyPort > 0 {
			var auth *proxy.Auth
			if settings.ProxyUser != "" {
				auth = &proxy.Auth{
					User:     settings.ProxyUser,
					Password: settings.ProxyPass,
				}
			}
			addr := fmt.Sprintf("%s:%d", settings.ProxyHost, settings.ProxyPort)
			configureSOCKS5Transport(transport, addr, auth, dialer)
		}
	case "pac":
		if strings.TrimSpace(settings.ProxyPACUrl) != "" {
			scheme, hostPort := ResolvePACProxy(settings.ProxyPACUrl)
			if scheme == "http" && hostPort != "" {
				proxyURL := &url.URL{Scheme: "http", Host: hostPort}
				if settings.ProxyUser != "" {
					proxyURL.User = url.UserPassword(settings.ProxyUser, settings.ProxyPass)
				}
				transport.Proxy = http.ProxyURL(proxyURL)
			} else if scheme == "socks5" && hostPort != "" {
				var auth *proxy.Auth
				if settings.ProxyUser != "" {
					auth = &proxy.Auth{User: settings.ProxyUser, Password: settings.ProxyPass}
				}
				configureSOCKS5Transport(transport, hostPort, auth, dialer)
			}
		}
	}

	return &http.Client{
		Transport: transport,
	}
}

func configureSOCKS5Transport(transport *http.Transport, addr string, auth *proxy.Auth, dialer *net.Dialer) {
	socksDialer, err := proxy.SOCKS5("tcp", addr, auth, dialer)
	if err == nil {
		if cd, ok := socksDialer.(proxy.ContextDialer); ok {
			transport.DialContext = cd.DialContext
		} else {
			transport.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
				return socksDialer.Dial(network, address)
			}
		}
	}
}

// ResolvePACProxy lê um script PAC (via URL HTTP/HTTPS ou caminho local) e extrai a diretiva PROXY ou SOCKS5.
func ResolvePACProxy(pacSource string) (scheme string, hostPort string) {
	pacSource = strings.TrimSpace(pacSource)
	if pacSource == "" {
		return "", ""
	}

	var script string
	if strings.HasPrefix(strings.ToLower(pacSource), "http://") || strings.HasPrefix(strings.ToLower(pacSource), "https://") {
		c := &http.Client{Timeout: 5 * time.Second}
		resp, err := c.Get(pacSource)
		if err != nil {
			return "", ""
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
		if err != nil {
			return "", ""
		}
		script = string(b)
	} else {
		path := strings.TrimPrefix(pacSource, "file://")
		b, err := os.ReadFile(path)
		if err != nil {
			return "", ""
		}
		script = string(b)
	}

	if m := rePACProxy.FindStringSubmatch(script); len(m) == 2 {
		return "http", m[1]
	}
	if m := rePACSocks.FindStringSubmatch(script); len(m) == 2 {
		return "socks5", m[1]
	}
	return "", ""
}

// ApplyRequestHeaders aplica User-Agent compatível com navegadores modernos e credenciais do Site Login (Basic ou Bearer).
func ApplyRequestHeaders(req *http.Request, login *models.SiteLogin) {
	ApplyRequestHeadersWithReferrer(req, login, "")
}

// ApplyRequestHeadersWithReferrer aplica cabeçalhos de navegador, Referer de origem e credenciais de Site Login.
func ApplyRequestHeadersWithReferrer(req *http.Request, login *models.SiteLogin, referrer string) {
	// Define um User-Agent padrão de navegador para evitar bloqueios 403 em CDNs (Cloudflare, S3, GitHub, SourceForge).
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")

	cleanRef := strings.TrimSpace(referrer)
	if cleanRef != "" && (strings.HasPrefix(strings.ToLower(cleanRef), "http://") || strings.HasPrefix(strings.ToLower(cleanRef), "https://")) {
		req.Header.Set("Referer", cleanRef)
	} else if req.URL != nil && (req.URL.Scheme == "http" || req.URL.Scheme == "https") && req.URL.Host != "" {
		req.Header.Set("Referer", fmt.Sprintf("%s://%s/", req.URL.Scheme, req.URL.Host))
	}

	if login != nil && login.Username != "" {
		if login.AuthType == "bearer" {
			req.Header.Set("Authorization", "Bearer "+login.Password)
		} else {
			token := base64.StdEncoding.EncodeToString([]byte(login.Username + ":" + login.Password))
			req.Header.Set("Authorization", "Basic "+token)
		}
	}
}
