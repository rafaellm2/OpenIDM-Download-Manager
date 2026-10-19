import React, { useEffect, useRef, useState } from "react";
import { formatCategoryName, useI18n } from "../i18n";
import { api } from "../services/api";
import {
  AddDownloadRequest,
  CapturePrompt,
  Category,
  Download,
  Queue
} from "../types";
import { formatBytes } from "../utils/format";

interface AddUrlModalProps {
  initialUrl?: string;
  initialCapture?: CapturePrompt | null;
  categories: Category[];
  queues: Queue[];
  defaultMaxConnections: number;
  standalone?: boolean;
  cardDownloads?: Download[];
  pendingCaptures?: CapturePrompt[];
  forceShowForm?: boolean;
  onClose: () => void;
  onSavedToCard?: () => void;
  onSubmit: (req: AddDownloadRequest) => Promise<Download | void>;
  onCategoryUpdated?: () => void;
  onSelectPendingCapture?: (prompt: CapturePrompt) => void;
  onDismissPendingCapture?: (url: string) => void;
  onOpenFullApp?: () => void;
}

// Ícones simplificados por ID de categoria
export const CATEGORY_ICONS: Record<string, string> = {
  compressed: "📦",
  documents: "📄",
  music: "🎵",
  programs: "💻",
  video: "🎬",
  general: "📂"
};

// Tela dedicada exclusivamente para salvar um novo download (escolher nome, categoria e pasta).
// Após confirmar ("Baixar Agora" ou "Baixar Depois"), transiciona imediatamente para o Card Reduzido de acompanhamento.
export const AddUrlModal: React.FC<AddUrlModalProps> = ({
  initialUrl = "",
  initialCapture = null,
  categories,
  defaultMaxConnections,
  standalone = true,
  cardDownloads = [],
  pendingCaptures = [],
  onClose,
  onSavedToCard,
  onSubmit,
  onCategoryUpdated,
  onSelectPendingCapture,
  onDismissPendingCapture,
  onOpenFullApp
}) => {
  const { language, toggleLanguage, t } = useI18n();

  // Estados do formulário de salvar novo download
  const [url, setUrl] = useState(initialCapture?.url || initialUrl || "");
  const [fileName, setFileName] = useState(initialCapture?.fileName || "");
  const [categoryId, setCategoryId] = useState(initialCapture?.categoryId || "general");
  const [autoMatchedCatId, setAutoMatchedCatId] = useState(initialCapture?.categoryId || "");
  const [saveDir, setSaveDir] = useState(initialCapture?.saveDir || "");
  const [saveAsCategoryDefault, setSaveAsCategoryDefault] = useState(false);
  const [openOnComplete] = useState(false);

  // Estados de telemetria e sondagem prévia (probe)
  const [probing, setProbing] = useState(false);
  const [probedSize, setProbedSize] = useState<number>(initialCapture?.totalSize || 0);
  const [acceptRanges, setAcceptRanges] = useState<boolean>(initialCapture?.acceptRanges ?? true);
  const [probedDone, setProbedDone] = useState<boolean>(initialCapture?.probed || false);
  const [probeError, setProbeError] = useState<string>(initialCapture?.probeError || "");

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [existingFolders, setExistingFolders] = useState<Array<{ name: string; path: string }>>([]);

  // Rastreamento de interação do usuário para evitar sobreescrita indesejada
  const userEditedFileNameRef = useRef(false);
  const userSelectedCategoryRef = useRef(false);
  const userEditedSaveDirRef = useRef(false);
  const lastInitKeyRef = useRef<string>("");
  const originCaptureUrlRef = useRef<string>((initialCapture?.url || initialUrl || "").trim());
  const hintFileNameRef = useRef<string>((initialCapture?.fileName || "").trim());
  const referrerRef = useRef<string>((initialCapture?.referrer || "").trim());

  const headerRef = useRef<HTMLDivElement | null>(null);
  const queueBarRef = useRef<HTMLDivElement | null>(null);
  const saveSectionRef = useRef<HTMLDivElement | null>(null);
  const lastSizeRef = useRef<{ w: number; h: number }>({ w: 0, h: 0 });

  // Preenche a pasta padrão da categoria selecionada quando o formulário abre vazio
  useEffect(() => {
    if (!saveDir.trim() && categories.length > 0 && !userEditedSaveDirRef.current) {
      const cat = categories.find((c) => c.id === categoryId) || categories[0];
      if (cat?.defaultDir) {
        setSaveDir(cat.defaultDir);
      }
    }
  }, [categories, categoryId, saveDir]);

  // Carrega pastas existentes em Downloads para atalhos rápidos em chips
  useEffect(() => {
    api
      .getExistingDownloadFolders()
      .then((folders) => {
        if (Array.isArray(folders)) {
          const normalized = folders
            .map((item: any) => {
              if (typeof item === "string") {
                const name = item.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || item;
                return { name, path: item };
              }
              if (item && typeof item.path === "string") {
                const name =
                  item.name ||
                  item.path.replace(/[\\/]+$/, "").split(/[\\/]/).pop() ||
                  item.path;
                return { name, path: item.path };
              }
              return null;
            })
            .filter((x): x is { name: string; path: string } => !!x && !!x.path);
          setExistingFolders(normalized);
        }
      })
      .catch(() => {});
  }, []);

  // Notifica backend sobre abertura do formulário de captura
  useEffect(() => {
    api.ackCaptureModalOpened(url, existingFolders.length).catch(() => {});
  }, [url, existingFolders.length]);

  // Ajusta a janela em formato retangular responsivo durante a escolha de onde salvar, sem cortar botões
  useEffect(() => {
    if (!standalone) return;
    const targetWidth = 700;
    const updateSize = () => {
      const queueExtra = pendingCaptures.length > 1 ? 38 : 0;
      const fallbackHeight = 425 + queueExtra;
      const headerH = headerRef.current?.offsetHeight || 48;
      const queueH = queueBarRef.current?.offsetHeight || 0;
      const sectionH = saveSectionRef.current?.scrollHeight || 0;
      const measured = sectionH > 0 ? headerH + queueH + sectionH + 30 : fallbackHeight;
      const targetHeight = Math.max(415, Math.min(540, Math.max(fallbackHeight, measured)));

      if (
        lastSizeRef.current.w !== targetWidth ||
        Math.abs(lastSizeRef.current.h - targetHeight) >= 6
      ) {
        lastSizeRef.current = { w: targetWidth, h: targetHeight };
        api.resizeStandaloneCard(targetWidth, targetHeight).catch(() => {});
      }
    };

    updateSize();
    const ro =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(() => updateSize())
        : null;
    if (ro && saveSectionRef.current) {
      ro.observe(saveSectionRef.current);
    }
    return () => {
      if (ro) ro.disconnect();
    };
  }, [standalone, pendingCaptures.length, existingFolders.length, error]);

  // Sincroniza a posição caso o usuário mova o card de lugar
  useEffect(() => {
    if (!standalone) return;
    const syncPos = () => {
      api.getCardPosition().catch(() => {});
    };
    window.addEventListener("mouseup", syncPos);
    window.addEventListener("blur", syncPos);
    return () => {
      window.removeEventListener("mouseup", syncPos);
      window.removeEventListener("blur", syncPos);
    };
  }, [standalone]);

  // Sincroniza dados quando uma nova captura de link chega ao Card
  useEffect(() => {
    const incomingUrl = (initialCapture?.url || initialUrl || "").trim();
    if (!incomingUrl) return;
    const incomingFile = (initialCapture?.fileName || "").trim();
    const initKey = `${incomingUrl}|${incomingFile}`;
    if (lastInitKeyRef.current === initKey) return;
    lastInitKeyRef.current = initKey;
    originCaptureUrlRef.current = incomingUrl;

    userEditedFileNameRef.current = false;
    userSelectedCategoryRef.current = false;
    userEditedSaveDirRef.current = false;
    hintFileNameRef.current = incomingFile;
    referrerRef.current = (initialCapture?.referrer || "").trim();
    setUrl(incomingUrl);
    setError("");

    if (initialCapture) {
      if (initialCapture.fileName) setFileName(initialCapture.fileName);
      if (initialCapture.categoryId) {
        setCategoryId(initialCapture.categoryId);
        setAutoMatchedCatId(initialCapture.categoryId);
      }
      if (initialCapture.saveDir) setSaveDir(initialCapture.saveDir);
    }
  }, [initialCapture, initialUrl]);

  // Ao abrir formulário sem URL, lê link da área de transferência automaticamente
  useEffect(() => {
    if (!url.trim()) {
      api
        .getClipboardURL()
        .then((clip) => {
          if (clip) {
            lastInitKeyRef.current = `${clip.trim()}|`;
            hintFileNameRef.current = "";
            referrerRef.current = "";
            setUrl(clip.trim());
          }
        })
        .catch(() => {});
    }
  }, []);

  // Atalho Esc: volta para a lista compacta de downloads (se houver) ou fecha
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        if (standalone && cardDownloads.length > 0 && onSavedToCard) {
          onSavedToCard();
        } else {
          onClose();
        }
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose, onSavedToCard, standalone, cardDownloads.length]);

  // Consulta metadados (tamanho e nome sugerido) ao alterar a URL no formulário
  useEffect(() => {
    const clean = url.trim();
    if (
      !clean.startsWith("http://") &&
      !clean.startsWith("https://") &&
      !clean.startsWith("ftp://")
    ) {
      setProbedDone(false);
      setProbedSize(0);
      setProbeError("");
      return;
    }

    let cancelled = false;
    const hintName = hintFileNameRef.current;

    api
      .classifyURL(clean, hintName)
      .then((res) => {
        if (cancelled) return;
        if (res.fileName && !userEditedFileNameRef.current) setFileName(res.fileName);
        if (res.categoryId) {
          setAutoMatchedCatId(res.categoryId);
          if (!userSelectedCategoryRef.current) {
            setCategoryId(res.categoryId);
          }
        }
        if (res.saveDir && !userSelectedCategoryRef.current && !userEditedSaveDirRef.current) {
          setSaveDir(res.saveDir);
        }
      })
      .catch(() => {});

    const timer = setTimeout(() => {
      setProbing(true);
      setProbeError("");
      api
        .probeAndClassifyURL(clean, hintName)
        .then((res) => {
          if (cancelled || !res) return;
          if (res.fileName && res.fileName !== "download.bin" && !hintFileNameRef.current) {
            hintFileNameRef.current = res.fileName;
          }
          if (res.url && res.url !== clean) {
            lastInitKeyRef.current = `${res.url}|${hintFileNameRef.current || res.fileName || ""}`;
            setUrl(res.url);
          }
          setProbedSize(res.totalSize || 0);
          setAcceptRanges(!!res.acceptRanges);
          setProbedDone(!!res.probed);
          setProbeError(res.probeError || "");

          if (
            res.fileName &&
            res.fileName !== "download.bin" &&
            !userEditedFileNameRef.current
          ) {
            setFileName(res.fileName);
          }
          if (res.categoryId) {
            setAutoMatchedCatId(res.categoryId);
            if (!userSelectedCategoryRef.current) {
              setCategoryId(res.categoryId);
            }
          }
          if (res.saveDir && !userSelectedCategoryRef.current && !userEditedSaveDirRef.current) {
            setSaveDir(res.saveDir);
          }
        })
        .catch(() => {})
        .finally(() => {
          if (!cancelled) setProbing(false);
        });
    }, 60);

    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [url, initialCapture?.fileName]);

  const handleCategoryChange = (catId: string) => {
    userSelectedCategoryRef.current = true;
    userEditedSaveDirRef.current = false;
    setCategoryId(catId);
    setSaveAsCategoryDefault(false);
    const found = categories.find((c) => c.id === catId);
    if (found && found.defaultDir) {
      setSaveDir(found.defaultDir);
    }
  };

  const handleBrowseFolder = async () => {
    try {
      const picked = await api.browseFolder(saveDir);
      if (picked) {
        userEditedSaveDirRef.current = true;
        setSaveDir(picked);
      }
    } catch (_err) {
      // Usuário cancelou
    }
  };

  const handlePasteClipboard = async () => {
    const clip = await api.getClipboardURL();
    if (clip) {
      userEditedFileNameRef.current = false;
      userSelectedCategoryRef.current = false;
      userEditedSaveDirRef.current = false;
      hintFileNameRef.current = "";
      referrerRef.current = "";
      setUrl(clip);
    }
  };

  const selectedCategory = categories.find((c) => c.id === categoryId);

  const applyPendingPrompt = (nextPrompt: CapturePrompt) => {
    userEditedFileNameRef.current = false;
    userSelectedCategoryRef.current = false;
    userEditedSaveDirRef.current = false;
    originCaptureUrlRef.current = (nextPrompt.url || "").trim();
    hintFileNameRef.current = (nextPrompt.fileName || "").trim();
    referrerRef.current = (nextPrompt.referrer || "").trim();
    lastInitKeyRef.current = `${nextPrompt.url}|${nextPrompt.fileName || ""}`;
    setUrl(nextPrompt.url || "");
    setFileName(nextPrompt.fileName || "");
    if (nextPrompt.categoryId) {
      setCategoryId(nextPrompt.categoryId);
      setAutoMatchedCatId(nextPrompt.categoryId);
    }
    if (nextPrompt.saveDir) {
      setSaveDir(nextPrompt.saveDir);
    }
    onSelectPendingCapture?.(nextPrompt);
  };

  // Confirma o download e transiciona para o Card Reduzido de acompanhamento (apenas lista)
  const handleAdd = async (startNow: boolean) => {
    setError("");
    const currentUrl = url.trim();
    if (!currentUrl) {
      setError(t.addModal.invalidUrlError);
      return;
    }
    setSubmitting(true);
    try {
      if (saveAsCategoryDefault && selectedCategory && saveDir.trim()) {
        await api.saveCategory({
          ...selectedCategory,
          defaultDir: saveDir.trim()
        });
        onCategoryUpdated?.();
      }

      const originUrl = originCaptureUrlRef.current.trim();

      await onSubmit({
        url: currentUrl,
        referrer: referrerRef.current || initialCapture?.referrer || "",
        fileName: fileName.trim(),
        saveDir: saveDir.trim(),
        description: "",
        categoryId,
        queueId: "main",
        maxConnections: defaultMaxConnections || 8,
        startNow,
        username: "",
        password: "",
        openOnComplete
      });

      onDismissPendingCapture?.(currentUrl);
      if (originUrl && originUrl.toLowerCase() !== currentUrl.toLowerCase()) {
        onDismissPendingCapture?.(originUrl);
      }

      setUrl("");
      setFileName("");
      originCaptureUrlRef.current = "";
      lastInitKeyRef.current = "";

      // Após confirmar o download, abre imediatamente o Card Reduzido de Andamento (lista limpa)
      if (standalone && onSavedToCard) {
        onSavedToCard();
      } else {
        onClose();
      }
    } catch (err: any) {
      setError(err?.message || String(err));
    } finally {
      setSubmitting(false);
    }
  };

  // Inicia todos os links pendentes capturados de uma só vez e vai para o Card Reduzido
  const handleDownloadAllPending = async () => {
    if (pendingCaptures.length === 0 || submitting) return;
    setSubmitting(true);
    setError("");
    try {
      for (const item of pendingCaptures) {
        if (!item.url?.trim()) continue;
        await onSubmit({
          url: item.url.trim(),
          referrer: item.referrer || "",
          fileName: item.fileName || "",
          saveDir: item.saveDir || saveDir.trim(),
          description: "",
          categoryId: item.categoryId || "general",
          queueId: "main",
          maxConnections: defaultMaxConnections || 8,
          startNow: true,
          username: "",
          password: "",
          openOnComplete: false
        });
        onDismissPendingCapture?.(item.url);
      }
      setUrl("");
      setFileName("");
      originCaptureUrlRef.current = "";
      lastInitKeyRef.current = "";
      if (standalone && onSavedToCard) {
        onSavedToCard();
      } else {
        onClose();
      }
    } catch (err: any) {
      setError(err?.message || String(err));
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancelForm = () => {
    const currentUrl = url.trim();
    const originUrl = originCaptureUrlRef.current.trim();
    if (currentUrl) {
      onDismissPendingCapture?.(currentUrl);
    }
    if (originUrl && originUrl.toLowerCase() !== currentUrl.toLowerCase()) {
      onDismissPendingCapture?.(originUrl);
    }
    const remainingQueue = pendingCaptures.filter((p) => {
      const candidate = p.url.trim().toLowerCase();
      return (
        candidate !== currentUrl.toLowerCase() &&
        (!originUrl || candidate !== originUrl.toLowerCase())
      );
    });
    if (remainingQueue.length > 0) {
      applyPendingPrompt(remainingQueue[0]);
      return;
    }
    setUrl("");
    setFileName("");
    originCaptureUrlRef.current = "";
    lastInitKeyRef.current = "";
    if (standalone && cardDownloads.length > 0 && onSavedToCard) {
      onSavedToCard();
      return;
    }
    onClose();
  };

  return (
    <div
      className={
        standalone ? "standalone-card-shell rectangular-save-card" : "modal-backdrop"
      }
      onClick={standalone ? undefined : onClose}
    >
      <div
        className={`modal-window capture-modal ${standalone ? "standalone-card-window" : ""}`}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Cabeçalho da tela de Salvar Novo Download */}
        <div
          ref={headerRef}
          className="modal-header capture-header standalone-card-header"
        >
          <div className="standalone-header-left">
            <div className="standalone-title-line">
              <h3 className="standalone-card-main-title">{t.addModal.title}</h3>
              {probing ? (
                <span className="probe-pill" style={{ fontSize: 11, padding: "2px 8px" }}>
                  {t.addModal.probing}
                </span>
              ) : probedDone && probedSize > 0 ? (
                <span className="probe-pill ok" style={{ fontSize: 11, padding: "2px 8px" }}>
                  <b>{formatBytes(probedSize, language)}</b>
                  {acceptRanges ? t.addModal.multiConn : ""}
                </span>
              ) : probeError ? (
                <span className="probe-pill warn" style={{ fontSize: 11, padding: "2px 8px" }}>
                  {t.addModal.checkLink}
                </span>
              ) : null}
            </div>
          </div>

          <div className="standalone-header-actions">
            {standalone && cardDownloads.length > 0 && onSavedToCard && (
              <button
                type="button"
                className="btn standalone-top-btn"
                onClick={onSavedToCard}
                title={t.addModal.viewListTitle}
              >
                {t.addModal.viewListBtn(cardDownloads.length)}
              </button>
            )}

            {standalone && onOpenFullApp && (
              <button
                type="button"
                className="btn standalone-top-btn"
                onClick={onOpenFullApp}
                title={t.addModal.fullAppTitle}
              >
                {t.addModal.fullAppBtn}
              </button>
            )}

            <button
              type="button"
              className="btn standalone-top-btn"
              onClick={toggleLanguage}
              title={t.toolbar.languageToggleTitle}
            >
              🌐 {language === "en-US" ? "EN" : "PT"}
            </button>

            <button
              type="button"
              className="close-btn"
              onClick={handleCancelForm}
              title={t.addModal.closeTitle}
            >
              ✕
            </button>
          </div>
        </div>

        {/* Barra de múltiplos links capturados aguardando confirmação */}
        {pendingCaptures.length > 1 && (
          <div ref={queueBarRef} className="card-pending-queue-bar">
            <div className="card-pending-queue-info">
              <span>{t.addModal.capturedQueueLabel(pendingCaptures.length)}</span>
              <div className="card-pending-pills">
                {pendingCaptures.map((item, idx) => {
                  const isCurrent =
                    item.url.trim().toLowerCase() === url.trim().toLowerCase();
                  return (
                    <button
                      key={`${item.url}-${idx}`}
                      type="button"
                      className={`pending-capture-pill ${isCurrent ? "active" : ""}`}
                      onClick={() => applyPendingPrompt(item)}
                      title={item.url}
                    >
                      #{idx + 1} {item.fileName || "link"}
                    </button>
                  );
                })}
              </div>
            </div>
            <button
              type="button"
              className="btn primary standalone-top-btn"
              onClick={handleDownloadAllPending}
              disabled={submitting}
              title={t.addModal.downloadAllTitle}
            >
              {t.addModal.downloadAllBtn(pendingCaptures.length)}
            </button>
          </div>
        )}

        <div className="modal-body standalone-card-body">
          <div ref={saveSectionRef} className="card-save-section">
            {error && <div className="alert-banner danger">{error}</div>}

            {/* Campo 1: URL do Download */}
            <div className="form-group" style={{ marginBottom: 10 }}>
              <label>{t.addModal.urlLabel}</label>
              <div className="inline-group" style={{ marginBottom: 0 }}>
                <input
                  type="text"
                  autoFocus
                  placeholder={t.addModal.urlPlaceholder}
                  value={url}
                  onChange={(e) => {
                    userEditedFileNameRef.current = false;
                    userSelectedCategoryRef.current = false;
                    userEditedSaveDirRef.current = false;
                    hintFileNameRef.current = "";
                    referrerRef.current = "";
                    setUrl(e.target.value);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !submitting) {
                      e.preventDefault();
                      handleAdd(true);
                    }
                  }}
                />
                <button
                  type="button"
                  className="btn"
                  onClick={handlePasteClipboard}
                  title={t.addModal.pasteTitle}
                >
                  {t.addModal.pasteBtn}
                </button>
              </div>
            </div>

            {/* Campo 2: Nome do Arquivo e Categoria lado a lado */}
            <div className="form-grid-2">
              <div className="form-group" style={{ marginBottom: 10 }}>
                <label>{t.addModal.fileNameLabel}</label>
                <input
                  type="text"
                  placeholder={t.addModal.fileNamePlaceholder}
                  value={fileName}
                  onChange={(e) => {
                    userEditedFileNameRef.current = true;
                    setFileName(e.target.value);
                  }}
                />
              </div>

              <div className="form-group" style={{ marginBottom: 10 }}>
                <label>{t.addModal.categoryLabel}</label>
                <select
                  value={categoryId}
                  onChange={(e) => handleCategoryChange(e.target.value)}
                >
                  {categories.map((c) => (
                    <option key={c.id} value={c.id}>
                      {CATEGORY_ICONS[c.id] || "📁"} {formatCategoryName(c, language)}
                      {c.id === autoMatchedCatId ? t.addModal.autoSuffix : ""}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            {/* Campo 3: Local / Pasta onde salvar */}
            <div className="form-group" style={{ marginBottom: 8 }}>
              <label>{t.addModal.saveDirLabel}</label>
              <div className="inline-group" style={{ marginBottom: 6 }}>
                <input
                  type="text"
                  value={saveDir}
                  onChange={(e) => {
                    userEditedSaveDirRef.current = true;
                    setSaveDir(e.target.value);
                  }}
                  placeholder="C:\Users\...\Downloads"
                />
                <button
                  type="button"
                  className="btn"
                  onClick={handleBrowseFolder}
                  title={t.addModal.folderBtnTitle}
                >
                  {t.addModal.folderBtn}
                </button>
              </div>

              {existingFolders.length > 0 && (
                <div className="existing-folders-chips" style={{ marginTop: 4 }}>
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 5 }}>
                    {existingFolders.slice(0, 6).map((folder) => {
                      const folderPath = typeof folder === "string" ? folder : folder.path || "";
                      const folderName =
                        (typeof folder === "object" && folder?.name) ||
                        folderPath.replace(/[\\/]+$/, "").split(/[\\/]/).pop() ||
                        folderPath;
                      if (!folderPath) return null;
                      const isSelectedFolder =
                        saveDir.trim().toLowerCase() === folderPath.trim().toLowerCase();
                      return (
                        <button
                          key={folderPath}
                          type="button"
                          className={`btn ${isSelectedFolder ? "primary" : ""}`}
                          style={{
                            padding: "2px 8px",
                            fontSize: 11,
                            borderRadius: 6
                          }}
                          title={folderPath}
                          onClick={() => {
                            userEditedSaveDirRef.current = true;
                            setSaveDir(folderPath);
                          }}
                        >
                          📁 {folderName}
                        </button>
                      );
                    })}
                  </div>
                </div>
              )}
            </div>

            {/* Botões de confirmação que iniciam/enfileiram e abrem o Card Reduzido */}
            <div className="card-save-actions">
              <button
                type="button"
                className="btn"
                onClick={handleCancelForm}
                disabled={submitting}
              >
                {t.addModal.cancelBtn}
              </button>
              <div className="footer-right">
                <button
                  type="button"
                  className="btn"
                  onClick={() => handleAdd(false)}
                  disabled={submitting}
                  title={t.addModal.downloadLaterTitle}
                >
                  {t.addModal.downloadLaterBtn}
                </button>
                <button
                  type="button"
                  className="btn primary"
                  onClick={() => handleAdd(true)}
                  disabled={submitting}
                >
                  {t.addModal.downloadNowBtn}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
