import React, { useState } from "react";
import { useI18n } from "../i18n";
import { api } from "../services/api";
import { Download } from "../types";
import { formatBytes, formatETA, formatPercent, formatSpeed, statusLabel } from "../utils/format";
import { SegmentBar } from "./SegmentBar";

interface DownloadDetailsModalProps {
  download: Download;
  onClose: () => void;
  onStart: (id: string) => void;
  onPause: (id: string) => void;
  onCancel: (id: string) => void;
  onOpenFile: (id: string) => void;
  onOpenFolder: (id: string) => void;
  onUpdateSpeedLimit: (id: string, bps: number) => void;
  onDelete?: (id: string, deleteFile: boolean) => Promise<void> | void;
}

// Janela de detalhes do download com barra de progresso contínua destacada e conexões colapsáveis
export const DownloadDetailsModal: React.FC<DownloadDetailsModalProps> = ({
  download,
  onClose,
  onStart,
  onPause,
  onCancel,
  onOpenFile,
  onOpenFolder,
  onDelete
}) => {
  const { language, t } = useI18n();

  // Conexões e mapa de fatias ocultos por padrão sob botão toggle expansível (Requisito R6)
  const [showConnections, setShowConnections] = useState<boolean>(false);
  const [deleting, setDeleting] = useState<boolean>(false);

  const isRunning = download.status === "downloading" || download.status === "probing";
  const pctClamped = Math.min(100, Math.max(0, download.progress));

  // Exclui o download e apaga o arquivo do disco, fechando a modal
  const handleDelete = async () => {
    if (deleting) return;
    setDeleting(true);
    try {
      if (onDelete) {
        await onDelete(download.id, true);
      } else {
        await api.deleteDownload(download.id, true);
        onClose();
      }
    } catch (_err) {
      setDeleting(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-window large" onClick={(e) => e.stopPropagation()}>
        {/* Cabeçalho com nome do arquivo e pasta de destino */}
        <div className="modal-header">
          <div>
            <h3>
              {formatPercent(download.progress, 1, language)} — {download.fileName}
            </h3>
            <span className="modal-subtitle" title={download.targetPath || download.saveDir}>
              {t.detailsModal.savedIn} {download.saveDir || download.targetPath}
            </span>
          </div>
          <button className="close-btn" onClick={onClose} title={t.detailsModal.closeTitle}>
            ✕
          </button>
        </div>

        <div className="modal-body">
          {/* Barra de progresso única destacada de 0% a 100% */}
          <div className="details-progress-highlight">
            <div className="details-progress-header">
              <div className="details-pct-large">
                {formatPercent(download.progress, 1, language)}
              </div>
              <div className="details-progress-metrics">
                <span className={`status-badge status-${download.status}`}>
                  {statusLabel(download.status, language)}
                </span>
                <span className="metric-item">
                  <strong>{t.detailsModal.downloadedLabel}</strong>{" "}
                  {formatBytes(download.downloadedSize, language)} {t.detailsModal.ofLabel}{" "}
                  {download.totalSize > 0
                    ? formatBytes(download.totalSize, language)
                    : t.detailsModal.unknownSize}
                </span>
                <span className="metric-item">
                  <strong>{t.detailsModal.speedLabel}</strong>{" "}
                  {formatSpeed(download.speedBps, language)}
                </span>
                <span className="metric-item">
                  <strong>{t.detailsModal.remainingLabel}</strong> {formatETA(download.etaSeconds)}
                </span>
              </div>
            </div>

            <div className="details-progress-track">
              <div
                className={`details-progress-bar status-${download.status}`}
                style={{ width: `${pctClamped}%` }}
              />
            </div>
          </div>

          {download.errorMessage && (
            <div className="alert-banner danger" style={{ marginTop: 10 }}>
              {download.errorMessage}
            </div>
          )}

          {/* Botão expansível para mostrar/ocultar conexões (oculto por padrão) */}
          <div className="toggle-connections-container" style={{ marginTop: 14 }}>
            <button
              type="button"
              className="btn toggle-connections-btn"
              onClick={() => setShowConnections(!showConnections)}
            >
              <span>
                {showConnections
                  ? t.detailsModal.hideConnections
                  : t.detailsModal.showConnections}
              </span>
              <span className="badge-connections-count">
                {t.detailsModal.activeConnectionsCount(download.activeWorkers)}
              </span>
            </button>
          </div>

          {/* Área colapsável de conexões (exibida apenas se o usuário clicar para expandir) */}
          {showConnections && (
            <div className="collapsible-connections-area">
              <div className="segment-section">
                <SegmentBar
                  segments={download.segments || []}
                  totalSize={download.totalSize}
                  downloadedSize={download.downloadedSize}
                  height={18}
                  showLegend={false}
                />
              </div>

              <div className="segment-table-wrap">
                <table className="segment-table">
                  <thead>
                    <tr>
                      <th>#</th>
                      <th>{t.detailsModal.colConnection}</th>
                      <th>{t.detailsModal.colDownloaded}</th>
                      <th>{t.detailsModal.colSpeed}</th>
                      <th>{t.detailsModal.colStatus}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(download.segments || []).map((seg, i) => {
                      const segTotal = Math.max(0, seg.end - seg.start);
                      const segDone = Math.max(0, seg.current - seg.start);
                      const pct = formatPercent(
                        segTotal > 0 ? (segDone * 100) / segTotal : 0,
                        1,
                        language
                      );
                      return (
                        <tr key={`${seg.start}-${i}`}>
                          <td>#{i + 1}</td>
                          <td>
                            {seg.workerId > 0 ? t.detailsModal.connectionNum(seg.workerId) : "—"}
                          </td>
                          <td className="mono-cell">
                            {formatBytes(segDone, language)} ({pct})
                          </td>
                          <td className="mono-cell">{formatSpeed(seg.speedBps, language)}</td>
                          <td>
                            {seg.status === "downloading" && (
                              <span className="info-tag">{t.detailsModal.segDownloading}</span>
                            )}
                            {seg.status === "completed" && (
                              <span className="done-tag">{t.detailsModal.segCompleted}</span>
                            )}
                            {seg.status === "pending" && (
                              <span className="muted-text">
                                {seg.lastError
                                  ? `${t.detailsModal.segWaiting} (${seg.lastError})`
                                  : t.detailsModal.segWaiting}
                              </span>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>

        {/* Rodapé com botão Excluir e controles de status */}
        <div className="modal-footer">
          <div className="footer-left">
            <button className="btn" onClick={() => onOpenFolder(download.id)}>
              {t.detailsModal.openFolderBtn}
            </button>
            {download.status === "completed" && (
              <button className="btn primary" onClick={() => onOpenFile(download.id)}>
                {t.detailsModal.openFileBtn}
              </button>
            )}
            {/* Botão Excluir que remove download e arquivo do disco (Requisito R6) */}
            <button
              className="btn danger"
              onClick={handleDelete}
              disabled={deleting}
              title={t.detailsModal.deleteTitle}
            >
              {t.detailsModal.deleteBtn}
            </button>
          </div>

          <div className="footer-right">
            {isRunning ? (
              <button className="btn warn" onClick={() => onPause(download.id)}>
                {t.detailsModal.pauseBtn}
              </button>
            ) : (
              download.status !== "completed" && (
                <button className="btn primary" onClick={() => onStart(download.id)}>
                  {t.detailsModal.resumeBtn}
                </button>
              )
            )}
            {download.status !== "completed" && (
              <button className="btn danger" onClick={() => onCancel(download.id)}>
                {t.detailsModal.stopBtn}
              </button>
            )}
            <button className="btn" onClick={onClose}>
              {t.detailsModal.closeBtn}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
