import React, { useEffect, useState } from "react";
import { useI18n } from "../i18n";
import { api } from "../services/api";
import { InstallationStatus, InstallOptions, InstallResult } from "../types";

interface InstallerModalProps {
  onClose: () => void;
  onInstalled: () => void;
}

// Assistente Profissional de Instalação do Sistema e Extensão do Navegador com suporte bilíngue.
export const InstallerModal: React.FC<InstallerModalProps> = ({ onClose, onInstalled }) => {
  const { language, toggleLanguage, t } = useI18n();
  const [status, setStatus] = useState<InstallationStatus | null>(null);
  const [installDir, setInstallDir] = useState("");
  const [startWithWindows, setStartWithWindows] = useState(true);
  const [browserIntegration, setBrowserIntegration] = useState(true);
  const [createShortcuts, setCreateShortcuts] = useState(true);
  const [launchAppAfter, setLaunchAppAfter] = useState(false);
  const [chromeExtId, setChromeExtId] = useState("");

  const [installing, setInstalling] = useState(false);
  const [progressStep, setProgressStep] = useState(0);
  const [progressTotal, setProgressTotal] = useState(7);
  const [progressMsg, setProgressMsg] = useState("");
  const [result, setResult] = useState<InstallResult | null>(null);
  const [errorMsg, setErrorMsg] = useState("");
  const [copiedBanner, setCopiedBanner] = useState("");

  const loadStatus = async () => {
    try {
      const st = await api.getInstallationStatus();
      setStatus(st);
      if (!installDir && st.installDir) {
        setInstallDir(st.installDir);
      }
    } catch (_e) {
      // Silencioso durante carregamento inicial
    }
  };

  useEffect(() => {
    loadStatus();

    const unsub = api.subscribeEvents((ev) => {
      if (ev.type === "installer_progress" && ev.payload) {
        setProgressStep(Number(ev.payload.step) || 1);
        setProgressTotal(Number(ev.payload.total) || 7);
        setProgressMsg(String(ev.payload.message || ""));
      }
    });

    return () => unsub();
  }, []);

  const handleBrowseInstallDir = async () => {
    const picked = await api.browseFolder(installDir);
    if (picked) {
      setInstallDir(picked);
    }
  };

  const handleRunInstaller = async () => {
    setInstalling(true);
    setErrorMsg("");
    setResult(null);
    setProgressStep(1);
    setProgressMsg(t.installerModal.startingMsg);

    try {
      const opts: InstallOptions = {
        installDir: installDir.trim(),
        language,
        startWithWindows,
        startInTray: true,
        createDesktopShortcut: createShortcuts,
        createStartMenu: createShortcuts,
        registerUninstaller: createShortcuts,
        installBrowserExt: browserIntegration,
        launchBrowserExtSetup: browserIntegration,
        launchAppAfterInstall: launchAppAfter,
        chromeExtId: chromeExtId.trim()
      };
      const res = await api.runSystemInstaller(opts);
      setResult(res);
      setProgressStep(progressTotal);
      setProgressMsg(t.installerModal.completedMsg);
      await loadStatus();
      onInstalled();
    } catch (err: any) {
      setErrorMsg(String(err?.message || err || t.installerModal.failedMsg));
    } finally {
      setInstalling(false);
    }
  };

  const handleLaunchExtension = async () => {
    try {
      await api.launchBrowserExtensionSetup();
      setCopiedBanner(t.installerModal.launchedExtBanner);
    } catch (err: any) {
      setErrorMsg(String(err?.message || err));
    }
  };

  const handleOpenExtFolder = async () => {
    try {
      const dir = await api.openExtensionFolder();
      setCopiedBanner(t.installerModal.openedExtFolderBanner(dir));
    } catch (err: any) {
      setErrorMsg(String(err?.message || err));
    }
  };

  const pct = installing
    ? Math.min(98, Math.max(12, Math.round((progressStep / Math.max(1, progressTotal)) * 100)))
    : result
    ? 100
    : 0;

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal-window large"
        style={{ maxWidth: "820px" }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <div style={{ display: "flex", alignItems: "center", gap: "14px" }}>
            <div
              style={{
                width: "44px",
                height: "44px",
                borderRadius: "12px",
                overflow: "hidden",
                background: "#F4B601",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                boxShadow: "0 6px 18px rgba(244, 182, 1, 0.35)"
              }}
            >
              <img
                src="/openidm_icon.jpg"
                alt="OpenIDM"
                style={{ width: "100%", height: "100%", objectFit: "cover" }}
              />
            </div>
            <div>
              <h3>{t.installerModal.title}</h3>
              <span className="modal-subtitle">{t.installerModal.subtitle}</span>
            </div>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
            <button
              type="button"
              className="secondary-btn small"
              onClick={toggleLanguage}
              title={t.toolbar.languageToggleTitle}
            >
              🌐 {language === "en-US" ? "EN" : "PT"}
            </button>
            <button className="close-btn" onClick={onClose}>
              ✕
            </button>
          </div>
        </div>

        <div className="modal-body">
          {errorMsg && <div className="alert-banner danger">{errorMsg}</div>}
          {copiedBanner && <div className="alert-banner success">{copiedBanner}</div>}

          {/* Painel de diagnóstico atual do sistema */}
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(4, 1fr)",
              gap: "10px",
              marginBottom: "16px"
            }}
          >
            <div
              style={{
                background: "#181816",
                border: "1px solid #2e2d29",
                borderRadius: "10px",
                padding: "10px 12px"
              }}
            >
              <div style={{ fontSize: "11px", color: "#9c988d", marginBottom: "4px" }}>
                {t.installerModal.cardAutoStart}
              </div>
              <div
                style={{
                  fontSize: "12.5px",
                  fontWeight: 700,
                  color: status?.autoStartEnabled ? "#4ade80" : "#fbbf24"
                }}
              >
                {status?.autoStartEnabled
                  ? t.installerModal.statusActive
                  : t.installerModal.statusNotConfigured}
              </div>
            </div>

            <div
              style={{
                background: "#181816",
                border: "1px solid #2e2d29",
                borderRadius: "10px",
                padding: "10px 12px"
              }}
            >
              <div style={{ fontSize: "11px", color: "#9c988d", marginBottom: "4px" }}>
                {t.installerModal.cardBrowserIntegration}
              </div>
              <div
                style={{
                  fontSize: "12.5px",
                  fontWeight: 700,
                  color: status?.nativeHostRegistered ? "#4ade80" : "#fbbf24"
                }}
              >
                {status?.nativeHostRegistered
                  ? t.installerModal.statusIntegrated
                  : t.installerModal.statusPending}
              </div>
            </div>

            <div
              style={{
                background: "#181816",
                border: "1px solid #2e2d29",
                borderRadius: "10px",
                padding: "10px 12px"
              }}
            >
              <div style={{ fontSize: "11px", color: "#9c988d", marginBottom: "4px" }}>
                {t.installerModal.cardSystemShortcuts}
              </div>
              <div
                style={{
                  fontSize: "12.5px",
                  fontWeight: 700,
                  color:
                    status?.desktopShortcutExist || status?.startMenuExist
                      ? "#4ade80"
                      : "#fbbf24"
                }}
              >
                {status?.desktopShortcutExist || status?.startMenuExist
                  ? t.installerModal.statusConfigured
                  : t.installerModal.statusPending}
              </div>
            </div>

            <div
              style={{
                background: "#181816",
                border: "1px solid #2e2d29",
                borderRadius: "10px",
                padding: "10px 12px"
              }}
            >
              <div style={{ fontSize: "11px", color: "#9c988d", marginBottom: "4px" }}>
                {t.installerModal.cardDetectedBrowsers}
              </div>
              <div style={{ fontSize: "12px", fontWeight: 600, color: "#faf9f5" }}>
                {status?.browsersDetected?.length
                  ? status.browsersDetected
                      .map((b) =>
                        b.name
                          .replace("Google ", "")
                          .replace("Microsoft ", "")
                          .replace("Mozilla ", "")
                      )
                      .join(", ")
                  : "Chrome / Edge"}
              </div>
            </div>
          </div>

          <div className="form-group" style={{ marginBottom: "14px" }}>
            <label>{t.installerModal.installDirLabel}</label>
            <div className="inline-group">
              <input
                type="text"
                value={installDir}
                onChange={(e) => setInstallDir(e.target.value)}
                placeholder="C:\Users\...\AppData\Local\Programs\OpenIDM"
              />
              <button className="btn" type="button" onClick={handleBrowseInstallDir}>
                {t.installerModal.browseBtn}
              </button>
            </div>
          </div>

          <div
            style={{
              background: "#181816",
              border: "1px solid #2e2d29",
              borderRadius: "12px",
              padding: "14px 16px",
              marginBottom: "14px"
            }}
          >
            <h4
              style={{ marginTop: 0, marginBottom: "12px", color: "#faf9f5", fontSize: "13.5px" }}
            >
              {t.installerModal.optionsHeading}
            </h4>

            <div style={{ display: "flex", flexDirection: "column", gap: "10px" }}>
              <label
                style={{
                  display: "flex",
                  alignItems: "flex-start",
                  gap: "12px",
                  cursor: "pointer",
                  padding: "4px 0",
                  lineHeight: "1.4"
                }}
              >
                <input
                  type="checkbox"
                  style={{
                    width: "17px",
                    height: "17px",
                    marginTop: "2px",
                    accentColor: "var(--accent)",
                    flexShrink: 0,
                    cursor: "pointer"
                  }}
                  checked={startWithWindows}
                  onChange={(e) => setStartWithWindows(e.target.checked)}
                />
                <span style={{ display: "flex", flexDirection: "column", gap: "2px" }}>
                  <b style={{ fontSize: "13px", color: "#faf9f5" }}>
                    {t.installerModal.optStartWithWinTitle}
                  </b>
                  <span style={{ fontSize: "12px", color: "#a39f94", lineHeight: "1.35" }}>
                    {t.installerModal.optStartWithWinSub}
                  </span>
                </span>
              </label>

              <label
                style={{
                  display: "flex",
                  alignItems: "flex-start",
                  gap: "12px",
                  cursor: "pointer",
                  padding: "4px 0",
                  lineHeight: "1.4"
                }}
              >
                <input
                  type="checkbox"
                  style={{
                    width: "17px",
                    height: "17px",
                    marginTop: "2px",
                    accentColor: "var(--accent)",
                    flexShrink: 0,
                    cursor: "pointer"
                  }}
                  checked={browserIntegration}
                  onChange={(e) => setBrowserIntegration(e.target.checked)}
                />
                <span style={{ display: "flex", flexDirection: "column", gap: "2px" }}>
                  <b style={{ fontSize: "13px", color: "#faf9f5" }}>
                    {t.installerModal.optBrowserTitle}
                  </b>
                  <span style={{ fontSize: "12px", color: "#a39f94", lineHeight: "1.35" }}>
                    {t.installerModal.optBrowserSub}
                  </span>
                </span>
              </label>

              <label
                style={{
                  display: "flex",
                  alignItems: "flex-start",
                  gap: "12px",
                  cursor: "pointer",
                  padding: "4px 0",
                  lineHeight: "1.4"
                }}
              >
                <input
                  type="checkbox"
                  style={{
                    width: "17px",
                    height: "17px",
                    marginTop: "2px",
                    accentColor: "var(--accent)",
                    flexShrink: 0,
                    cursor: "pointer"
                  }}
                  checked={createShortcuts}
                  onChange={(e) => setCreateShortcuts(e.target.checked)}
                />
                <span style={{ display: "flex", flexDirection: "column", gap: "2px" }}>
                  <b style={{ fontSize: "13px", color: "#faf9f5" }}>
                    {t.installerModal.optShortcutsTitle}
                  </b>
                  <span style={{ fontSize: "12px", color: "#a39f94", lineHeight: "1.35" }}>
                    {t.installerModal.optShortcutsSub}
                  </span>
                </span>
              </label>

              <label
                style={{
                  display: "flex",
                  alignItems: "flex-start",
                  gap: "12px",
                  cursor: "pointer",
                  padding: "4px 0",
                  lineHeight: "1.4"
                }}
              >
                <input
                  type="checkbox"
                  style={{
                    width: "17px",
                    height: "17px",
                    marginTop: "2px",
                    accentColor: "var(--accent)",
                    flexShrink: 0,
                    cursor: "pointer"
                  }}
                  checked={launchAppAfter}
                  onChange={(e) => setLaunchAppAfter(e.target.checked)}
                />
                <span style={{ display: "flex", flexDirection: "column", gap: "2px" }}>
                  <b style={{ fontSize: "13px", color: "#faf9f5" }}>
                    {t.installerModal.optLaunchAfterTitle}
                  </b>
                </span>
              </label>
            </div>
          </div>

          <div
            style={{
              background: "#181816",
              border: "1px solid #2e2d29",
              borderRadius: "12px",
              padding: "14px 16px",
              marginBottom: "14px"
            }}
          >
            <div
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                gap: "12px",
                flexWrap: "wrap"
              }}
            >
              <div>
                <h4 style={{ margin: "0 0 4px 0" }}>{t.installerModal.extActivationHeading}</h4>
                <div className="muted-text" style={{ fontSize: "12px" }}>
                  {t.installerModal.extFolderLabel}{" "}
                  <code>{result?.extensionDir || status?.extensionDir || "browser-extension/"}</code>
                </div>
              </div>
              <div style={{ display: "flex", gap: "8px" }}>
                <button className="btn" type="button" onClick={handleOpenExtFolder}>
                  {t.installerModal.openExtFolderBtn}
                </button>
                <button className="btn primary" type="button" onClick={handleLaunchExtension}>
                  {t.installerModal.installExtBtn}
                </button>
              </div>
            </div>
          </div>

          {(installing || result) && (
            <div
              style={{
                background: "#141413",
                border: "1px solid rgba(217, 119, 87, 0.45)",
                borderRadius: "12px",
                padding: "14px 16px"
              }}
            >
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  fontSize: "12.5px",
                  fontWeight: 600,
                  marginBottom: "8px"
                }}
              >
                <span>{progressMsg}</span>
                <span style={{ color: "#d97757" }}>{pct}%</span>
              </div>
              <div
                style={{
                  width: "100%",
                  height: "8px",
                  background: "#262522",
                  borderRadius: "999px",
                  overflow: "hidden",
                  marginBottom: result ? "12px" : "0"
                }}
              >
                <div
                  style={{
                    width: `${pct}%`,
                    height: "100%",
                    background: "linear-gradient(90deg, #d97757, #4ade80)",
                    transition: "width 0.25s ease"
                  }}
                />
              </div>

              {result && (
                <div style={{ fontSize: "12px", color: "#d5d2c8", lineHeight: 1.6 }}>
                  <div style={{ color: "#4ade80", fontWeight: 700, marginBottom: "6px" }}>
                    {t.installerModal.installSuccessHeading}
                  </div>
                  {result.stepsCompleted?.map((step, i) => (
                    <div key={i}>• {step}</div>
                  ))}
                  <div style={{ marginTop: "6px", color: "#9c988d" }}>
                    <b>{t.installerModal.executableLabel}</b> <code>{result.appExePath}</code> •{" "}
                    <b>{t.installerModal.iconLabel}</b> <code>{result.iconPath}</code>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        <div className="modal-footer">
          <button className="btn" onClick={onClose}>
            {t.installerModal.closeBtn}
          </button>
          <button className="btn primary" disabled={installing} onClick={handleRunInstaller}>
            {installing ? t.installerModal.installingBtn : t.installerModal.installBtn}
          </button>
        </div>
      </div>
    </div>
  );
};
