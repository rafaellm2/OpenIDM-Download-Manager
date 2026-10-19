import React, { useEffect, useRef } from "react";
import { useI18n } from "../i18n";
import { api } from "../services/api";
import { Download } from "../types";
import { formatSpeedStable } from "../utils/format";
import { CATEGORY_ICONS } from "./AddUrlModal";
import { CardDownloadsList } from "./CardDownloadsList";

interface StandaloneProgressCardProps {
  downloads: Download[];
  pendingCapturesCount?: number;
  onOpenNewSavePrompt: () => void;
  onOpenPendingCaptures?: () => void;
  onOpenFullApp: () => void;
  onClose: () => void;
  onStartDownload: (id: string) => void;
  onPauseDownload: (id: string) => void;
  onPauseAll: () => void;
  onCancelDownload: (id: string) => void;
  onDeleteDownload: (id: string) => void;
  onOpenFile: (id: string) => void;
  onOpenFolder: (id: string) => void;
  onDismissCardItem: (id: string) => void;
  onClearCompletedCard: () => void;
}

// Tela separada do Card Reduzido: exibe exclusivamente o andamento da lista de downloads
// em formato retangular e responsivo, sem opções de pastas ou formulário de salvar.
export const StandaloneProgressCard: React.FC<StandaloneProgressCardProps> = ({
  downloads,
  pendingCapturesCount = 0,
  onOpenNewSavePrompt,
  onOpenPendingCaptures,
  onOpenFullApp,
  onClose,
  onStartDownload,
  onPauseDownload,
  onPauseAll,
  onCancelDownload,
  onDeleteDownload,
  onOpenFile,
  onOpenFolder,
  onDismissCardItem,
  onClearCompletedCard
}) => {
  const { language, toggleLanguage, t } = useI18n();

  // Cache de velocidade para manter o indicador de velocidade total 100% estável sem oscilar para zero
  const speedCacheRef = useRef<Record<string, { bps: number; ts: number }>>({});
  const headerRef = useRef<HTMLDivElement | null>(null);
  const bodyContentRef = useRef<HTMLDivElement | null>(null);
  const lastSizeRef = useRef<{ w: number; h: number }>({ w: 0, h: 0 });

  // Ajusta a altura retangular do card de forma responsiva conforme o conteúdo real da lista
  useEffect(() => {
    const targetWidth = 700;
    const updateCardSize = () => {
      const count = downloads.length;
      const fallbackHeight =
        count <= 1
          ? 235
          : Math.min(470, 145 + Math.min(count, 4) * 80);

      const headerH = headerRef.current?.offsetHeight || 48;
      const contentH = bodyContentRef.current?.scrollHeight || 0;
      const measured = contentH > 0 ? headerH + contentH + 28 : fallbackHeight;
      const targetHeight = Math.max(215, Math.min(480, Math.max(fallbackHeight, measured)));

      if (
        lastSizeRef.current.w !== targetWidth ||
        Math.abs(lastSizeRef.current.h - targetHeight) >= 6
      ) {
        lastSizeRef.current = { w: targetWidth, h: targetHeight };
        api.resizeStandaloneCard(targetWidth, targetHeight).catch(() => {});
      }
    };

    updateCardSize();
    const ro =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(() => updateCardSize())
        : null;
    if (ro && bodyContentRef.current) {
      ro.observe(bodyContentRef.current);
    }
    return () => {
      if (ro) ro.disconnect();
    };
  }, [downloads.length, pendingCapturesCount]);

  // Sincroniza a preferência de posição atual do card sempre que o usuário terminar de movê-lo
  useEffect(() => {
    const syncPos = () => {
      api.getCardPosition().catch(() => {});
    };
    window.addEventListener("mouseup", syncPos);
    window.addEventListener("blur", syncPos);
    return () => {
      window.removeEventListener("mouseup", syncPos);
      window.removeEventListener("blur", syncPos);
    };
  }, []);

  const now = Date.now();
  const activeCount = downloads.filter(
    (d) =>
      d.status === "downloading" ||
      d.status === "probing" ||
      d.status === "assembling"
  ).length;
  const completedCount = downloads.filter(
    (d) => d.status === "completed" || d.status === "cancelled"
  ).length;

  const totalSpeedBps = downloads.reduce((acc, d) => {
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

  return (
    <div className="standalone-card-shell progress-only-card">
      <div className="modal-window standalone-card-window">
        {/* Cabeçalho compacto em linha única (sem quebra de linha durante o download) */}
        <div
          ref={headerRef}
          className="modal-header capture-header standalone-card-header"
        >
          <div className="standalone-header-left">
            <div className="standalone-title-line">
              <h3 className="standalone-card-main-title">
                {t.standaloneCard.title(downloads.length)}
              </h3>
              <span
                className="card-speed-pill"
                data-active={activeCount > 0 ? "true" : "false"}
                title={t.standaloneCard.speedTitle}
              >
                <span className="speed-dot" data-active={activeCount > 0} />
                <span className="card-speed-pill-text">
                  {activeCount > 0
                    ? t.standaloneCard.activeStatus(
                        activeCount,
                        formatSpeedStable(totalSpeedBps, true, language)
                      )
                    : t.standaloneCard.idleStatus}
                </span>
              </span>
            </div>
          </div>

          <div className="standalone-header-actions">
            {pendingCapturesCount > 0 && (
              <button
                type="button"
                className="btn primary standalone-top-btn"
                onClick={onOpenPendingCaptures || onOpenNewSavePrompt}
                title={t.standaloneCard.savePendingTitle}
              >
                {t.standaloneCard.savePendingBtn(pendingCapturesCount)}
              </button>
            )}

            {activeCount > 0 && (
              <button
                type="button"
                className="btn standalone-top-btn"
                onClick={onPauseAll}
                title={t.standaloneCard.pauseTitle}
              >
                {t.standaloneCard.pauseBtn}
              </button>
            )}

            {completedCount > 0 && (
              <button
                type="button"
                className="btn standalone-top-btn"
                onClick={onClearCompletedCard}
                title={t.standaloneCard.clearTitle}
              >
                {t.standaloneCard.clearBtn}
              </button>
            )}

            <button
              type="button"
              className="btn standalone-top-btn"
              onClick={onOpenNewSavePrompt}
              title={t.standaloneCard.newTitle}
            >
              {t.standaloneCard.newBtn}
            </button>

            <button
              type="button"
              className="btn standalone-top-btn"
              onClick={onOpenFullApp}
              title={t.standaloneCard.fullAppTitle}
            >
              {t.standaloneCard.fullAppBtn}
            </button>

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
              onClick={onClose}
              title={t.standaloneCard.closeTitle}
            >
              ✕
            </button>
          </div>
        </div>

        {/* Corpo direto e limpo: apenas a lista de andamento dos downloads */}
        <div className="modal-body standalone-card-body standalone-progress-body">
          <div ref={bodyContentRef}>
            {downloads.length === 0 ? (
              <div className="card-empty-progress">
                <span>{t.standaloneCard.emptyMessage}</span>
                <div className="card-empty-actions">
                  <button
                    type="button"
                    className="btn primary standalone-top-btn"
                    onClick={onOpenNewSavePrompt}
                  >
                    {t.standaloneCard.addLinkBtn}
                  </button>
                  <button
                    type="button"
                    className="btn standalone-top-btn"
                    onClick={onOpenFullApp}
                  >
                    {t.standaloneCard.openFullAppBtn}
                  </button>
                </div>
              </div>
            ) : (
              <CardDownloadsList
                downloads={downloads}
                categoryIcons={CATEGORY_ICONS}
                hideHeader={true}
                onStart={onStartDownload}
                onPause={onPauseDownload}
                onPauseAll={onPauseAll}
                onCancel={onCancelDownload}
                onDelete={onDeleteDownload}
                onOpenFile={onOpenFile}
                onOpenFolder={onOpenFolder}
                onDismissItem={onDismissCardItem}
                onClearCompleted={onClearCompletedCard}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
