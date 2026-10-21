import React from "react";
import { useI18n } from "../i18n";
import { formatSpeedStable } from "../utils/format";

interface ToolbarProps {
  selectedId: string | null;
  canResume: boolean;
  canPause: boolean;
  totalSpeedBps: number;
  activeDownloadsCount: number;
  searchQuery: string;
  onSearchChange: (q: string) => void;
  onOpenAddModal: () => void;
  onSwitchToCardMode?: () => void;
  onResumeSelected: () => void;
  onPauseSelected: () => void;
  onPauseAll: () => void;
  onDeleteSelected: () => void;
  onDeleteCompleted: () => void;
  onOpenScheduler: () => void;
  onStartMainQueue?: () => void;
  onStopMainQueue?: () => void;
  onOpenSettings: () => void;
  onOpenInstaller: () => void;
  onOpenUpdater: () => void;
  updateAvailable?: boolean;
  onHideToTray: () => void;
}

// Barra de ferramentas principal do OpenIDM (PT-BR / EN-US)
export const Toolbar: React.FC<ToolbarProps> = ({
  selectedId,
  canResume,
  canPause,
  totalSpeedBps,
  activeDownloadsCount,
  searchQuery,
  onSearchChange,
  onOpenAddModal,
  onSwitchToCardMode,
  onResumeSelected,
  onPauseSelected,
  onPauseAll,
  onDeleteSelected,
  onDeleteCompleted,
  onOpenScheduler,
  onOpenSettings,
  onOpenInstaller,
  onOpenUpdater,
  updateAvailable,
  onHideToTray
}) => {
  const { language, t, toggleLanguage } = useI18n();

  return (
    <header className="toolbar">
      <div className="toolbar-actions">
        {/* Identificação de versão oficial alinhada em v1.0.1 */}
        <div className="brand-pill" title={t.toolbar.brandTooltip}>
          <span
            className="brand-terminal-badge"
            style={{
              display: "inline-flex",
              alignItems: "center",
              justifyContent: "center",
              padding: 0,
              overflow: "hidden",
              background: "#F4B601"
            }}
          >
            <img
              src="/openidm_icon.jpg"
              alt="OpenIDM"
              style={{ width: "100%", height: "100%", objectFit: "cover" }}
            />
          </span>
          <span className="brand-title">OpenIDM</span>
          <span className="brand-tag">v1.0.1</span>
        </div>

        {/* Botão principal para adicionar download no Card Solto */}
        <button
          className="tool-btn primary"
          onClick={onOpenAddModal}
          title={t.toolbar.captureLinkTitle}
        >
          <span className="tool-icon">＋</span>
          <span>{t.toolbar.captureLink}</span>
        </button>

        <div className="toolbar-divider" />

        {/* Controles de execução do item selecionado */}
        <button
          className="tool-btn"
          disabled={!selectedId || !canResume}
          onClick={onResumeSelected}
          title={t.toolbar.resumeTitle}
        >
          <span className="tool-icon success-text">▶</span>
          <span>{t.toolbar.resume}</span>
        </button>

        <button
          className="tool-btn"
          disabled={!selectedId || !canPause}
          onClick={onPauseSelected}
          title={t.toolbar.pauseTitle}
        >
          <span className="tool-icon warn-text">⏸</span>
          <span>{t.toolbar.pause}</span>
        </button>

        <button className="tool-btn" onClick={onPauseAll} title={t.toolbar.stopAllTitle}>
          <span className="tool-icon danger-text">⏹</span>
          <span>{t.toolbar.stopAll}</span>
        </button>

        <div className="toolbar-divider" />

        {/* Ações de exclusão e limpeza */}
        <button
          className="tool-btn"
          disabled={!selectedId}
          onClick={onDeleteSelected}
          title={t.toolbar.deleteTitle}
        >
          <span className="tool-icon">🗑</span>
          <span>{t.toolbar.delete}</span>
        </button>

        <button
          className="tool-btn"
          onClick={onDeleteCompleted}
          title={t.toolbar.clearCompletedTitle}
        >
          <span className="tool-icon">🧹</span>
          <span>{t.toolbar.clearCompleted}</span>
        </button>

        <div className="toolbar-divider" />

        {/* Agendador, Opções e Integração */}
        <button className="tool-btn" onClick={onOpenScheduler} title={t.toolbar.schedulerTitle}>
          <span className="tool-icon accent-text">📅</span>
          <span>{t.toolbar.scheduler}</span>
        </button>

        <button
          className="tool-btn"
          onClick={onOpenSettings}
          title={t.toolbar.optionsTitle}
        >
          <span className="tool-icon">⚙</span>
          <span>{t.toolbar.options}</span>
        </button>

        <button
          className="tool-btn"
          onClick={onOpenInstaller}
          title={t.toolbar.installerTitle}
        >
          <span className="tool-icon accent-text">📦</span>
          <span>{t.toolbar.installer}</span>
        </button>

        <button
          className="tool-btn"
          onClick={onOpenUpdater}
          title={t.toolbar.updateTitle}
          style={
            updateAvailable
              ? {
                  borderColor: "rgba(244, 182, 1, 0.55)",
                  background: "rgba(244, 182, 1, 0.12)"
                }
              : undefined
          }
        >
          <span className="tool-icon accent-text">🔄</span>
          <span>{t.toolbar.update}</span>
          {updateAvailable && (
            <span
              style={{
                marginLeft: 4,
                padding: "1px 6px",
                borderRadius: 999,
                fontSize: 10,
                fontWeight: 700,
                background: "#F4B601",
                color: "#111"
              }}
            >
              {t.toolbar.updateAvailableBadge}
            </span>
          )}
        </button>
      </div>

      <div className="toolbar-right">
        {/* Indicador de velocidade total estabilizado */}
        <div className="speed-pill" title={t.toolbar.speedTooltip}>
          <span className="speed-dot" data-active={activeDownloadsCount > 0} />
          <span>
            {activeDownloadsCount > 0
              ? t.toolbar.activeCountLabel(
                  activeDownloadsCount,
                  formatSpeedStable(totalSpeedBps, true, language)
                )
              : t.toolbar.idleLabel}
          </span>
        </div>

        {/* Campo de busca rápida */}
        <input
          type="search"
          className="search-input"
          placeholder={t.toolbar.searchPlaceholder}
          value={searchQuery}
          onChange={(e) => onSearchChange(e.target.value)}
        />

        {/* Botão rápido de troca de idioma (PT-BR / EN-US) */}
        <button
          type="button"
          className="tool-btn"
          onClick={toggleLanguage}
          title={t.toolbar.languageToggleTitle}
        >
          <span>🌐 {language === "en-US" ? "EN" : "PT"}</span>
        </button>

        {/* Alternar para o Card Solto mantendo o app principal minimizado no relógio */}
        {onSwitchToCardMode && (
          <button
            className="tool-btn"
            onClick={onSwitchToCardMode}
            title={t.toolbar.standaloneCardTitle}
          >
            <span>{t.toolbar.standaloneCardBtn}</span>
          </button>
        )}

        {/* Minimizar para bandeja do sistema */}
        <button
          className="tool-btn tray-btn"
          onClick={onHideToTray}
          title={t.toolbar.minimizeToTrayTitle}
        >
          <span>{t.toolbar.minimizeToTray}</span>
        </button>
      </div>
    </header>
  );
};
