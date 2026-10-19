// background.js — Intercepta cliques e downloads do navegador e abre diretamente a tela de captura e organização do OpenIDM.
const NATIVE_HOST = "com.rafatech.openidm";
const BRIDGE_URL = "http://127.0.0.1:41820/api/capture";
const HEALTH_URL = "http://127.0.0.1:41820/api/health";
const EXT_VERSION = "1.0.0";

let captureEnabled = true;
let lastModifierKey = "";
let lastModifierTimestamp = 0;
let lastLangSyncAt = 0;
let currentLanguage = (typeof navigator !== "undefined" && /^en/i.test(navigator.language || "")) ? "en-US" : "pt-BR";

chrome.storage.local.get(["captureEnabled", "language"], (res) => {
  if (typeof res.captureEnabled === "boolean") {
    captureEnabled = res.captureEnabled;
  }
  if (res.language === "en-US" || res.language === "pt-BR") {
    updateContextMenuLanguage(res.language);
  }
});

if (chrome.storage && chrome.storage.onChanged) {
  chrome.storage.onChanged.addListener((changes, area) => {
    if (area === "local" && changes.language) {
      const next = changes.language.newValue;
      if ((next === "en-US" || next === "pt-BR") && next !== currentLanguage) {
        updateContextMenuLanguage(next);
      }
    }
  });
}

function getContextMenuTitle(lang) {
  return lang === "en-US"
    ? "Download with OpenIDM (Save in Organization)"
    : "Baixar com o OpenIDM (Salvar na Organização)";
}

function updateContextMenuLanguage(lang) {
  if (lang === "en-US" || lang === "pt-BR") {
    currentLanguage = lang;
    chrome.storage.local.set({ language: lang });
  }
  try {
    chrome.contextMenus.update(
      "openidm-download-link",
      { title: getContextMenuTitle(currentLanguage) },
      () => { void chrome.runtime.lastError; }
    );
  } catch (_err) {}
}

function syncLanguageFromBridge(force) {
  const now = Date.now();
  if (!force && now - lastLangSyncAt < 1500) {
    return;
  }
  lastLangSyncAt = now;
  fetch(HEALTH_URL)
    .then((r) => r.json())
    .then((data) => {
      if (data && (data.language === "en-US" || data.language === "pt-BR")) {
        if (data.language !== currentLanguage) {
          updateContextMenuLanguage(data.language);
        }
      }
    })
    .catch(() => {});
}

function announceExtensionAlive() {
  sendToOpenIDM({
    action: "ping",
    fileName: `chrome-extension://${chrome.runtime.id} (v${EXT_VERSION})`
  }).catch(() => {});
  syncLanguageFromBridge(true);
}

function setupContextMenusAndTabs() {
  announceExtensionAlive();
  try {
    chrome.contextMenus.removeAll(() => {
      void chrome.runtime.lastError;
      chrome.contextMenus.create(
        {
          id: "openidm-download-link",
          title: getContextMenuTitle(currentLanguage),
          contexts: ["link", "video", "audio"]
        },
        () => {
          void chrome.runtime.lastError;
        }
      );
    });
  } catch (_err) {
    // Ignora caso contextMenus não esteja disponível
  }

  // Fecha qualquer aba efêmera de autoreload e injeta content.js nas abas HTTP/HTTPS já abertas.
  if (chrome.tabs) {
    try {
      chrome.tabs.query({}, (allTabs) => {
        if (!allTabs) return;
        for (const tab of allTabs) {
          if (tab.id && tab.url && tab.url.includes("popup.html?autoreload=1")) {
            chrome.tabs.remove(tab.id).catch(() => {});
            continue;
          }
          if (
            chrome.scripting &&
            tab.id &&
            tab.url &&
            (tab.url.startsWith("http://") || tab.url.startsWith("https://"))
          ) {
            chrome.scripting
              .executeScript({
                target: { tabId: tab.id, allFrames: true },
                files: ["content.js"]
              })
              .catch(() => {});
          }
        }
      });
    } catch (_err) {
      // Ignora erros de abas restritas (chrome://, webstore, etc.)
    }
  }
}

setupContextMenusAndTabs();
chrome.runtime.onInstalled.addListener(setupContextMenusAndTabs);
if (chrome.runtime.onStartup) {
  chrome.runtime.onStartup.addListener(setupContextMenusAndTabs);
}

chrome.contextMenus.onClicked.addListener((info, tab) => {
  const targetUrl = info.linkUrl || info.srcUrl;
  if (targetUrl) {
    sendToOpenIDM({
      action: "capture",
      url: targetUrl,
      referrer: tab?.url || "",
      modifierKey: "",
      startNow: true
    });
  }
});

// Extrai apenas o nome base do arquivo a partir de um caminho Windows/Unix.
function sanitizeBaseFileName(rawName) {
  if (!rawName) return "";
  const normalized = String(rawName).trim().replace(/\\/g, "/");
  const base = normalized.split("/").pop() || "";
  return base.trim();
}

// Verifica se o nome possui uma extensão de arquivo real (e não é script web genérico ou número de versão).
function hasValidFileExtension(name) {
  const base = sanitizeBaseFileName(name);
  if (!base || /^v?\d+\.\d+(\.\d+)?$/i.test(base)) return false;
  const m = base.match(/\.([a-z0-9]{2,12})$/i);
  if (!m) return false;
  const ext = m[1].toLowerCase();
  return !["php", "asp", "aspx", "jsp", "cgi", "do", "action", "html", "htm", "mhtml", "xhtml"].includes(ext);
}

// Extrai um nome de arquivo candidato a partir da URL original antes do redirecionamento CDN.
function extractFileNameHint(rawUrl) {
  if (!rawUrl) return "";
  try {
    const u = new URL(rawUrl);
    const rcd =
      u.searchParams.get("response-content-disposition") ||
      u.searchParams.get("rscd") ||
      u.searchParams.get("content-disposition") ||
      "";
    if (rcd) {
      const m = rcd.match(/filename\*?=(?:UTF-8'')?["']?([^"';]+)["']?/i);
      if (m && m[1]) {
        return sanitizeBaseFileName(decodeURIComponent(m[1].trim()));
      }
    }
    for (const key of ["filename", "file", "name"]) {
      const val = u.searchParams.get(key);
      if (val && hasValidFileExtension(val)) {
        return sanitizeBaseFileName(val);
      }
    }
    let cleanPath = u.pathname.replace(/\/+$/, "");
    if (/sourceforge\.net$/i.test(u.hostname) && /\/download$/i.test(cleanPath)) {
      cleanPath = cleanPath.replace(/\/download$/i, "");
    }
    if (/mediafire\.com$/i.test(u.hostname) && /\/file$/i.test(cleanPath)) {
      cleanPath = cleanPath.replace(/\/file$/i, "");
    }
    const seg = cleanPath.split("/").pop() || "";
    const decoded = decodeURIComponent(seg);
    if (hasValidFileExtension(decoded)) {
      return sanitizeBaseFileName(decoded);
    }
  } catch (_err) {
    // Ignora URL inválida
  }
  return "";
}

// Prioriza o nome de arquivo que possua extensão válida (evita que hashes de blobs CDN sobrescrevam o nome real da URL inicial).
function resolveBestFileName(downloadItem) {
  const fromBrowser = sanitizeBaseFileName(downloadItem.filename || "");
  const fromFinalUrl = extractFileNameHint(downloadItem.finalUrl);
  const fromInitialUrl = extractFileNameHint(downloadItem.url);

  if (hasValidFileExtension(fromBrowser)) return fromBrowser;
  if (hasValidFileExtension(fromFinalUrl)) return fromFinalUrl;
  if (hasValidFileExtension(fromInitialUrl)) return fromInitialUrl;
  return fromBrowser || fromFinalUrl || fromInitialUrl || "";
}

// Recebe mensagens do content.js e do popup.js.
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg.type === "MODIFIER_STATE") {
    lastModifierKey = msg.modifierKey || "";
    lastModifierTimestamp = Date.now();
    syncLanguageFromBridge(false);
    sendResponse({ ok: true, version: EXT_VERSION });
    return true;
  }

  if (msg.type === "CLICK_CAPTURE") {
    if (!captureEnabled) {
      sendResponse({ captured: false });
      return true;
    }
    const hintName = sanitizeBaseFileName(msg.fileName) || extractFileNameHint(msg.url);
    // Faz uma única chamada "capture" direta (o servidor valida regras de captura antes de abrir o modal).
    sendToOpenIDM({
      action: "capture",
      url: msg.url,
      fileName: hintName,
      referrer: msg.referrer || "",
      modifierKey: msg.modifierKey || "",
      startNow: true
    }).then((capResp) => {
      sendResponse(capResp || { captured: false });
    });
    return true;
  }

  if (msg.type === "MANUAL_CAPTURE") {
    sendToOpenIDM({
      action: "capture",
      url: msg.url,
      fileName: sanitizeBaseFileName(msg.fileName) || extractFileNameHint(msg.url),
      referrer: msg.referrer || "",
      modifierKey: "",
      startNow: true
    }).then((res) => sendResponse(res));
    return true;
  }

  if (msg.type === "OPEN_APP") {
    sendToOpenIDM({
      action: "open_app"
    }).then((res) => sendResponse(res));
    return true;
  }

  if (msg.type === "SET_ENABLED") {
    captureEnabled = !!msg.enabled;
    chrome.storage.local.set({ captureEnabled });
    sendResponse({ enabled: captureEnabled });
    return true;
  }

  if (msg.type === "SET_LANGUAGE") {
    updateContextMenuLanguage(msg.language);
    if (msg.syncToApp !== false) {
      sendToOpenIDM({
        action: "set_language",
        language: currentLanguage
      }).catch(() => {});
    }
    sendResponse({ language: currentLanguage });
    return true;
  }
});

const pendingDownloads = new Map();
const capturedDownloads = new Map();

// Cancela e remove o download nativo do Chrome aguardando a confirmação antes de liberar onDeterminingFilename.
function cancelAndEraseNativeDownload(downloadId) {
  return new Promise((resolve) => {
    try {
      chrome.downloads.cancel(downloadId, () => {
        void chrome.runtime.lastError;
        chrome.downloads.erase({ id: downloadId }, () => {
          void chrome.runtime.lastError;
          resolve();
        });
      });
      setTimeout(() => {
        try {
          chrome.downloads.erase({ id: downloadId }, () => {
            void chrome.runtime.lastError;
          });
        } catch (_e) {}
        resolve();
      }, 160);
    } catch (_err) {
      resolve();
    }
  });
}

async function evaluateAndCapture(downloadId, url, fileName, referrer, activeModifier) {
  const cleanFile = sanitizeBaseFileName(fileName || "");
  if (capturedDownloads.has(downloadId)) {
    await cancelAndEraseNativeDownload(downloadId);
    const prev = capturedDownloads.get(downloadId);
    if (
      prev &&
      cleanFile &&
      cleanFile.toLowerCase() !== (prev.fileName || "").toLowerCase() &&
      (hasValidFileExtension(cleanFile) || !hasValidFileExtension(prev.fileName))
    ) {
      capturedDownloads.set(downloadId, {
        url: url || prev.url,
        fileName: cleanFile,
        referrer: referrer || prev.referrer || ""
      });
      await sendToOpenIDM({
        action: "capture",
        url: url || prev.url,
        fileName: cleanFile,
        referrer: referrer || prev.referrer || "",
        modifierKey: activeModifier,
        startNow: true
      });
    }
    return true;
  }

  const checkResp = await sendToOpenIDM({
    action: "check",
    url,
    fileName: cleanFile,
    referrer: referrer || "",
    modifierKey: activeModifier
  });

  if (checkResp && checkResp.captured) {
    const existing = capturedDownloads.get(downloadId);
    if (existing) {
      await cancelAndEraseNativeDownload(downloadId);
      if (
        cleanFile &&
        cleanFile.toLowerCase() !== (existing.fileName || "").toLowerCase() &&
        (hasValidFileExtension(cleanFile) || !hasValidFileExtension(existing.fileName))
      ) {
        capturedDownloads.set(downloadId, {
          url: url || existing.url,
          fileName: cleanFile,
          referrer: referrer || existing.referrer || ""
        });
        await sendToOpenIDM({
          action: "capture",
          url: url || existing.url,
          fileName: cleanFile,
          referrer: referrer || existing.referrer || "",
          modifierKey: activeModifier,
          startNow: true
        });
      }
      return true;
    }

    capturedDownloads.set(downloadId, {
      url,
      fileName: cleanFile,
      referrer: referrer || ""
    });
    pendingDownloads.delete(downloadId);

    // Cancela imediatamente o download nativo do navegador (impedindo o diálogo Salvar Como) e delega ao OpenIDM!
    await cancelAndEraseNativeDownload(downloadId);

    await sendToOpenIDM({
      action: "capture",
      url,
      fileName: cleanFile,
      referrer: referrer || "",
      modifierKey: activeModifier,
      startNow: true
    });
    return true;
  }
  return false;
}

// Intercepta o início de qualquer download no navegador (igual ao IDMGCExt.crx).
chrome.downloads.onCreated.addListener(async (downloadItem) => {
  const url = downloadItem.finalUrl || downloadItem.url;
  if (url && url.includes("127.0.0.1:41820/api/extension-wakeup")) {
    await cancelAndEraseNativeDownload(downloadItem.id);
    announceExtensionAlive();
    chrome.storage.local.get(["extReloadedVersion"], (res) => {
      if (res.extReloadedVersion !== EXT_VERSION) {
        chrome.storage.local.set({ extReloadedVersion: EXT_VERSION }, () => {
          chrome.runtime.reload();
        });
      }
    });
    return;
  }

  if (!captureEnabled) return;
  if (downloadItem.state && downloadItem.state !== "in_progress") return;
  if (!url || url.startsWith("blob:") || url.startsWith("data:") || url.startsWith("chrome-extension:")) return;

  // Se o usuário segurou ALT nos últimos 2.5 segundos, respeita o bypass.
  const activeModifier = Date.now() - lastModifierTimestamp < 2500 ? lastModifierKey : "";
  const hintFileName = resolveBestFileName(downloadItem);

  // Registra em pendingDownloads ANTES do await para evitar condição de corrida caso onChanged dispare durante o IPC.
  if (!activeModifier) {
    pendingDownloads.set(downloadItem.id, {
      url,
      fileName: hintFileName,
      referrer: downloadItem.referrer || "",
      activeModifier
    });
  }

  const captured = await evaluateAndCapture(
    downloadItem.id,
    url,
    hintFileName,
    downloadItem.referrer || "",
    activeModifier
  );

  if (captured) {
    pendingDownloads.delete(downloadItem.id);
  }
});

// Intercepta antes da janela nativa "Salvar como" do Chrome abrir quando o nome do arquivo é determinado.
if (chrome.downloads.onDeterminingFilename) {
  chrome.downloads.onDeterminingFilename.addListener((downloadItem, suggest) => {
    const url = downloadItem.finalUrl || downloadItem.url;
    if (url && url.includes("127.0.0.1:41820/api/extension-wakeup")) {
      cancelAndEraseNativeDownload(downloadItem.id).finally(() => {
        try {
          suggest();
        } catch (_err) {}
      });
      return true;
    }
    if (!captureEnabled) {
      suggest();
      return;
    }
    if (!url || url.startsWith("blob:") || url.startsWith("data:") || url.startsWith("chrome-extension:")) {
      suggest();
      return;
    }
    const activeModifier = Date.now() - lastModifierTimestamp < 2500 ? lastModifierKey : "";
    const hintFileName = resolveBestFileName(downloadItem);

    evaluateAndCapture(
      downloadItem.id,
      url,
      hintFileName,
      downloadItem.referrer || "",
      activeModifier
    ).finally(() => {
      try {
        suggest();
      } catch (_err) {}
    });
    return true;
  });
}

// Intercepta downloads cujo nome de arquivo foi resolvido após os cabeçalhos HTTP (Content-Disposition).
chrome.downloads.onChanged.addListener(async (delta) => {
  if (!captureEnabled) return;
  if (capturedDownloads.has(delta.id)) {
    if (delta.filename && delta.filename.current) {
      const prev = capturedDownloads.get(delta.id);
      const nextName = sanitizeBaseFileName(delta.filename.current);
      await evaluateAndCapture(delta.id, prev?.url || "", nextName || prev?.fileName || "", prev?.referrer || "", "");
    }
    if (delta.state && delta.state.current && delta.state.current !== "in_progress") {
      try {
        chrome.downloads.erase({ id: delta.id }, () => {
          void chrome.runtime.lastError;
        });
      } catch (_e) {}
    }
    return;
  }
  if (delta.state && delta.state.current && delta.state.current !== "in_progress") {
    pendingDownloads.delete(delta.id);
    return;
  }
  if (delta.filename && delta.filename.current && pendingDownloads.has(delta.id)) {
    const info = pendingDownloads.get(delta.id);
    pendingDownloads.delete(delta.id);
    const resolvedName = sanitizeBaseFileName(delta.filename.current);
    const bestName = hasValidFileExtension(resolvedName) ? resolvedName : info.fileName || resolvedName;
    await evaluateAndCapture(
      delta.id,
      info.url,
      bestName,
      info.referrer,
      info.activeModifier
    );
  }
});

// Prioriza a ponte IPC HTTP local rápida (127.0.0.1:41820) com timeout de 1200ms e usa Native Messaging para despertar o app caso esteja fechado.
async function sendToOpenIDM(payload) {
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 1200);
    const res = await fetch(BRIDGE_URL, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      signal: controller.signal
    });
    clearTimeout(timer);
    if (res.ok) {
      const data = await res.json();
      if (data && (data.language === "en-US" || data.language === "pt-BR") && data.language !== currentLanguage) {
        updateContextMenuLanguage(data.language);
      }
      if (data && data.success) {
        return data;
      }
    }
  } catch (_err) {
    // Caso o OpenIDM esteja fechado, aciona o Native Messaging Host que inicia openidm.exe automaticamente.
  }

  try {
    const nativeResp = await new Promise((resolve, reject) => {
      if (!chrome.runtime.sendNativeMessage) {
        return reject(new Error("nativeMessaging indisponível"));
      }
      chrome.runtime.sendNativeMessage(NATIVE_HOST, payload, (response) => {
        if (chrome.runtime.lastError) {
          return reject(chrome.runtime.lastError);
        }
        resolve(response);
      });
    });
    if (nativeResp && (nativeResp.language === "en-US" || nativeResp.language === "pt-BR") && nativeResp.language !== currentLanguage) {
      updateContextMenuLanguage(nativeResp.language);
    }
    if (nativeResp && nativeResp.success) {
      return nativeResp;
    }
  } catch (err) {
    return { success: false, captured: false, reason: String(err) };
  }

  return { success: false, captured: false };
}

