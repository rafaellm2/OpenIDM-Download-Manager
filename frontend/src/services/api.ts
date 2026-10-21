import {
  AddDownloadRequest,
  AppSettings,
  CapturePrompt,
  Category,
  Download,
  InstallationStatus,
  InstallOptions,
  InstallResult,
  Queue,
  SiteLogin,
  SiteRule,
  UpdateInfo
} from "../types";

const BRIDGE_BASE = "http://127.0.0.1:41820";

declare global {
  interface Window {
    go?: {
      main?: {
        App?: Record<string, (...args: any[]) => Promise<any>>;
      };
    };
    runtime?: {
      EventsOn?: (eventName: string, callback: (...data: any[]) => void) => () => void;
      EventsOff?: (eventName: string) => void;
      WindowMinimise?: () => void;
      WindowToggleMaximise?: () => void;
      WindowHide?: () => void;
      Quit?: () => void;
    };
  }
}

function getWailsApp() {
  return window.go?.main?.App;
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BRIDGE_BASE}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers || {})
    }
  });
  const data = await res.json();
  if (!res.ok) {
    throw new Error(data?.error || `Erro HTTP ${res.status}`);
  }
  return data as T;
}

// Camada de serviço nativa: prioriza 100% os bindings nativos do Wails v2 (WebView2 no Windows).
export const api = {
  async getDownloads(): Promise<Download[]> {
    const w = getWailsApp();
    if (w?.GetDownloads) return (await w.GetDownloads()) || [];
    return (await fetchJSON<Download[]>("/api/downloads")) || [];
  },

  async getDownload(id: string): Promise<Download> {
    const w = getWailsApp();
    if (w?.GetDownload) return await w.GetDownload(id);
    return await fetchJSON<Download>(`/api/downloads?id=${encodeURIComponent(id)}`);
  },

  async addDownload(req: AddDownloadRequest): Promise<Download> {
    const w = getWailsApp();
    if (w?.AddDownload) return await w.AddDownload(req);
    return await fetchJSON<Download>("/api/downloads", {
      method: "POST",
      body: JSON.stringify(req)
    });
  },

  async startDownload(id: string): Promise<void> {
    const w = getWailsApp();
    if (w?.StartDownload) return await w.StartDownload(id);
    await fetchJSON("/api/downloads/action", {
      method: "POST",
      body: JSON.stringify({ action: "start", id })
    });
  },

  async pauseDownload(id: string): Promise<void> {
    const w = getWailsApp();
    if (w?.PauseDownload) return await w.PauseDownload(id);
    await fetchJSON("/api/downloads/action", {
      method: "POST",
      body: JSON.stringify({ action: "pause", id })
    });
  },

  async pauseAll(): Promise<void> {
    const w = getWailsApp();
    if (w?.PauseAll) return await w.PauseAll();
    await fetchJSON("/api/downloads/action", {
      method: "POST",
      body: JSON.stringify({ action: "pause_all" })
    });
  },

  async cancelDownload(id: string): Promise<void> {
    const w = getWailsApp();
    if (w?.CancelDownload) return await w.CancelDownload(id);
    await fetchJSON("/api/downloads/action", {
      method: "POST",
      body: JSON.stringify({ action: "cancel", id })
    });
  },

  async deleteDownload(id: string, deleteFile: boolean): Promise<void> {
    const w = getWailsApp();
    if (w?.DeleteDownload) return await w.DeleteDownload(id, deleteFile);
    await fetchJSON("/api/downloads/action", {
      method: "POST",
      body: JSON.stringify({ action: "delete", id, deleteFile })
    });
  },

  async deleteCompleted(): Promise<void> {
    const w = getWailsApp();
    if (w?.DeleteCompleted) return await w.DeleteCompleted();
    await fetchJSON("/api/downloads/action", {
      method: "POST",
      body: JSON.stringify({ action: "delete_completed" })
    });
  },

  async setDownloadSpeedLimit(id: string, speedLimitBps: number): Promise<void> {
    const w = getWailsApp();
    if (w?.SetDownloadSpeedLimit) return await w.SetDownloadSpeedLimit(id, speedLimitBps);
    await fetchJSON("/api/downloads/action", {
      method: "POST",
      body: JSON.stringify({ action: "speed_limit", id, speedLimitBps })
    });
  },

  async openFile(id: string): Promise<void> {
    const w = getWailsApp();
    if (w?.OpenFile) return await w.OpenFile(id);
    await fetchJSON("/api/downloads/action", {
      method: "POST",
      body: JSON.stringify({ action: "open_file", id })
    });
  },

  async openFolder(id: string): Promise<void> {
    const w = getWailsApp();
    if (w?.OpenFolder) return await w.OpenFolder(id);
    await fetchJSON("/api/downloads/action", {
      method: "POST",
      body: JSON.stringify({ action: "open_folder", id })
    });
  },

  async getCategories(): Promise<Category[]> {
    const w = getWailsApp();
    if (w?.GetCategories) return (await w.GetCategories()) || [];
    return (await fetchJSON<Category[]>("/api/categories")) || [];
  },

  async saveCategory(c: Category): Promise<void> {
    const w = getWailsApp();
    if (w?.SaveCategory) return await w.SaveCategory(c);
    await fetchJSON("/api/categories", {
      method: "POST",
      body: JSON.stringify(c)
    });
  },

  async getQueues(): Promise<Queue[]> {
    const w = getWailsApp();
    if (w?.GetQueues) return (await w.GetQueues()) || [];
    return (await fetchJSON<Queue[]>("/api/queues")) || [];
  },

  async saveQueue(q: Queue): Promise<void> {
    const w = getWailsApp();
    if (w?.SaveQueue) return await w.SaveQueue(q);
    await fetchJSON("/api/queues", {
      method: "POST",
      body: JSON.stringify(q)
    });
  },

  async deleteQueue(id: string): Promise<void> {
    const w = getWailsApp();
    if (w?.DeleteQueue) return await w.DeleteQueue(id);
    await fetchJSON(`/api/queues?id=${encodeURIComponent(id)}`, {
      method: "DELETE"
    });
  },

  async startQueue(queueId: string): Promise<void> {
    const w = getWailsApp();
    if (w?.StartQueue) return await w.StartQueue(queueId);
    await fetchJSON("/api/queues/action", {
      method: "POST",
      body: JSON.stringify({ action: "start", queueId })
    });
  },

  async stopQueue(queueId: string): Promise<void> {
    const w = getWailsApp();
    if (w?.StopQueue) return await w.StopQueue(queueId);
    await fetchJSON("/api/queues/action", {
      method: "POST",
      body: JSON.stringify({ action: "stop", queueId })
    });
  },

  async cancelSystemShutdown(): Promise<void> {
    const w = getWailsApp();
    if (w?.CancelSystemShutdown) return await w.CancelSystemShutdown();
    await fetchJSON("/api/queues/action", {
      method: "POST",
      body: JSON.stringify({ action: "cancel_shutdown" })
    });
  },

  async getSettings(): Promise<AppSettings> {
    const w = getWailsApp();
    if (w?.GetSettings) return await w.GetSettings();
    return await fetchJSON<AppSettings>("/api/settings");
  },

  async saveSettings(cfg: AppSettings): Promise<void> {
    const w = getWailsApp();
    if (w?.SaveSettings) return await w.SaveSettings(cfg);
    await fetchJSON("/api/settings", {
      method: "POST",
      body: JSON.stringify(cfg)
    });
  },

  async getLanguage(): Promise<"pt-BR" | "en-US"> {
    const w = getWailsApp();
    if (w?.GetLanguage) {
      const lang = await w.GetLanguage();
      return String(lang).toLowerCase().startsWith("en") ? "en-US" : "pt-BR";
    }
    const cfg = await this.getSettings();
    return String(cfg?.language || "").toLowerCase().startsWith("en") ? "en-US" : "pt-BR";
  },

  async setLanguage(lang: "pt-BR" | "en-US"): Promise<"pt-BR" | "en-US"> {
    const w = getWailsApp();
    if (w?.SetLanguage) {
      const updated = await w.SetLanguage(lang);
      return String(updated).toLowerCase().startsWith("en") ? "en-US" : "pt-BR";
    }
    const cfg = await this.getSettings();
    cfg.language = lang;
    await this.saveSettings(cfg);
    return lang;
  },

  async getSiteRules(): Promise<SiteRule[]> {
    const w = getWailsApp();
    if (w?.GetSiteRules) return (await w.GetSiteRules()) || [];
    return (await fetchJSON<SiteRule[]>("/api/site-rules")) || [];
  },

  async saveSiteRule(r: SiteRule): Promise<void> {
    const w = getWailsApp();
    if (w?.SaveSiteRule) return await w.SaveSiteRule(r);
    await fetchJSON("/api/site-rules", {
      method: "POST",
      body: JSON.stringify(r)
    });
  },

  async deleteSiteRule(id: number): Promise<void> {
    const w = getWailsApp();
    if (w?.DeleteSiteRule) return await w.DeleteSiteRule(id);
    await fetchJSON(`/api/site-rules?id=${id}`, { method: "DELETE" });
  },

  async getSiteLogins(): Promise<SiteLogin[]> {
    const w = getWailsApp();
    if (w?.GetSiteLogins) return (await w.GetSiteLogins()) || [];
    return (await fetchJSON<SiteLogin[]>("/api/site-logins")) || [];
  },

  async saveSiteLogin(l: SiteLogin): Promise<void> {
    const w = getWailsApp();
    if (w?.SaveSiteLogin) return await w.SaveSiteLogin(l);
    await fetchJSON("/api/site-logins", {
      method: "POST",
      body: JSON.stringify(l)
    });
  },

  async deleteSiteLogin(id: number): Promise<void> {
    const w = getWailsApp();
    if (w?.DeleteSiteLogin) return await w.DeleteSiteLogin(id);
    await fetchJSON(`/api/site-logins?id=${id}`, { method: "DELETE" });
  },

  async classifyURL(url: string, fileName = ""): Promise<{ fileName: string; categoryId: string; saveDir: string }> {
    const w = getWailsApp();
    if (w?.ClassifyURL) return await w.ClassifyURL(url, fileName);
    return await fetchJSON(
      `/api/classify?url=${encodeURIComponent(url)}&fileName=${encodeURIComponent(fileName)}`
    );
  },

  async probeAndClassifyURL(url: string, fileName = ""): Promise<CapturePrompt> {
    const w = getWailsApp();
    if (w?.ProbeAndClassifyURL) {
      return await w.ProbeAndClassifyURL(url, fileName);
    }
    const cls = await this.classifyURL(url, fileName);
    return {
      url,
      fileName: cls.fileName,
      categoryId: cls.categoryId,
      categoryName: cls.categoryId,
      saveDir: cls.saveDir,
      targetPath: cls.saveDir && cls.fileName ? `${cls.saveDir}\\${cls.fileName}` : cls.saveDir,
      source: "manual",
      totalSize: 0,
      acceptRanges: false,
      probed: false
    };
  },

  async getPendingCapture(): Promise<CapturePrompt | null> {
    const w = getWailsApp();
    if (w?.GetPendingCapture) {
      return (await w.GetPendingCapture()) || null;
    }
    return null;
  },

  async getClipboardURL(): Promise<string> {
    const w = getWailsApp();
    if (w?.GetClipboardURL) {
      return (await w.GetClipboardURL()) || "";
    }
    return "";
  },

  async browseFolder(defaultDir = ""): Promise<string> {
    const w = getWailsApp();
    if (w?.BrowseFolder) {
      return (await w.BrowseFolder(defaultDir)) || "";
    }
    return "";
  },

  async openDirectoryPath(dirPath: string): Promise<void> {
    const w = getWailsApp();
    if (w?.OpenDirectoryPath) {
      await w.OpenDirectoryPath(dirPath);
    }
  },

  async getExistingDownloadFolders(): Promise<Array<{ name: string; path: string }>> {
    const w = getWailsApp();
    if (w?.GetExistingDownloadFolders) {
      return (await w.GetExistingDownloadFolders()) || [];
    }
    return [];
  },

  async ackCaptureModalOpened(url: string, foldersCount: number): Promise<void> {
    const w = getWailsApp();
    if (w?.AckCaptureModalOpened) {
      await w.AckCaptureModalOpened(url, foldersCount);
    }
  },

  async reportFrontendError(errMsg: string): Promise<void> {
    const w = getWailsApp();
    if (w?.ReportFrontendError) {
      await w.ReportFrontendError(errMsg);
    }
  },

  async hideToTray(): Promise<void> {
    const w = getWailsApp();
    if (w?.HideToTray) {
      await w.HideToTray();
    }
  },

  async showWindow(): Promise<void> {
    const w = getWailsApp();
    if (w?.ShowWindow) {
      await w.ShowWindow();
    }
  },

  async getWindowMode(): Promise<"card" | "full"> {
    const w = getWailsApp();
    if (w?.GetWindowMode) {
      const mode = await w.GetWindowMode();
      return mode === "card" ? "card" : "full";
    }
    return "full";
  },

  async setWindowMode(mode: "card" | "full"): Promise<"card" | "full"> {
    const w = getWailsApp();
    if (w?.SetWindowMode) {
      const updated = await w.SetWindowMode(mode);
      return updated === "card" ? "card" : "full";
    }
    return mode;
  },

  async showStandaloneCard(): Promise<void> {
    const w = getWailsApp();
    if (w?.ShowStandaloneCard) {
      await w.ShowStandaloneCard();
    }
  },

  async resizeStandaloneCard(width: number, height: number): Promise<void> {
    const w = getWailsApp();
    if (w?.ResizeStandaloneCard) {
      await w.ResizeStandaloneCard(Math.round(width), Math.round(height));
    }
  },

  async saveCardPosition(x: number, y: number): Promise<void> {
    const w = getWailsApp();
    if (w?.SaveCardPosition) {
      await w.SaveCardPosition(Math.round(x), Math.round(y));
    }
  },

  async getCardPosition(): Promise<{ x: number; y: number; saved: boolean }> {
    const w = getWailsApp();
    if (w?.GetCardPosition) {
      const res = await w.GetCardPosition();
      return {
        x: Number(res?.x || 0),
        y: Number(res?.y || 0),
        saved: Boolean(res?.saved)
      };
    }
    return { x: 0, y: 0, saved: false };
  },

  async openFullAppFromCard(): Promise<void> {
    const w = getWailsApp();
    if (w?.OpenFullAppFromCard) {
      await w.OpenFullAppFromCard();
    } else if (w?.ShowWindow) {
      await w.ShowWindow();
    }
  },

  async minimizeToCard(): Promise<void> {
    const w = getWailsApp();
    if (w?.MinimizeToCard) {
      await w.MinimizeToCard();
    }
  },

  async closeStandaloneCard(): Promise<void> {
    const w = getWailsApp();
    if (w?.CloseStandaloneCard) {
      await w.CloseStandaloneCard();
    } else if (w?.HideToTray) {
      await w.HideToTray();
    }
  },

  async getCardDownloads(): Promise<Download[]> {
    const w = getWailsApp();
    if (w?.GetCardDownloads) {
      return (await w.GetCardDownloads()) || [];
    }
    return [];
  },

  async dismissCardDownload(id: string): Promise<void> {
    const w = getWailsApp();
    if (w?.DismissCardDownload) {
      await w.DismissCardDownload(id);
    }
  },

  async clearCompletedFromCard(): Promise<void> {
    const w = getWailsApp();
    if (w?.ClearCompletedFromCard) {
      await w.ClearCompletedFromCard();
    }
  },

  async getPendingCaptureQueue(): Promise<CapturePrompt[]> {
    const w = getWailsApp();
    if (w?.GetPendingCaptureQueue) {
      return (await w.GetPendingCaptureQueue()) || [];
    }
    return [];
  },

  async removePendingCaptureByURL(url: string): Promise<void> {
    const w = getWailsApp();
    if (w?.RemovePendingCaptureByURL) {
      await w.RemovePendingCaptureByURL(url);
    }
  },

  async quitApp(): Promise<void> {
    const w = getWailsApp();
    if (w?.QuitApp) {
      await w.QuitApp();
    }
  },

  async installBrowserIntegration(chromeExtID: string): Promise<string> {
    const w = getWailsApp();
    if (w?.InstallBrowserIntegration) return await w.InstallBrowserIntegration(chromeExtID);
    return "Manifesto registrado via Native Host CLI";
  },

  async shouldOpenInstaller(): Promise<boolean> {
    const w = getWailsApp();
    if (w?.ShouldOpenInstaller) {
      return !!(await w.ShouldOpenInstaller());
    }
    return false;
  },

  async getInstallationStatus(): Promise<InstallationStatus> {
    const w = getWailsApp();
    if (w?.GetInstallationStatus) {
      return await w.GetInstallationStatus();
    }
    return await fetchJSON<InstallationStatus>("/api/installer/status");
  },

  async runSystemInstaller(opts: InstallOptions): Promise<InstallResult> {
    const w = getWailsApp();
    if (w?.RunSystemInstaller) {
      return await w.RunSystemInstaller(opts);
    }
    return await fetchJSON<InstallResult>("/api/installer/install", {
      method: "POST",
      body: JSON.stringify(opts)
    });
  },

  async launchBrowserExtensionSetup(): Promise<void> {
    const w = getWailsApp();
    if (w?.LaunchBrowserExtensionSetup) {
      await w.LaunchBrowserExtensionSetup();
      return;
    }
    await fetchJSON("/api/installer/launch-extension", { method: "POST" });
  },

  async openExtensionFolder(): Promise<string> {
    const w = getWailsApp();
    if (w?.OpenExtensionFolder) {
      return (await w.OpenExtensionFolder()) || "";
    }
    const res = await fetchJSON<{ extensionDir: string }>("/api/installer/open-extension-folder", {
      method: "POST"
    });
    return res?.extensionDir || "";
  },

  async checkForUpdates(manual = false): Promise<UpdateInfo> {
    const w = getWailsApp();
    if (w?.CheckForUpdates) {
      return await w.CheckForUpdates(manual);
    }
    return await fetchJSON<UpdateInfo>(`/api/update/check?manual=${manual ? "true" : "false"}`);
  },

  async postponeUpdate(): Promise<number> {
    const w = getWailsApp();
    if (w?.PostponeUpdate) {
      return Number(await w.PostponeUpdate());
    }
    const res = await fetchJSON<{ updatePostponedUntil: number }>("/api/update/postpone", {
      method: "POST"
    });
    return Number(res?.updatePostponedUntil || 0);
  },

  async startSelfUpdate(
    downloadUrl: string,
    assetName: string
  ): Promise<{ status: string; filePath?: string; releaseUrl?: string }> {
    const w = getWailsApp();
    if (w?.StartSelfUpdate) {
      return await w.StartSelfUpdate(downloadUrl, assetName);
    }
    return await fetchJSON<{ status: string; filePath?: string; releaseUrl?: string }>(
      "/api/update/apply",
      {
        method: "POST",
        body: JSON.stringify({ downloadUrl, assetName })
      }
    );
  },

  async openExternalURL(url: string): Promise<void> {
    const w = getWailsApp();
    if (w?.OpenExternalURL) {
      await w.OpenExternalURL(url);
      return;
    }
    window.open(url, "_blank", "noopener,noreferrer");
  },

  subscribeEvents(onEvent: (ev: { type: string; payload: any }) => void): () => void {
    // 1. Eventos nativos do runtime Wails v2 (WebView2 no Windows)
    if (window.runtime?.EventsOn) {
      const off = window.runtime.EventsOn("openidm:event", (ev: any) => {
        if (ev && ev.type) {
          onEvent(ev);
        }
      });
      if (typeof off === "function") {
        return off;
      }
      return () => window.runtime?.EventsOff?.("openidm:event");
    }

    // 2. Fallback apenas caso o runtime Wails não esteja presente (ex: ambiente de testes)
    try {
      const es = new EventSource(`${BRIDGE_BASE}/api/events`);
      es.onmessage = (e) => {
        try {
          const parsed = JSON.parse(e.data);
          onEvent(parsed);
        } catch (_err) {
          // Ignora mensagens malformadas
        }
      };
      return () => es.close();
    } catch (_err) {
      return () => {};
    }
  }
};
