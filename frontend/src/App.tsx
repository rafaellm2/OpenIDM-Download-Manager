import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AddUrlModal } from "./components/AddUrlModal";
import { DownloadDetailsModal } from "./components/DownloadDetailsModal";
import { DownloadTable } from "./components/DownloadTable";
import { InstallerModal } from "./components/InstallerModal";
import { SchedulerModal } from "./components/SchedulerModal";
import { SettingsModal } from "./components/SettingsModal";
import { Sidebar, SidebarFilter } from "./components/Sidebar";
import { StandaloneProgressCard } from "./components/StandaloneProgressCard";
import { Toolbar } from "./components/Toolbar";
import {
  getActiveLanguage,
  LanguageProvider,
  setActiveLanguageInMemory,
  useI18n
} from "./i18n";
import { api } from "./services/api";
import {
  AddDownloadRequest,
  AppLanguage,
  AppSettings,
  CapturePrompt,
  Category,
  Download,
  Queue,
  SiteLogin,
  SiteRule
} from "./types";

const AppContent: React.FC<{
  onBackendLanguageLoaded: (lang: AppLanguage) => void;
}> = ({ onBackendLanguageLoaded }) => {
  const { t } = useI18n();
  const [downloads, setDownloads] = useState<Download[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [queues, setQueues] = useState<Queue[]>([]);
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const [siteRules, setSiteRules] = useState<SiteRule[]>([]);
  const [siteLogins, setSiteLogins] = useState<SiteLogin[]>([]);

  // Telas separadas: "full" (App Completo preservado) vs "card" (Card Solto independente)
  const [windowMode, setWindowMode] = useState<"card" | "full">("full");
  // No modo "card": "list" exibe apenas o Card Reduzido de andamento; "save" exibe apenas a tela de salvar novo download
  const [cardView, setCardView] = useState<"list" | "save">("list");

  const [cardSessionIds, setCardSessionIds] = useState<string[]>([]);
  const [dismissedCardIds, setDismissedCardIds] = useState<Record<string, boolean>>({});
  const [pendingCaptures, setPendingCaptures] = useState<CapturePrompt[]>([]);

  // Rastreia IDs recém-excluídos para evitar que eventos em trânsito reinsiram o item na lista
  const deletedIdsRef = useRef<Set<string>>(new Set());
  // Cache de velocidade para manter a velocidade total do Toolbar estável sem piscar para zero
  const speedCacheRef = useRef<Record<string, { bps: number; ts: number }>>({});

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detailsId, setDetailsId] = useState<string | null>(null);
  const [filter, setFilter] = useState<SidebarFilter>({ type: "all", value: "all" });
  const [searchQuery, setSearchQuery] = useState("");

  // Modais do App Completo e captura de links
  const [showAddModalInFullApp, setShowAddModalInFullApp] = useState(false);
  const [prefillUrl, setPrefillUrl] = useState("");
  const [capturePrompt, setCapturePrompt] = useState<CapturePrompt | null>(null);
  const [showScheduler, setShowScheduler] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [showInstaller, setShowInstaller] = useState(false);

  // Insere ou atualiza um link capturado na fila de captura
  const enqueuePrompt = useCallback((prompt: CapturePrompt) => {
    if (!prompt?.url?.trim()) return;
    const clean = prompt.url.trim().toLowerCase();
    setPendingCaptures((prev) => {
      const idx = prev.findIndex((item) => item.url.trim().toLowerCase() === clean);
      if (idx !== -1) {
        const copy = [...prev];
        copy[idx] = prompt;
        return copy;
      }
      return [...prev, prompt];
    });
  }, []);

  // Vincula um download à lista do Card Reduzido
  const trackDownloadInCard = useCallback((id: string, unDismiss = false) => {
    if (!id || deletedIdsRef.current.has(id)) return;
    if (unDismiss) {
      setDismissedCardIds((prev) => {
        if (!prev[id]) return prev;
        const copy = { ...prev };
        delete copy[id];
        return copy;
      });
    }
    setCardSessionIds((prev) => (prev.includes(id) ? prev : [id, ...prev]));
  }, []);

  const loadAll = useCallback(async () => {
    try {
      const [dls, cats, qs, cfg, rules, logins, cardDls] = await Promise.all([
        api.getDownloads(),
        api.getCategories(),
        api.getQueues(),
        api.getSettings(),
        api.getSiteRules(),
        api.getSiteLogins(),
        api.getCardDownloads()
      ]);
      const visibleDls = dls.filter((d) => !deletedIdsRef.current.has(d.id));
      setDownloads(visibleDls);
      setCategories(cats);
      setQueues(qs);
      setSettings(cfg);
      if (cfg?.language) {
        const norm = setActiveLanguageInMemory(cfg.language);
        onBackendLanguageLoaded(norm);
      }
      setSiteRules(rules);
      setSiteLogins(logins);
      if (cardDls.length > 0) {
        setCardSessionIds((prev) => {
          const merged = [...prev];
          for (const item of cardDls) {
            if (!deletedIdsRef.current.has(item.id) && !merged.includes(item.id)) {
              merged.push(item.id);
            }
          }
          return merged;
        });
      }
    } catch (_err) {
      // Silencioso durante inicialização
    }
  }, [onBackendLanguageLoaded]);

  useEffect(() => {
    loadAll();

    api
      .getWindowMode()
      .then((mode) => {
        if (mode === "card") {
          setWindowMode("card");
        } else {
          setWindowMode("full");
        }
      })
      .catch(() => {});

    api
      .shouldOpenInstaller()
      .then((shouldOpen) => {
        if (shouldOpen) {
          setWindowMode("full");
          setShowInstaller(true);
        }
      })
      .catch(() => {});

    const checkPendingCapture = () => {
      api
        .getPendingCaptureQueue()
        .then((queue) => {
          if (Array.isArray(queue)) {
            setPendingCaptures(queue);
          }
        })
        .catch(() => {});

      Promise.all([api.getWindowMode(), api.getPendingCapture()])
        .then(([mode, pending]) => {
          if (pending) {
            enqueuePrompt(pending);
            setCapturePrompt((prev) => prev || pending);
            setPrefillUrl((prev) => prev || pending.url || "");
            if (mode === "card") {
              setCardView("save");
              setWindowMode("card");
            }
          }
        })
        .catch(() => {});
    };

    checkPendingCapture();

    const unsub = api.subscribeEvents((ev) => {
      if (ev.type === "download_update" && ev.payload) {
        const updated = ev.payload as Download;
        if (deletedIdsRef.current.has(updated.id)) {
          return;
        }
        if (
          updated.status === "downloading" ||
          updated.status === "probing" ||
          updated.status === "assembling"
        ) {
          trackDownloadInCard(updated.id, false);
        }
        setDownloads((prev) => {
          const idx = prev.findIndex((item) => item.id === updated.id);
          if (idx === -1) return [updated, ...prev];
          const copy = [...prev];
          copy[idx] = updated;
          return copy;
        });
      } else if (ev.type === "download_deleted" && ev.payload?.id) {
        const delId = String(ev.payload.id);
        deletedIdsRef.current.add(delId);
        delete speedCacheRef.current[delId];
        setDownloads((prev) => prev.filter((item) => item.id !== delId));
        setCardSessionIds((prev) => prev.filter((id) => id !== delId));
      } else if (ev.type === "card_download_added" && ev.payload) {
        const added = ev.payload as Download;
        deletedIdsRef.current.delete(added.id);
        trackDownloadInCard(added.id, true);
      } else if (ev.type === "window_mode" && ev.payload?.mode) {
        if (ev.payload.mode === "card") {
          setWindowMode("card");
        } else {
          setWindowMode("full");
        }
      } else if (ev.type === "language_changed" && ev.payload?.language) {
        const norm = setActiveLanguageInMemory(String(ev.payload.language));
        onBackendLanguageLoaded(norm);
        setSettings((prev) => (prev ? { ...prev, language: norm } : prev));
      } else if (ev.type === "capture_queue_updated" && Array.isArray(ev.payload)) {
        const queue = ev.payload as CapturePrompt[];
        setPendingCaptures(queue);
        setCapturePrompt((prev) => {
          if (!prev) return queue[0] || null;
          const stillExists = queue.find(
            (item) => item.url.trim().toLowerCase() === prev.url.trim().toLowerCase()
          );
          if (stillExists) return stillExists;
          return queue[0] || null;
        });
      } else if (ev.type === "capture_prompt" && ev.payload) {
        const prompt = ev.payload as CapturePrompt;
        api.getPendingCapture().catch(() => {});
        enqueuePrompt(prompt);
        setCapturePrompt((prev) => {
          if (
            !prev ||
            prev.url.trim().toLowerCase() === prompt.url.trim().toLowerCase() ||
            (prev.fileName && prev.fileName === prompt.fileName && prev.referrer === prompt.referrer)
          ) {
            setPrefillUrl(prompt.url || "");
            return prompt;
          }
          return prev;
        });
        setCardView("save");
        setWindowMode("card");
      } else if (ev.type === "clipboard_url" && ev.payload?.url) {
        setCapturePrompt(null);
        setPrefillUrl(ev.payload.url);
        setCardView("save");
        setWindowMode("card");
        api.showStandaloneCard().catch(() => {});
      } else if (ev.type === "captured_url" && ev.payload) {
        const dl = ev.payload as Download;
        if (dl?.id) {
          deletedIdsRef.current.delete(dl.id);
          trackDownloadInCard(dl.id, true);
        }
        loadAll();
      } else if (ev.type === "open_installer") {
        setWindowMode("full");
        setShowInstaller(true);
      }
    });

    // Atalho global Ctrl+V para capturar link
    const onPaste = (e: ClipboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT")
      ) {
        return;
      }
      const txt = e.clipboardData?.getData("text/plain")?.trim() || "";
      if (
        txt.startsWith("http://") ||
        txt.startsWith("https://") ||
        txt.startsWith("ftp://")
      ) {
        setCapturePrompt(null);
        setPrefillUrl(txt);
        if (windowMode === "full") {
          setShowAddModalInFullApp(true);
        } else {
          setCardView("save");
          setWindowMode("card");
          api.showStandaloneCard().catch(() => {});
        }
      }
    };
    const onError = (e: ErrorEvent) => {
      api.reportFrontendError(String(e.error?.stack || e.message || e)).catch(() => {});
    };
    const onRejection = (e: PromiseRejectionEvent) => {
      api.reportFrontendError(String(e.reason?.stack || e.reason || e)).catch(() => {});
    };

    window.addEventListener("paste", onPaste);
    window.addEventListener("focus", checkPendingCapture);
    window.addEventListener("error", onError);
    window.addEventListener("unhandledrejection", onRejection);
    document.addEventListener("visibilitychange", checkPendingCapture);

    const poll = setInterval(() => {
      checkPendingCapture();
      api
        .getDownloads()
        .then((dls) => setDownloads(dls.filter((d) => !deletedIdsRef.current.has(d.id))))
        .catch(() => {});
      api.getQueues().then(setQueues).catch(() => {});
    }, 1500);

    return () => {
      unsub();
      window.removeEventListener("paste", onPaste);
      window.removeEventListener("focus", checkPendingCapture);
      window.removeEventListener("error", onError);
      window.removeEventListener("unhandledrejection", onRejection);
      document.removeEventListener("visibilitychange", checkPendingCapture);
      clearInterval(poll);
    };
  }, [loadAll, enqueuePrompt, trackDownloadInCard, windowMode, onBackendLanguageLoaded]);

  // Lista consolidada de downloads acompanhados no Card Reduzido
  const cardDownloads = useMemo(() => {
    const byId = new Map<string, Download>();
    for (const d of downloads) {
      byId.set(d.id, d);
    }

    const result: Download[] = [];
    const seen = new Set<string>();

    // 1. Mantém na ordem os downloads iniciados ou acompanhados neste card
    for (const id of cardSessionIds) {
      if (dismissedCardIds[id] || seen.has(id)) continue;
      const found = byId.get(id);
      if (found) {
        seen.add(id);
        result.push(found);
      }
    }

    // 2. Garante que qualquer download ativo ou na fila também apareça na lista do card
    for (const d of downloads) {
      if (dismissedCardIds[d.id] || seen.has(d.id)) continue;
      if (
        d.status === "downloading" ||
        d.status === "probing" ||
        d.status === "assembling" ||
        d.status === "queued"
      ) {
        seen.add(d.id);
        result.push(d);
      }
    }

    return result;
  }, [downloads, cardSessionIds, dismissedCardIds]);

  // Filtra a lista exibida conforme seleção na Sidebar e busca na tela completa
  const filteredDownloads = useMemo(() => {
    return downloads.filter((d) => {
      if (filter.type === "category" && d.categoryId !== filter.value) return false;
      if (filter.type === "queue" && d.queueId !== filter.value) return false;
      if (filter.type === "status") {
        if (filter.value === "completed" && d.status !== "completed") return false;
        if (filter.value === "unfinished" && d.status === "completed") return false;
      }
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        return (
          d.fileName.toLowerCase().includes(q) ||
          d.url.toLowerCase().includes(q) ||
          (d.description || "").toLowerCase().includes(q)
        );
      }
      return true;
    });
  }, [downloads, filter, searchQuery]);

  const selectedDownload = useMemo(
    () => downloads.find((d) => d.id === selectedId) || null,
    [downloads, selectedId]
  );

  const detailsDownload = useMemo(
    () => downloads.find((d) => d.id === detailsId) || null,
    [downloads, detailsId]
  );

  const totalSpeedBps = useMemo(() => {
    const now = Date.now();
    return downloads.reduce((acc, d) => {
      const isRunning =
        d.status === "downloading" ||
        d.status === "probing" ||
        d.status === "assembling";
      if (!isRunning) {
        delete speedCacheRef.current[d.id];
        return acc;
      }
      if (d.speedBps && d.speedBps > 0) {
        speedCacheRef.current[d.id] = { bps: d.speedBps, ts: now };
        return acc + d.speedBps;
      }
      const cached = speedCacheRef.current[d.id];
      if (cached && now - cached.ts < 2200) {
        return acc + cached.bps;
      }
      return acc;
    }, 0);
  }, [downloads]);

  const activeCount = useMemo(
    () =>
      downloads.filter(
        (d) =>
          d.status === "downloading" ||
          d.status === "probing" ||
          d.status === "assembling"
      ).length,
    [downloads]
  );

  // Adiciona novo download e após salvar transiciona imediatamente para o Card Reduzido de acompanhamento
  const handleAddDownload = async (req: AddDownloadRequest) => {
    const created = await api.addDownload(req);
    deletedIdsRef.current.delete(created.id);
    trackDownloadInCard(created.id, true);

    const submittedClean = req.url.trim().toLowerCase();
    const currentPromptClean = (capturePrompt?.url || "").trim().toLowerCase();
    const submittedFileClean = (req.fileName || created.fileName || "").trim().toLowerCase();

    if (currentPromptClean && currentPromptClean !== submittedClean) {
      api.removePendingCaptureByURL(capturePrompt!.url).catch(() => {});
    }

    setPendingCaptures((prev) => {
      const remaining = prev.filter((item) => {
        const u = item.url.trim().toLowerCase();
        const f = (item.fileName || "").trim().toLowerCase();
        if (u === submittedClean || u === currentPromptClean) {
          return false;
        }
        if (submittedFileClean && f === submittedFileClean) {
          api.removePendingCaptureByURL(item.url).catch(() => {});
          return false;
        }
        return true;
      });
      return remaining;
    });

    // Após clicar em Baixar Agora ou Baixar Depois, exibe o Card Reduzido de Andamento (lista limpa)
    setCapturePrompt(null);
    setPrefillUrl("");
    setCardView("list");

    await loadAll();
    setSelectedId(created.id);
    return created;
  };

  const handleDismissPendingCapture = useCallback((rawUrl: string) => {
    const clean = rawUrl.trim().toLowerCase();
    api.removePendingCaptureByURL(rawUrl).catch(() => {});
    setPendingCaptures((prev) => {
      const remaining = prev.filter((item) => item.url.trim().toLowerCase() !== clean);
      if (remaining.length === 0) {
        setCapturePrompt(null);
        setPrefillUrl("");
        setCardView("list");
      } else {
        setCapturePrompt((cur) => {
          if (cur && cur.url.trim().toLowerCase() === clean) {
            setPrefillUrl(remaining[0].url || "");
            return remaining[0];
          }
          return cur;
        });
      }
      return remaining;
    });
  }, []);

  const handleDismissCardItem = useCallback((id: string) => {
    api.dismissCardDownload(id).catch(() => {});
    setDismissedCardIds((prev) => ({ ...prev, [id]: true }));
    setCardSessionIds((prev) => prev.filter((cur) => cur !== id));
  }, []);

  // Exclui diretamente um download (para o download ativo imediatamente e o remove da lista/disco sem piscar)
  const handleDeleteDownloadById = useCallback(
    async (id: string) => {
      if (!id) return;
      deletedIdsRef.current.add(id);
      delete speedCacheRef.current[id];
      setDownloads((prev) => prev.filter((d) => d.id !== id));
      handleDismissCardItem(id);
      if (detailsId === id) setDetailsId(null);
      if (selectedId === id) setSelectedId(null);
      await api.deleteDownload(id, true);
      await loadAll();
    },
    [detailsId, selectedId, handleDismissCardItem, loadAll]
  );

  const handleClearCompletedFromCard = useCallback(() => {
    api.clearCompletedFromCard().catch(() => {});
    setDismissedCardIds((prev) => {
      const next = { ...prev };
      for (const d of cardDownloads) {
        if (d.status === "completed" || d.status === "cancelled") {
          next[d.id] = true;
        }
      }
      return next;
    });
    setCardSessionIds((prev) =>
      prev.filter((id) => {
        const found = downloads.find((d) => d.id === id);
        return found && found.status !== "completed" && found.status !== "cancelled";
      })
    );
  }, [cardDownloads, downloads]);

  // Abre o Card Reduzido mostrando apenas a lista de downloads em andamento
  const handleSwitchToProgressCard = useCallback(() => {
    setShowAddModalInFullApp(false);
    setCapturePrompt(null);
    setPrefillUrl("");
    setCardView("list");
    setWindowMode("card");
    api.showStandaloneCard().catch(() => {});
  }, []);

  // Abre o App Completo preservado
  const handleOpenFullApp = useCallback(() => {
    setShowAddModalInFullApp(false);
    setCapturePrompt(null);
    setPrefillUrl("");
    setCardView("list");
    setWindowMode("full");
    api.openFullAppFromCard().catch(() => {});
  }, []);

  const handleCloseStandaloneCard = useCallback(() => {
    setCapturePrompt(null);
    setPrefillUrl("");
    setCardView("list");
    api.closeStandaloneCard().catch(() => {});
  }, []);

  const handleDeleteSelected = async () => {
    if (!selectedId) return;
    await handleDeleteDownloadById(selectedId);
  };

  // MODO CARD SOLTO: duas telas estritamente separadas do App Completo
  // 1) "save": exibida SOMENTE na hora de escolher onde salvar/organizar um novo download
  // 2) "list": exibida após clicar em Baixar Agora / Baixar Depois, mostrando apenas a lista compacta de downloads
  if (windowMode === "card") {
    const shouldShowSaveScreen = cardView === "save";

    if (shouldShowSaveScreen) {
      return (
        <AddUrlModal
          standalone={true}
          initialUrl={prefillUrl}
          initialCapture={capturePrompt}
          categories={categories}
          queues={queues}
          defaultMaxConnections={settings?.maxConnectionsPerDownload || 8}
          cardDownloads={cardDownloads}
          pendingCaptures={pendingCaptures}
          onClose={handleCloseStandaloneCard}
          onSavedToCard={() => {
            setCapturePrompt(null);
            setPrefillUrl("");
            setCardView("list");
          }}
          onSubmit={handleAddDownload}
          onCategoryUpdated={loadAll}
          onSelectPendingCapture={(prompt) => {
            setCapturePrompt(prompt);
            setPrefillUrl(prompt.url || "");
          }}
          onDismissPendingCapture={handleDismissPendingCapture}
          onOpenFullApp={handleOpenFullApp}
        />
      );
    }

    return (
      <StandaloneProgressCard
        downloads={cardDownloads}
        pendingCapturesCount={pendingCaptures.length}
        onOpenNewSavePrompt={() => {
          setCapturePrompt(null);
          setPrefillUrl("");
          setCardView("save");
        }}
        onOpenPendingCaptures={() => {
          const next = pendingCaptures[0] || null;
          setCapturePrompt(next);
          setPrefillUrl(next?.url || "");
          setCardView("save");
        }}
        onOpenFullApp={handleOpenFullApp}
        onClose={handleCloseStandaloneCard}
        onStartDownload={(id) => api.startDownload(id)}
        onPauseDownload={(id) => api.pauseDownload(id)}
        onPauseAll={() => api.pauseAll()}
        onCancelDownload={(id) => api.cancelDownload(id)}
        onDeleteDownload={handleDeleteDownloadById}
        onOpenFile={(id) => api.openFile(id)}
        onOpenFolder={(id) => api.openFolder(id)}
        onDismissCardItem={handleDismissCardItem}
        onClearCompletedCard={handleClearCompletedFromCard}
      />
    );
  }

  // MODO APP COMPLETO PRESERVADO (aberto ao clicar no ícone do relógio)
  return (
    <div className="app-shell">
      <Toolbar
        selectedId={selectedId}
        canResume={
          !!selectedDownload &&
          selectedDownload.status !== "downloading" &&
          selectedDownload.status !== "probing" &&
          selectedDownload.status !== "completed"
        }
        canPause={
          !!selectedDownload &&
          (selectedDownload.status === "downloading" || selectedDownload.status === "probing")
        }
        totalSpeedBps={totalSpeedBps}
        activeDownloadsCount={activeCount}
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        onOpenAddModal={() => {
          setPrefillUrl("");
          setShowAddModalInFullApp(true);
        }}
        onSwitchToCardMode={handleSwitchToProgressCard}
        onResumeSelected={() => selectedId && api.startDownload(selectedId)}
        onPauseSelected={() => selectedId && api.pauseDownload(selectedId)}
        onPauseAll={() => api.pauseAll()}
        onDeleteSelected={handleDeleteSelected}
        onDeleteCompleted={async () => {
          await api.deleteCompleted();
          await loadAll();
        }}
        onOpenScheduler={() => setShowScheduler(true)}
        onStartMainQueue={async () => {
          await api.startQueue("main");
          await loadAll();
        }}
        onStopMainQueue={async () => {
          await api.stopQueue("main");
          await loadAll();
        }}
        onOpenSettings={() => setShowSettings(true)}
        onOpenInstaller={() => setShowInstaller(true)}
        onHideToTray={() => {
          api.hideToTray();
        }}
      />

      <div className="main-workspace">
        <Sidebar
          categories={categories}
          queues={queues}
          downloads={downloads}
          filter={filter}
          onSelectFilter={setFilter}
          onToggleQueue={async (q) => {
            if (q.active) {
              await api.stopQueue(q.id);
            } else {
              await api.startQueue(q.id);
            }
            await loadAll();
          }}
          onOpenCategoryFolder={(cat) => api.openDirectoryPath(cat.defaultDir)}
        />

        <main className="content-area">
          <DownloadTable
            downloads={filteredDownloads}
            categories={categories}
            queues={queues}
            selectedId={selectedId}
            onSelect={setSelectedId}
            onOpenDetails={(id) => setDetailsId(id)}
            onStart={(id) => api.startDownload(id)}
            onPause={(id) => api.pauseDownload(id)}
            onDelete={handleDeleteDownloadById}
            onOpenFolder={(id) => api.openFolder(id)}
            onOpenFile={(id) => api.openFile(id)}
            onDropURL={(droppedUrl) => {
              setPrefillUrl(droppedUrl);
              setShowAddModalInFullApp(true);
            }}
          />
        </main>
      </div>

      <footer className="statusbar">
        <div className="statusbar-left">
          <span className="tray-indicator-dot" />
          <span>
            <b>OpenIDM</b> • {t.statusbar.activeBackground}
          </span>
        </div>
        <div className="statusbar-right">
          <button
            className="statusbar-link-btn"
            onClick={() => settings?.defaultSaveDir && api.openDirectoryPath(settings.defaultSaveDir)}
            title={t.statusbar.openDownloadsFolderTitle}
          >
            {t.statusbar.openDownloadsFolder}
          </button>
          <span>
            {t.statusbar.summary(downloads.length, settings?.maxConnectionsPerDownload || 8)}
          </span>
        </div>
      </footer>

      {showAddModalInFullApp && (
        <AddUrlModal
          standalone={false}
          initialUrl={prefillUrl}
          initialCapture={null}
          categories={categories}
          queues={queues}
          defaultMaxConnections={settings?.maxConnectionsPerDownload || 8}
          onClose={() => {
            setShowAddModalInFullApp(false);
            setPrefillUrl("");
          }}
          onSubmit={handleAddDownload}
          onCategoryUpdated={loadAll}
        />
      )}

      {detailsDownload && (
        <DownloadDetailsModal
          download={detailsDownload}
          onClose={() => setDetailsId(null)}
          onStart={(id) => api.startDownload(id)}
          onPause={(id) => api.pauseDownload(id)}
          onCancel={(id) => api.cancelDownload(id)}
          onDelete={async (id, deleteFile) => {
            await api.deleteDownload(id, deleteFile);
            handleDismissCardItem(id);
            setDetailsId(null);
            if (selectedId === id) setSelectedId(null);
            await loadAll();
          }}
          onOpenFile={(id) => api.openFile(id)}
          onOpenFolder={(id) => api.openFolder(id)}
          onUpdateSpeedLimit={(id, bps) => api.setDownloadSpeedLimit(id, bps)}
        />
      )}

      {showScheduler && (
        <SchedulerModal
          queues={queues}
          onClose={() => setShowScheduler(false)}
          onSaveQueue={async (q) => {
            await api.saveQueue(q);
            await loadAll();
          }}
          onDeleteQueue={async (id) => {
            await api.deleteQueue(id);
            await loadAll();
          }}
          onStartQueue={async (id) => {
            await api.startQueue(id);
            await loadAll();
          }}
          onStopQueue={async (id) => {
            await api.stopQueue(id);
            await loadAll();
          }}
          onCancelShutdown={() => api.cancelSystemShutdown()}
        />
      )}

      {showSettings && settings && (
        <SettingsModal
          settings={settings}
          siteRules={siteRules}
          siteLogins={siteLogins}
          onClose={() => setShowSettings(false)}
          onSaveSettings={async (cfg) => {
            await api.saveSettings(cfg);
            if (cfg.language) {
              const norm = setActiveLanguageInMemory(cfg.language);
              onBackendLanguageLoaded(norm);
            }
            await loadAll();
          }}
          onSaveSiteRule={async (r) => {
            await api.saveSiteRule(r);
            await loadAll();
          }}
          onDeleteSiteRule={async (id) => {
            await api.deleteSiteRule(id);
            await loadAll();
          }}
          onSaveSiteLogin={async (l) => {
            await api.saveSiteLogin(l);
            await loadAll();
          }}
          onDeleteSiteLogin={async (id) => {
            await api.deleteSiteLogin(id);
            await loadAll();
          }}
          onInstallBrowserIntegration={(extId) => api.installBrowserIntegration(extId)}
          onOpenInstaller={() => setShowInstaller(true)}
        />
      )}

      {showInstaller && (
        <InstallerModal
          onClose={() => setShowInstaller(false)}
          onInstalled={() => {
            loadAll();
          }}
        />
      )}
    </div>
  );
};

export const App: React.FC = () => {
  const [lang, setLang] = useState<AppLanguage>(() => getActiveLanguage());

  const handleLanguageChange = useCallback((next: AppLanguage) => {
    const norm = setActiveLanguageInMemory(next);
    setLang(norm);
    api.setLanguage(norm).catch(() => {});
  }, []);

  return (
    <LanguageProvider initialLanguage={lang} onLanguageChange={handleLanguageChange}>
      <AppContent onBackendLanguageLoaded={setLang} />
    </LanguageProvider>
  );
};
