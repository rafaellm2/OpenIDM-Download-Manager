import React, { useCallback, useEffect, useState } from "react";
import { useI18n } from "../i18n";
import { api } from "../services/api";
import { UpdateInfo, UpdateProgress } from "../types";
import { formatBytes } from "../utils/format";

interface UpdateModalProps {
  initialInfo?: UpdateInfo | null;
  onClose: () => void;
  onUpdateInfoChanged?: (info: UpdateInfo | null) => void;
}

// Modal de verificação e aplicação de atualizações automáticas via GitHub Releases.
export const UpdateModal: React.FC<UpdateModalProps> = ({
  initialInfo,
  onClose,
  onUpdateInfoChanged
}) => {
  const { t } = useI18n();
  const [info, setInfo] = useState<UpdateInfo | null>(initialInfo || null);
  const [checking, setChecking] = useState<boolean>(!initialInfo);
  const [updating, setUpdating] = useState<boolean>(false);
  const [progress, setProgress] = useState<UpdateProgress | null>(null);
  const [errorMsg, setErrorMsg] = useState<string>("");
  const [statusBanner, setStatusBanner] = useState<string>("");

  const runCheck = useCallback(
    async (manual: boolean) => {
      setChecking(true);
      setErrorMsg("");
      setStatusBanner("");
      try {
        const res = await api.checkForUpdates(manual);
        setInfo(res);
        onUpdateInfoChanged?.(res);
      } catch (err: any) {
        setErrorMsg(String(err?.message || err || "Erro ao consultar GitHub Releases"));
      } finally {
        setChecking(false);
      }
    },
    [onUpdateInfoChanged]
  );

  useEffect(() => {
    if (!initialInfo) {
      runCheck(true);
    } else {
      setInfo(initialInfo);
    }

    const unsub = api.subscribeEvents((ev) => {
      if (ev.type === "update_progress" && ev.payload) {
        setProgress(ev.payload as UpdateProgress);
      }
    });

    return () => unsub();
  }, [initialInfo, runCheck]);

  // Inicia o download do instalador/executável do novo release e executa a atualização
  const handleUpdateNow = async () => {
    if (!info) return;
    setUpdating(true);
    setErrorMsg("");
    setStatusBanner("");
    setProgress({
      downloadedBytes: 0,
      totalBytes: info.assetSize || 0,
      percent: 2,
      status: "downloading"
    });

    try {
      const res: any = await api.startSelfUpdate(info.downloadUrl || "", info.assetName || "");
      if (res?.status === "opened_release_page" || res?.openedBrowser) {
        setUpdating(false);
        onClose();
        return;
      }
      setProgress((prev) => ({
        downloadedBytes: prev?.totalBytes || 0,
        totalBytes: prev?.totalBytes || 0,
        percent: 100,
        status: "launching"
      }));
      setStatusBanner(t.updateModal.launchingMsg);
    } catch (err: any) {
      setUpdating(false);
      setErrorMsg(String(err?.message || err));
    }
  };

  // Adia a notificação automática por 24 horas
  const handlePostpone24h = async () => {
    try {
      const until = await api.postponeUpdate();
      if (info) {
        const updated: UpdateInfo = {
          ...info,
          postponed: true,
          postponedUntil: until
        };
        setInfo(updated);
        onUpdateInfoChanged?.(updated);
      }
      setStatusBanner(t.updateModal.postponedMsg);
      setTimeout(() => {
        onClose();
      }, 600);
    } catch (err: any) {
      setErrorMsg(String(err?.message || err));
    }
  };

  const handleOpenGitHub = () => {
    const url =
      info?.releaseUrl || "https://github.com/rafaellm2/OpenIDM-Download-Manager/releases";
    api.openExternalURL(url).catch(() => {});
  };

  const pct = progress ? Math.min(100, Math.max(2, Math.round(progress.percent || 0))) : 0;

  return (
    <div className="modal-backdrop" onClick={() => !updating && onClose()}>
      <div
        className="modal-window"
        style={{ width: 540, maxWidth: "95vw" }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <div>
            <h3 style={{ margin: 0, display: "flex", alignItems: "center", gap: 8 }}>
              <span>🔄</span>
              <span>{t.updateModal.title}</span>
            </h3>
            <div style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 2 }}>
              {t.updateModal.subtitle}
            </div>
          </div>
          <button
            className="icon-close-btn"
            disabled={updating}
            onClick={onClose}
            title={t.updateModal.closeBtn}
          >
            ✕
          </button>
        </div>

        <div className="modal-body" style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          {checking && (
            <div
              style={{
                padding: "22px 16px",
                textAlign: "center",
                background: "rgba(255,255,255,0.03)",
                borderRadius: 10,
                border: "1px solid var(--border-subtle)"
              }}
            >
              <div style={{ fontSize: 14, fontWeight: 600, color: "var(--text-primary)" }}>
                ⏳ {t.updateModal.checkingMsg}
              </div>
            </div>
          )}

          {!checking && info && !info.available && (
            <div
              style={{
                padding: "18px 16px",
                borderRadius: 10,
                background: "rgba(16, 185, 129, 0.08)",
                border: "1px solid rgba(16, 185, 129, 0.3)"
              }}
            >
              <div style={{ fontSize: 15, fontWeight: 700, color: "#10b981", marginBottom: 4 }}>
                {t.updateModal.upToDateTitle}
              </div>
              <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
                {t.updateModal.upToDateDesc(info.currentVersion || "1.0.1")}
              </div>
            </div>
          )}

          {!checking && info && info.available && (
            <>
              <div
                style={{
                  padding: "14px 16px",
                  borderRadius: 10,
                  background: "rgba(244, 182, 1, 0.1)",
                  border: "1px solid rgba(244, 182, 1, 0.35)"
                }}
              >
                <div style={{ fontSize: 15, fontWeight: 700, color: "#F4B601", marginBottom: 4 }}>
                  {t.updateModal.availableTitle(info.latestVersion)}
                </div>
                <div style={{ fontSize: 12.5, color: "var(--text-secondary)" }}>
                  {t.updateModal.availableSubtitle(info.currentVersion, info.latestVersion)}
                </div>
                {info.assetName && (
                  <div style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 6 }}>
                    📦{" "}
                    {t.updateModal.assetInfoLabel(
                      info.assetName,
                      info.assetSize > 0 ? formatBytes(info.assetSize) : ""
                    )}
                  </div>
                )}
              </div>

              <div>
                <div
                  style={{
                    fontSize: 12,
                    fontWeight: 600,
                    color: "var(--text-secondary)",
                    marginBottom: 6
                  }}
                >
                  {t.updateModal.releaseNotesLabel}
                </div>
                <div
                  style={{
                    maxHeight: 160,
                    overflowY: "auto",
                    padding: "10px 12px",
                    borderRadius: 8,
                    background: "rgba(0, 0, 0, 0.25)",
                    border: "1px solid var(--border-subtle)",
                    fontSize: 12.5,
                    lineHeight: 1.5,
                    whiteSpace: "pre-wrap",
                    color: "var(--text-primary)"
                  }}
                >
                  {info.releaseNotes?.trim() || t.updateModal.noReleaseNotes}
                </div>
              </div>
            </>
          )}

          {updating && progress && (
            <div
              style={{
                padding: "12px 14px",
                borderRadius: 8,
                background: "rgba(59, 130, 246, 0.08)",
                border: "1px solid rgba(59, 130, 246, 0.3)"
              }}
            >
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  fontSize: 12.5,
                  fontWeight: 600,
                  marginBottom: 6
                }}
              >
                <span>
                  {progress.status === "launching" || (progress as any).status === "applying"
                    ? t.updateModal.launchingMsg
                    : t.updateModal.downloadingMsg(String(pct))}
                </span>
                <span>{pct}%</span>
              </div>
              <div
                style={{
                  height: 8,
                  borderRadius: 999,
                  background: "rgba(255,255,255,0.08)",
                  overflow: "hidden"
                }}
              >
                <div
                  style={{
                    width: `${pct}%`,
                    height: "100%",
                    background: "linear-gradient(90deg, #F4B601, #10b981)",
                    transition: "width 0.2s ease"
                  }}
                />
              </div>
            </div>
          )}

          {statusBanner && (
            <div
              style={{
                padding: "10px 12px",
                borderRadius: 8,
                background: "rgba(16, 185, 129, 0.12)",
                border: "1px solid rgba(16, 185, 129, 0.35)",
                color: "#10b981",
                fontSize: 12.5,
                fontWeight: 600
              }}
            >
              ✔ {statusBanner}
            </div>
          )}

          {errorMsg && (
            <div
              style={{
                padding: "10px 12px",
                borderRadius: 8,
                background: "rgba(239, 68, 68, 0.12)",
                border: "1px solid rgba(239, 68, 68, 0.35)",
                color: "#f87171",
                fontSize: 12.5
              }}
            >
              ⚠ {errorMsg}
            </div>
          )}
        </div>

        <div
          className="modal-footer"
          style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8 }}
        >
          <div style={{ display: "flex", gap: 8 }}>
            <button
              type="button"
              className="btn secondary"
              disabled={checking || updating}
              onClick={() => runCheck(true)}
            >
              {t.updateModal.checkAgainBtn}
            </button>
            <button
              type="button"
              className="btn secondary"
              disabled={updating}
              onClick={handleOpenGitHub}
            >
              {t.updateModal.viewOnGitHubBtn}
            </button>
          </div>

          <div style={{ display: "flex", gap: 8 }}>
            {info?.available && (
              <>
                <button
                  type="button"
                  className="btn secondary"
                  disabled={updating}
                  onClick={handlePostpone24h}
                  title={t.updateModal.postpone24hTitle}
                >
                  {t.updateModal.postpone24hBtn}
                </button>
                <button
                  type="button"
                  className="btn primary"
                  disabled={updating}
                  onClick={handleUpdateNow}
                >
                  {updating ? t.updateModal.updatingBtn : t.updateModal.updateNowBtn}
                </button>
              </>
            )}
            {!info?.available && (
              <button
                type="button"
                className="btn secondary"
                disabled={updating}
                onClick={onClose}
              >
                {t.updateModal.closeBtn}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
