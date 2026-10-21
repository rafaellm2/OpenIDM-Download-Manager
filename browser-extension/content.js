// content.js — Monitora cliques em links de download e reporta ao background do OpenIDM.
(function () {
  if (typeof window.__openidmCleanup === "function") {
    try {
      window.__openidmCleanup();
    } catch (_e) {}
  }

  const CAPTURE_EXT_REGEX =
    /\.(zip|rar|7z|tar|gz|bz2|xz|tgz|zst|exe|msi|iso|img|apk|dmg|deb|rpm|bat|ps1|mp4|mkv|webm|avi|mov|flv|wmv|m4v|mp3|wav|flac|aac|ogg|m4a|wma|opus|pdf|doc|docx|xls|xlsx|ppt|pptx|epub|csv|safetensors|gguf|ckpt|pt|pth|onnx|whl|torrent)(\?|#|$)/i;

  const IGNORED_STREAMING_HOSTS = [
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
    "spotify.com"
  ];

  // Verifica se o host é local (localhost, 127.0.0.1, ::1, LAN) ou plataforma de streaming de vídeo.
  function shouldIgnoreHostOrUrl(rawUrl) {
    try {
      const u = new URL(rawUrl);
      if (/\/api\/test-download\//i.test(u.pathname)) {
        return false;
      }
      const h = (u.hostname || "").toLowerCase();
      if (
        h === "localhost" ||
        h === "127.0.0.1" ||
        h === "::1" ||
        h === "[::1]" ||
        h === "0.0.0.0" ||
        h.endsWith(".localhost") ||
        h.endsWith(".local") ||
        /^127\./.test(h) ||
        /^10\./.test(h) ||
        /^192\.168\./.test(h) ||
        /^172\.(1[6-9]|2\d|3[0-1])\./.test(h)
      ) {
        return true;
      }
      for (const domain of IGNORED_STREAMING_HOSTS) {
        if (h === domain || h.endsWith("." + domain)) {
          return true;
        }
      }
    } catch (_e) {
      return true;
    }
    return false;
  }

  function getModifier(e) {
    if (e.altKey) return "ALT";
    if (e.ctrlKey) return "CTRL";
    if (e.shiftKey) return "SHIFT";
    return "";
  }

  function onMouseDown(e) {
    try {
      chrome.runtime.sendMessage(
        {
          type: "MODIFIER_STATE",
          modifierKey: getModifier(e)
        },
        () => {
          void chrome.runtime.lastError;
        }
      );
    } catch (_err) {
      // Ignora caso o contexto da extensão tenha sido recarregado
    }
  }

  function fallbackProceed(anchor, href) {
    try {
      anchor.dataset.openidmBypass = "1";
      anchor.click();
      setTimeout(() => {
        delete anchor.dataset.openidmBypass;
      }, 300);
    } catch (_err) {
      window.location.assign(href);
    }
  }

  function isRepoViewerPage(href, downloadAttr) {
    if (downloadAttr !== null) return false;
    if (/[?&](download|raw)=(true|1)\b/i.test(href)) return false;
    try {
      const u = new URL(href);
      return /\/(blob|tree|commit|commits|blame)\//i.test(u.pathname);
    } catch (_e) {
      return false;
    }
  }

  function onClick(e) {
    const anchor = e.target && e.target.closest ? e.target.closest("a[href]") : null;
    if (anchor && anchor.dataset && anchor.dataset.openidmBypass === "1") {
      return;
    }

    const modifierKey = getModifier(e);
    try {
      chrome.runtime.sendMessage(
        {
          type: "MODIFIER_STATE",
          modifierKey
        },
        () => {
          void chrome.runtime.lastError;
        }
      );
    } catch (_err) {
      return;
    }

    // Se clicou com botão esquerdo sem teclas de atalho (ALT ignora captura)
    if (e.button !== 0 || e.altKey || e.ctrlKey || e.shiftKey || e.metaKey) {
      return;
    }

    if (!anchor) return;

    const href = anchor.href || "";
    if (!href.startsWith("http://") && !href.startsWith("https://") && !href.startsWith("ftp://")) {
      return;
    }

    if (shouldIgnoreHostOrUrl(href) || (!/\/api\/test-download\//i.test(href) && shouldIgnoreHostOrUrl(window.location.href))) {
      return;
    }

    const downloadAttr = anchor.getAttribute("download");
    if (isRepoViewerPage(href, downloadAttr)) {
      return;
    }

    const isDirectDownloadLink =
      downloadAttr !== null ||
      CAPTURE_EXT_REGEX.test(href) ||
      /[?&](download=(true|1)|dl=1|export=download|response-content-disposition=|rscd=)\b/i.test(href) ||
      /\/(releases\/download|archive\/refs|api\/test-download)\//i.test(href) ||
      /sourceforge\.net\/projects\/[^/]+\/files\/.+\/download/i.test(href) ||
      /mediafire\.com\/file\/[^/]+\/[^/]+\/file/i.test(href);
    if (!isDirectDownloadLink) return;

    // Intercepta o clique para abrir diretamente a tela de organização do OpenIDM sem navegação duplicada.
    e.preventDefault();
    e.stopPropagation();

    let handled = false;
    const fallbackTimer = setTimeout(() => {
      if (!handled) {
        handled = true;
        fallbackProceed(anchor, href);
      }
    }, 1200);

    try {
      chrome.runtime.sendMessage(
        {
          type: "CLICK_CAPTURE",
          url: href,
          fileName: downloadAttr || "",
          referrer: window.location.href,
          modifierKey: ""
        },
        (resp) => {
          void chrome.runtime.lastError;
          if (handled) return;
          if (resp && resp.captured) {
            handled = true;
            clearTimeout(fallbackTimer);
            return;
          }
          // Compatibilidade caso o Service Worker em memória seja anterior ao CLICK_CAPTURE:
          if (typeof resp === "undefined") {
            try {
              chrome.runtime.sendMessage(
                {
                  type: "MANUAL_CAPTURE",
                  url: href,
                  fileName: downloadAttr || "",
                  referrer: window.location.href
                },
                (manualResp) => {
                  void chrome.runtime.lastError;
                  if (handled) return;
                  handled = true;
                  clearTimeout(fallbackTimer);
                  if (!manualResp || !manualResp.captured) {
                    fallbackProceed(anchor, href);
                  }
                }
              );
              return;
            } catch (_err2) {
              handled = true;
              clearTimeout(fallbackTimer);
              fallbackProceed(anchor, href);
              return;
            }
          }
          handled = true;
          clearTimeout(fallbackTimer);
          fallbackProceed(anchor, href);
        }
      );
    } catch (_err) {
      if (!handled) {
        handled = true;
        clearTimeout(fallbackTimer);
        fallbackProceed(anchor, href);
      }
    }
  }

  window.addEventListener("mousedown", onMouseDown, true);
  window.addEventListener("click", onClick, true);
  window.__openidmCleanup = function () {
    window.removeEventListener("mousedown", onMouseDown, true);
    window.removeEventListener("click", onClick, true);
  };
})();

