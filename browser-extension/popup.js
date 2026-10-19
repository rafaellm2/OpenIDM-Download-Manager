// popup.js — Controla o popup da extensão OpenIDM (PT-BR / EN-US) e aciona o aplicativo desktop nativo.
const statusBadge = document.getElementById("statusBadge");
const toggleCapture = document.getElementById("toggleCapture");
const manualUrl = document.getElementById("manualUrl");
const sendBtn = document.getElementById("sendBtn");
const openAppBtn = document.getElementById("openAppBtn");
const langBtn = document.getElementById("langBtn");
const captureLabel = document.getElementById("captureLabel");
const hintText = document.getElementById("hintText");

const STRINGS = {
  "pt-BR": {
    checking: "Verificando...",
    appActive: "App Ativo no Relógio",
    appClosed: "App Fechado",
    interceptLabel: "Interceptar links de download",
    urlPlaceholder: "Cole uma URL para abrir no OpenIDM...",
    sendBtn: "Capturar e Salvar na Organização",
    openAppBtn: "Abrir Janela do OpenIDM",
    openedInApp: "Aberto no App!",
    sendError: "Erro ao enviar",
    hintHtml: 'Ao clicar em um link, a janela nativa do OpenIDM abrirá perguntando onde salvar na sua organização. Segure <b>ALT</b> para ignorar.',
    langBadge: "PT"
  },
  "en-US": {
    checking: "Checking...",
    appActive: "Active in System Tray",
    appClosed: "App Closed",
    interceptLabel: "Intercept download links",
    urlPlaceholder: "Paste a URL to open in OpenIDM...",
    sendBtn: "Capture & Save in Organization",
    openAppBtn: "Open OpenIDM Window",
    openedInApp: "Opened in App!",
    sendError: "Failed to send",
    hintHtml: 'When clicking a link, the native OpenIDM window will open asking where to save in your organization. Hold <b>ALT</b> to bypass.',
    langBadge: "EN"
  }
};

let currentLang = /^en/i.test(navigator.language || "") ? "en-US" : "pt-BR";
let healthState = "checking"; // "checking" | "ok" | "closed"

function applyLanguage(lang) {
  currentLang = lang === "en-US" ? "en-US" : "pt-BR";
  const t = STRINGS[currentLang];
  document.documentElement.lang = currentLang;
  if (langBtn) langBtn.textContent = t.langBadge;
  if (captureLabel) captureLabel.textContent = t.interceptLabel;
  if (manualUrl) manualUrl.placeholder = t.urlPlaceholder;
  if (sendBtn) sendBtn.textContent = t.sendBtn;
  if (openAppBtn) openAppBtn.textContent = t.openAppBtn;
  if (hintText) hintText.innerHTML = t.hintHtml;

  if (healthState === "ok") {
    statusBadge.textContent = t.appActive;
    statusBadge.style.color = "#4ade80";
  } else if (healthState === "closed") {
    statusBadge.textContent = t.appClosed;
    statusBadge.style.color = "#f87171";
  } else {
    statusBadge.textContent = t.checking;
  }
}

applyLanguage(currentLang);

if (window.location.search.includes("autoreload=1")) {
  if (!sessionStorage.getItem("openidm_reloaded")) {
    sessionStorage.setItem("openidm_reloaded", "1");
    chrome.runtime.reload();
  } else {
    window.close();
  }
} else {
  try {
    chrome.runtime.sendMessage({ type: "MODIFIER_STATE", modifierKey: "" }, (resp) => {
      void chrome.runtime.lastError;
      if (!resp || resp.version !== "1.0.1") {
        chrome.runtime.reload();
      }
    });
  } catch (_e) {}
}

let userToggledLang = false;

chrome.storage.local.get(["captureEnabled", "language"], (res) => {
  if (typeof res.captureEnabled === "boolean") {
    toggleCapture.checked = res.captureEnabled;
  }
  if (!userToggledLang && (res.language === "en-US" || res.language === "pt-BR")) {
    applyLanguage(res.language);
  }
});

if (langBtn) {
  langBtn.addEventListener("click", () => {
    userToggledLang = true;
    const nextLang = currentLang === "en-US" ? "pt-BR" : "en-US";
    applyLanguage(nextLang);
    chrome.storage.local.set({ language: nextLang });
    chrome.runtime.sendMessage({ type: "SET_LANGUAGE", language: nextLang, syncToApp: true });
    fetch("http://127.0.0.1:41820/api/language", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ language: nextLang })
    }).catch(() => {});
  });
}

toggleCapture.addEventListener("change", () => {
  chrome.runtime.sendMessage({
    type: "SET_ENABLED",
    enabled: toggleCapture.checked
  });
});

// Verifica se o aplicativo desktop OpenIDM está ativo na bandeja do Windows e sincroniza o idioma
fetch("http://127.0.0.1:41820/api/health")
  .then((r) => r.json())
  .then((data) => {
    if (data && data.status === "ok") {
      healthState = "ok";
      if (!userToggledLang && (data.language === "en-US" || data.language === "pt-BR")) {
        applyLanguage(data.language);
        chrome.storage.local.set({ language: data.language });
        chrome.runtime.sendMessage({ type: "SET_LANGUAGE", language: data.language, syncToApp: false });
      } else {
        applyLanguage(currentLang);
      }
    }
  })
  .catch(() => {
    healthState = "closed";
    applyLanguage(currentLang);
  });

sendBtn.addEventListener("click", () => {
  const url = manualUrl.value.trim();
  if (!url) return;
  const t = STRINGS[currentLang];
  chrome.runtime.sendMessage({ type: "MANUAL_CAPTURE", url }, (resp) => {
    if (resp && resp.captured) {
      manualUrl.value = "";
      statusBadge.textContent = t.openedInApp;
      window.close();
    } else {
      statusBadge.textContent = t.sendError;
    }
  });
});

openAppBtn.addEventListener("click", () => {
  chrome.runtime.sendMessage({ type: "OPEN_APP" }, () => {
    window.close();
  });
});
