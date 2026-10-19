import React from "react";
import { formatCategoryName, formatQueueName, useI18n } from "../i18n";
import { Category, Download, Queue } from "../types";

export interface SidebarFilter {
  type: "all" | "status" | "category" | "queue";
  value: string;
}

interface SidebarProps {
  categories: Category[];
  queues: Queue[];
  downloads: Download[];
  filter: SidebarFilter;
  onSelectFilter: (f: SidebarFilter) => void;
  onToggleQueue: (q: Queue) => void;
  onOpenCategoryFolder: (cat: Category) => void;
}

const CAT_ICONS: Record<string, string> = {
  compressed: "📦",
  documents: "📄",
  music: "🎵",
  programs: "💻",
  video: "🎬",
  general: "📂"
};

// Painel lateral de navegação com categorias, status e filas (PT-BR / EN-US)
export const Sidebar: React.FC<SidebarProps> = ({
  categories,
  queues,
  downloads,
  filter,
  onSelectFilter,
  onToggleQueue,
  onOpenCategoryFolder
}) => {
  const { language, t } = useI18n();

  // Contadores rápidos por categoria e fila
  const countByCat = (catId: string) => downloads.filter((d) => d.categoryId === catId).length;
  const countByQueue = (qId: string) => downloads.filter((d) => d.queueId === qId).length;
  const unfinishedCount = downloads.filter((d) => d.status !== "completed").length;
  const completedCount = downloads.filter((d) => d.status === "completed").length;

  return (
    <aside className="sidebar">
      {/* Seção de Categorias */}
      <div className="sidebar-section">
        <div className="sidebar-heading">{t.sidebar.categoriesHeading}</div>
        <button
          className={`sidebar-item ${filter.type === "all" ? "active" : ""}`}
          onClick={() => onSelectFilter({ type: "all", value: "all" })}
        >
          <span>{t.sidebar.allDownloads}</span>
          <span className="sidebar-count">{downloads.length}</span>
        </button>

        {categories.map((cat) => {
          const isActive = filter.type === "category" && filter.value === cat.id;
          const icon = CAT_ICONS[cat.id] || "📁";
          const displayName = formatCategoryName(cat, language);
          return (
            <div key={cat.id} className={`sidebar-cat-row ${isActive ? "active" : ""}`}>
              <button
                className="sidebar-item sub"
                onClick={() => onSelectFilter({ type: "category", value: cat.id })}
                title={t.sidebar.filterCategoryTitle(displayName)}
              >
                <span>
                  {icon} {displayName}
                </span>
                <span className="sidebar-count">{countByCat(cat.id)}</span>
              </button>
              <button
                className="cat-folder-btn"
                onClick={() => onOpenCategoryFolder(cat)}
                title={t.sidebar.openCategoryFolderTitle(displayName)}
              >
                📂
              </button>
            </div>
          );
        })}
      </div>

      {/* Seção de Status */}
      <div className="sidebar-section">
        <div className="sidebar-heading">{t.sidebar.statusHeading}</div>
        <button
          className={`sidebar-item ${
            filter.type === "status" && filter.value === "unfinished" ? "active" : ""
          }`}
          onClick={() => onSelectFilter({ type: "status", value: "unfinished" })}
        >
          <span>{t.sidebar.inProgress}</span>
          <span className="sidebar-count">{unfinishedCount}</span>
        </button>

        <button
          className={`sidebar-item ${
            filter.type === "status" && filter.value === "completed" ? "active" : ""
          }`}
          onClick={() => onSelectFilter({ type: "status", value: "completed" })}
        >
          <span>{t.sidebar.completed}</span>
          <span className="sidebar-count">{completedCount}</span>
        </button>
      </div>

      {/* Seção de Filas */}
      <div className="sidebar-section">
        <div className="sidebar-heading">{t.sidebar.queuesHeading}</div>
        {queues.map((q) => {
          const qName = formatQueueName(q, language);
          return (
            <div
              key={q.id}
              className={`sidebar-queue-row ${
                filter.type === "queue" && filter.value === q.id ? "active" : ""
              }`}
            >
              <button
                className="sidebar-queue-btn"
                onClick={() => onSelectFilter({ type: "queue", value: q.id })}
              >
                <span>
                  {q.isSync ? "🔄 " : "📋 "}
                  {qName}
                </span>
                <span className="sidebar-count">{countByQueue(q.id)}</span>
              </button>
              <button
                className={`queue-power-btn ${q.active ? "running" : ""}`}
                onClick={() => onToggleQueue(q)}
                title={q.active ? t.sidebar.queueRunningTitle : t.sidebar.queueStoppedTitle}
              >
                {q.active ? t.sidebar.queueActiveBadge : t.sidebar.queueStartBtn}
              </button>
            </div>
          );
        })}
      </div>
    </aside>
  );
};
