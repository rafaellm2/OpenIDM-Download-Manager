import React, { useRef, useState } from "react";
import { useI18n } from "../i18n";
import { Download } from "../types";
import {
  formatBytes,
  formatETA,
  formatPercent,
  formatSpeedStable,
  statusLabel
} from "../utils/format";
import { SegmentBar } from "./SegmentBar";

interface CardDownloadsListProps {
  downloads: Download[];
  categoryIcons: Record<string, string>;
  hideHeader?: boolean;
  onStart: (id: string) => void;
  onPause: (id: string) => void;
  onPauseAll: () => void;
  onCancel: (id: string) => void;
  onDelete: (id: string) => void;
  onOpenFile: (id: string) => void;
  onOpenFolder: (id: string) => void;
  onDismissItem: (id: string) => void;
  onClearCompleted: () => void;
}

// Lista compacta, retangular e direta de downloads em andamento
export const CardDownloadsList: React.FC<CardDownloadsListProps> = ({
  downloads,
  categoryIcons,
  hideHeader = false,
  onStart,
  onPause,
  onPauseAll,
  onCancel,
  onDelete,
  onOpenFile,
  onOpenFolder,
  onDismissItem,
  onClearCompleted
}) => {
  const { language, t } = useI18n();

  // Permite expandir opcionalmente o mapa de conexões de um item específico da lista
  const [expandedSegId, setExpandedSegId] = useState<string | null>(null);
  // Preserva última velocidade válida por até 1.5s para evitar qualquer oscilação visual
  const speedCacheRef = useRef<Record<string, { bps: number; ts: number }>>({});

  if (downloads.length === 0) {
    return null;
  }

  const now = Date.now();
  const getEffectiveSpeed = (d: Download): number => {
    const isRunning =
      d.status === "downloading" || d.status === "probing" || d.status === "assembling";
    if (!isRunning) {
      delete speedCacheRef.current[d.id];
      return 0;
    }
    if (d.speedBps && d.speedBps > 0) {
      speedCacheRef.current[d.id] = { bps: d.speedBps, ts: now };
      return d.speedBps;
    }
    const cached = speedCacheRef.current[d.id];
    if (cached && now - cached.ts < 2200) {
      return cached.bps;
    }
    return 0;
  };

  // Contabiliza métricas agregadas de todos os downloads ativos no card
  const activeCount = downloads.filter(
    (d) => d.status === "downloading" || d.status === "probing" || d.status === "assembling"
  ).length;
  const completedCount = downloads.filter(
    (d) => d.status === "completed" || d.status === "cancelled"
  ).length;
  const totalSpeedBps = downloads.reduce((acc, cur) => acc + getEffectiveSpeed(cur), 0);

  return (
    <div className="card-downloads-section">
      {/* Cabeçalho compacto opcional da lista */}
      {!hideHeader && (
        <div className="card-downloads-header">
          <div className="card-downloads-title-group">
            <span className="card-downloads-title">
              {t.cardList.headerTitle(downloads.length)}
            </span>
            <span
              className="card-speed-pill"
              data-active={activeCount > 0 ? "true" : "false"}
            >
              <span className="speed-dot" data-active={activeCount > 0} />
              <span className="card-speed-pill-text">
                {activeCount > 0
                  ? t.cardList.downloadingCount(
                      activeCount,
                      formatSpeedStable(totalSpeedBps, true, language)
                    )
                  : t.cardList.inListCount(downloads.length)}
              </span>
            </span>
          </div>

          <div className="card-downloads-actions">
            {activeCount > 0 && (
              <button
                type="button"
                className="mini-btn"
                onClick={onPauseAll}
                title={t.cardList.pauseAllTitle}
              >
                {t.cardList.pauseAllBtn}
              </button>
            )}
            {completedCount > 0 && (
              <button
                type="button"
                className="mini-btn"
                onClick={onClearCompleted}
                title={t.cardList.clearTitle}
              >
                {t.cardList.clearBtn}
              </button>
            )}
          </div>
        </div>
      )}

      {/* Lista rolável dos downloads agrupados no card */}
      <div className="card-downloads-list">
        {downloads.map((d) => {
          const isRunning =
            d.status === "downloading" ||
            d.status === "probing" ||
            d.status === "assembling";
          const isCompleted = d.status === "completed";
          const pct = Math.min(100, Math.max(0, d.progress || 0));
          const catIcon = categoryIcons[d.categoryId] || "📦";
          const showSegments = expandedSegId === d.id;
          const itemSpeedBps = getEffectiveSpeed(d);

          return (
            <div
              key={d.id}
              className={`card-download-item status-border-${d.status}`}
            >
              {/* Linha 1: Ícone, nome do arquivo, status e botões de ação direta (incluindo Excluir) */}
              <div className="card-item-top">
                <div className="card-item-file" title={d.targetPath || d.fileName}>
                  <span className="card-item-icon">{catIcon}</span>
                  <span className="card-item-name">{d.fileName}</span>
                </div>

                <div className="card-item-controls">
                  <span className={`status-badge status-${d.status}`}>
                    {statusLabel(d.status, language)}
                  </span>

                  {isRunning ? (
                    <button
                      type="button"
                      className="mini-btn"
                      onClick={() => onPause(d.id)}
                      title={t.cardList.pauseItemTitle}
                    >
                      ⏸
                    </button>
                  ) : (
                    !isCompleted && (
                      <button
                        type="button"
                        className="mini-btn primary-mini"
                        onClick={() => onStart(d.id)}
                        title={t.cardList.resumeItemTitle}
                      >
                        ▶
                      </button>
                    )
                  )}

                  {!isCompleted && d.status !== "cancelled" && (
                    <button
                      type="button"
                      className="mini-btn"
                      onClick={() => onCancel(d.id)}
                      title={t.cardList.stopItemTitle}
                    >
                      ⏹
                    </button>
                  )}

                  {isCompleted && (
                    <button
                      type="button"
                      className="mini-btn primary-mini"
                      onClick={() => onOpenFile(d.id)}
                      title={t.cardList.openFileTitle}
                    >
                      {t.cardList.openFileBtn}
                    </button>
                  )}

                  <button
                    type="button"
                    className="mini-btn"
                    onClick={() => onOpenFolder(d.id)}
                    title={t.cardList.openFolderTitle}
                  >
                    📂
                  </button>

                  {/* Botão de Excluir direto na lista de baixando */}
                  <button
                    type="button"
                    className="mini-btn danger-mini"
                    onClick={() => onDelete(d.id)}
                    title={t.cardList.deleteItemTitle}
                  >
                    🗑
                  </button>

                  {!isRunning && (
                    <button
                      type="button"
                      className="mini-btn dismiss-mini"
                      onClick={() => onDismissItem(d.id)}
                      title={t.cardList.dismissItemTitle}
                    >
                      ✕
                    </button>
                  )}
                </div>
              </div>

              {/* Linha 2: Barra de progresso individual */}
              <div className="card-item-progress-track">
                <div
                  className={`card-item-progress-bar status-${d.status}`}
                  style={{ width: `${pct}%` }}
                />
              </div>

              {/* Linha 3: Métricas em linha única com largura tabular estável (sem quebra de linha) */}
              <div className="card-item-metrics">
                <div className="card-item-metrics-left">
                  <strong className="card-item-pct">
                    {formatPercent(pct, 1, language)}
                  </strong>
                  <span className="card-item-bytes">
                    {formatBytes(d.downloadedSize, language)}
                    {d.totalSize > 0 ? ` / ${formatBytes(d.totalSize, language)}` : ""}
                  </span>
                </div>

                <div className="card-item-metrics-right">
                  {isRunning && (
                    <>
                      <span className="card-metric-speed">
                        ⚡ {formatSpeedStable(itemSpeedBps, true, language)}
                      </span>
                      <span className="card-metric-eta">
                        ⏱ {formatETA(d.etaSeconds)}
                      </span>
                    </>
                  )}
                  {isCompleted && (
                    <span className="success-text card-completed-dir" title={d.saveDir}>
                      {t.cardList.completedBadge}
                    </span>
                  )}
                  {d.status === "error" && d.errorMessage && (
                    <span className="danger-text card-error-msg" title={d.errorMessage}>
                      ⚠ {d.errorMessage}
                    </span>
                  )}
                  {(d.segments?.length || 0) > 1 && (
                    <button
                      type="button"
                      className="card-conn-toggle"
                      onClick={() =>
                        setExpandedSegId(showSegments ? null : d.id)
                      }
                      title={t.cardList.toggleSegmentsTitle}
                    >
                      {d.activeWorkers || d.segments.length}x{" "}
                      {showSegments ? "▴" : "▾"}
                    </button>
                  )}
                </div>
              </div>

              {/* Mapa de fatias opcional se o usuário clicar no indicador de conexões */}
              {showSegments && d.segments && d.segments.length > 0 && (
                <div style={{ marginTop: 6 }}>
                  <SegmentBar
                    segments={d.segments}
                    totalSize={d.totalSize}
                    downloadedSize={d.downloadedSize}
                    height={10}
                    showLegend={false}
                  />
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
};

