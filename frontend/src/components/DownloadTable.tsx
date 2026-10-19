import React, { useState } from "react";
import { formatCategoryName, useI18n } from "../i18n";
import { Category, Download, Queue } from "../types";
import {
  formatBytes,
  formatDate,
  formatETA,
  formatPercent,
  formatSpeedStable,
  statusLabel
} from "../utils/format";

// Campos disponíveis para ordenação na tabela
type SortField = "fileName" | "totalSize" | "progress" | "speedBps" | "etaSeconds" | "createdAt";

// Chaves das 8 colunas da tabela principal
type ColumnKey =
  | "fileName"
  | "totalSize"
  | "progress"
  | "speedBps"
  | "etaSeconds"
  | "category"
  | "createdAt"
  | "actions";

// Larguras padrão em pixels para table-layout: fixed
const DEFAULT_COL_WIDTHS: Record<ColumnKey, number> = {
  fileName: 250,
  totalSize: 100,
  progress: 165,
  speedBps: 90,
  etaSeconds: 85,
  category: 95,
  createdAt: 105,
  actions: 125
};

interface DownloadTableProps {
  downloads: Download[];
  categories: Category[];
  queues: Queue[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onOpenDetails: (id: string) => void;
  onStart: (id: string) => void;
  onPause: (id: string) => void;
  onDelete?: (id: string) => void;
  onOpenFolder: (id: string) => void;
  onOpenFile: (id: string) => void;
  onDropURL: (url: string) => void;
}

// Tabela principal de downloads com 8 colunas, redimensionamento interativo e barra de progresso contínua.
export const DownloadTable: React.FC<DownloadTableProps> = ({
  downloads,
  categories,
  selectedId,
  onSelect,
  onOpenDetails,
  onStart,
  onPause,
  onDelete,
  onOpenFolder,
  onOpenFile,
  onDropURL
}) => {
  const { language, t } = useI18n();

  // Estado de ordenação das colunas
  const [sortField, setSortField] = useState<SortField>("createdAt");
  const [sortAsc, setSortAsc] = useState<boolean>(false);
  const [dragActive, setDragActive] = useState<boolean>(false);

  // Estado de largura interativa das 8 colunas (table-layout: fixed)
  const [colWidths, setColWidths] = useState<Record<ColumnKey, number>>(DEFAULT_COL_WIDTHS);

  // Alterna o campo e direção de ordenação
  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortAsc(!sortAsc);
    } else {
      setSortField(field);
      setSortAsc(true);
    }
  };

  // Inicia o redimensionamento de coluna ao arrastar a alça lateral (.col-resizer)
  const handleResizeStart = (colKey: ColumnKey, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const startX = e.clientX;
    const startW = colWidths[colKey] || 100;

    const onMouseMove = (ev: MouseEvent) => {
      const delta = ev.clientX - startX;
      setColWidths((prev) => ({
        ...prev,
        [colKey]: Math.max(45, startW + delta)
      }));
    };

    const onMouseUp = () => {
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
    };

    window.addEventListener("mousemove", onMouseMove);
    window.addEventListener("mouseup", onMouseUp);
  };

  // Ordenação dos downloads conforme campo selecionado
  const sorted = [...downloads].sort((a, b) => {
    let cmp = 0;
    switch (sortField) {
      case "fileName":
        cmp = a.fileName.localeCompare(b.fileName);
        break;
      case "totalSize":
        cmp = a.totalSize - b.totalSize;
        break;
      case "progress":
        cmp = a.progress - b.progress;
        break;
      case "speedBps":
        cmp = a.speedBps - b.speedBps;
        break;
      case "etaSeconds":
        cmp = a.etaSeconds - b.etaSeconds;
        break;
      case "createdAt":
      default:
        cmp = new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
        break;
    }
    return sortAsc ? cmp : -cmp;
  });

  // Captura de URL arrastada para dentro da tabela (Drag and Drop)
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragActive(false);
    const uriList = e.dataTransfer.getData("text/uri-list") || e.dataTransfer.getData("text/plain");
    if (uriList) {
      const firstLine = uriList
        .split(/\r?\n/)
        .map((l) => l.trim())
        .find((l) => l.startsWith("http://") || l.startsWith("https://") || l.startsWith("ftp://"));
      if (firstLine) {
        onDropURL(firstLine);
      }
    }
  };

  // Busca o nome legível da categoria no idioma ativo
  const getCatName = (cId: string) => {
    const found = categories.find((c) => c.id === cId);
    return formatCategoryName(found || { id: cId || "general", name: "" }, language);
  };

  return (
    <div
      className={`table-container ${dragActive ? "drag-over" : ""}`}
      onDragOver={(e) => {
        e.preventDefault();
        setDragActive(true);
      }}
      onDragLeave={() => setDragActive(false)}
      onDrop={handleDrop}
    >
      {dragActive && <div className="drop-overlay">{t.table.dropOverlay}</div>}

      <table className="download-table">
        {/* Definição de colunas com larguras em pixels para layout fixo */}
        <colgroup>
          <col style={{ width: `${colWidths.fileName}px` }} />
          <col style={{ width: `${colWidths.totalSize}px` }} />
          <col style={{ width: `${colWidths.progress}px` }} />
          <col style={{ width: `${colWidths.speedBps}px` }} />
          <col style={{ width: `${colWidths.etaSeconds}px` }} />
          <col style={{ width: `${colWidths.category}px` }} />
          <col style={{ width: `${colWidths.createdAt}px` }} />
          <col style={{ width: `${colWidths.actions}px` }} />
        </colgroup>

        <thead>
          <tr>
            {/* Cabeçalho 1: Arquivo */}
            <th onClick={() => handleSort("fileName")} title={t.table.colFileTitle}>
              <span className="th-label">{t.table.colFile}</span>
              <span
                className="col-resizer"
                onMouseDown={(e) => handleResizeStart("fileName", e)}
                title={t.table.resizeTitle}
              />
            </th>

            {/* Cabeçalho 2: Tamanho */}
            <th onClick={() => handleSort("totalSize")} title={t.table.colSizeTitle}>
              <span className="th-label">{t.table.colSize}</span>
              <span
                className="col-resizer"
                onMouseDown={(e) => handleResizeStart("totalSize", e)}
                title={t.table.resizeTitle}
              />
            </th>

            {/* Cabeçalho 3: Progresso */}
            <th onClick={() => handleSort("progress")} title={t.table.colProgressTitle}>
              <span className="th-label">{t.table.colProgress}</span>
              <span
                className="col-resizer"
                onMouseDown={(e) => handleResizeStart("progress", e)}
                title={t.table.resizeTitle}
              />
            </th>

            {/* Cabeçalho 4: Velocidade */}
            <th onClick={() => handleSort("speedBps")} title={t.table.colSpeedTitle}>
              <span className="th-label">{t.table.colSpeed}</span>
              <span
                className="col-resizer"
                onMouseDown={(e) => handleResizeStart("speedBps", e)}
                title={t.table.resizeTitle}
              />
            </th>

            {/* Cabeçalho 5: Restante */}
            <th onClick={() => handleSort("etaSeconds")} title={t.table.colRemainingTitle}>
              <span className="th-label">{t.table.colRemaining}</span>
              <span
                className="col-resizer"
                onMouseDown={(e) => handleResizeStart("etaSeconds", e)}
                title={t.table.resizeTitle}
              />
            </th>

            {/* Cabeçalho 6: Categoria */}
            <th>
              <span className="th-label">{t.table.colCategory}</span>
              <span
                className="col-resizer"
                onMouseDown={(e) => handleResizeStart("category", e)}
                title={t.table.resizeTitle}
              />
            </th>

            {/* Cabeçalho 7: Adicionado */}
            <th onClick={() => handleSort("createdAt")} title={t.table.colAddedTitle}>
              <span className="th-label">{t.table.colAdded}</span>
              <span
                className="col-resizer"
                onMouseDown={(e) => handleResizeStart("createdAt", e)}
                title={t.table.resizeTitle}
              />
            </th>

            {/* Cabeçalho 8: Ações */}
            <th>
              <span className="th-label">{t.table.colActions}</span>
              <span
                className="col-resizer"
                onMouseDown={(e) => handleResizeStart("actions", e)}
                title={t.table.resizeTitle}
              />
            </th>
          </tr>
        </thead>

        <tbody>
          {sorted.length === 0 ? (
            <tr>
              <td colSpan={8} className="empty-state">
                {t.table.emptyPrefix}
                <b>{t.table.emptyAction}</b>
                {t.table.emptySuffix}
              </td>
            </tr>
          ) : (
            sorted.map((d) => {
              const isSelected = d.id === selectedId;
              const isRunning = d.status === "downloading" || d.status === "probing";
              const pctClamped = Math.min(100, Math.max(0, d.progress));

              return (
                <tr
                  key={d.id}
                  className={`${isSelected ? "selected" : ""}`}
                  onClick={() => onSelect(d.id)}
                  onDoubleClick={() => onOpenDetails(d.id)}
                >
                  {/* Coluna 1: Arquivo (Truncamento com reticências) */}
                  <td className="file-cell" title={`${d.fileName}\n${d.url}`}>
                    <div className="file-title">{d.fileName}</div>
                    <div className="file-url">{d.url}</div>
                  </td>

                  {/* Coluna 2: Tamanho */}
                  <td title={`${formatBytes(d.downloadedSize, language)} ${t.table.downloadedSuffix}`}>
                    <div>
                      {d.totalSize > 0
                        ? formatBytes(d.totalSize, language)
                        : t.table.unknownSize}
                    </div>
                    <small className="muted-text">
                      {formatBytes(d.downloadedSize, language)} {t.table.downloadedSuffix}
                    </small>
                  </td>

                  {/* Coluna 3: Progresso (Barra contínua única de 0% a 100%) */}
                  <td>
                    <div className="status-row">
                      <span className={`status-badge status-${d.status}`}>
                        {statusLabel(d.status, language)}
                      </span>
                      <span className="pct-text">{formatPercent(d.progress, 1, language)}</span>
                    </div>
                    {/* Barra de progresso contínua conforme Requisito R4 */}
                    <div className="main-progress-track">
                      <div
                        className={`main-progress-bar status-${d.status}`}
                        style={{ width: `${pctClamped}%` }}
                      />
                    </div>
                    {d.errorMessage && (
                      <div className="error-hint" title={d.errorMessage}>
                        {d.errorMessage}
                      </div>
                    )}
                  </td>

                  {/* Coluna 4: Velocidade */}
                  <td className="mono-cell">{formatSpeedStable(d.speedBps, isRunning, language)}</td>

                  {/* Coluna 5: Restante */}
                  <td className="mono-cell">{formatETA(d.etaSeconds)}</td>

                  {/* Coluna 6: Categoria */}
                  <td title={getCatName(d.categoryId)}>
                    <div>{getCatName(d.categoryId)}</div>
                  </td>

                  {/* Coluna 7: Adicionado */}
                  <td className="muted-text">{formatDate(d.createdAt, language)}</td>

                  {/* Coluna 8: Ações */}
                  <td className="row-actions" onClick={(e) => e.stopPropagation()}>
                    {isRunning ? (
                      <button
                        className="mini-btn"
                        onClick={() => onPause(d.id)}
                        title={t.table.pauseTitle}
                      >
                        ⏸
                      </button>
                    ) : d.status !== "completed" ? (
                      <button
                        className="mini-btn"
                        onClick={() => onStart(d.id)}
                        title={t.table.startResumeTitle}
                      >
                        ▶
                      </button>
                    ) : (
                      <button
                        className="mini-btn"
                        onClick={() => onOpenFile(d.id)}
                        title={t.table.openFileTitle}
                      >
                        📄
                      </button>
                    )}

                    <button
                      className="mini-btn"
                      onClick={() => onOpenFolder(d.id)}
                      title={t.table.openFolderTitle}
                    >
                      📂
                    </button>

                    <button
                      className="mini-btn"
                      onClick={() => onOpenDetails(d.id)}
                      title={t.table.detailsTitle}
                    >
                      📊
                    </button>

                    {onDelete && (
                      <button
                        className="mini-btn danger-mini"
                        onClick={() => onDelete(d.id)}
                        title={t.table.deleteTitle}
                      >
                        🗑
                      </button>
                    )}
                  </td>
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </div>
  );
};
