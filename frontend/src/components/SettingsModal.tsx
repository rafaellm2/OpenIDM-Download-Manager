import React, { useState } from "react";
import { useI18n } from "../i18n";
import { AppLanguage, AppSettings, SiteLogin, SiteRule } from "../types";

interface SettingsModalProps {
  settings: AppSettings;
  siteRules: SiteRule[];
  siteLogins: SiteLogin[];
  onClose: () => void;
  onSaveSettings: (cfg: AppSettings) => Promise<void>;
  onSaveSiteRule: (r: SiteRule) => Promise<void>;
  onDeleteSiteRule: (id: number) => Promise<void>;
  onSaveSiteLogin: (l: SiteLogin) => Promise<void>;
  onDeleteSiteLogin: (id: number) => Promise<void>;
  onInstallBrowserIntegration: (extId: string) => Promise<string>;
  onOpenInstaller?: () => void;
}

// Modal de Opções Globais do OpenIDM com suporte bilíngue (PT-BR / EN-US).
export const SettingsModal: React.FC<SettingsModalProps> = ({
  settings,
  siteRules,
  siteLogins,
  onClose,
  onSaveSettings,
  onSaveSiteRule,
  onDeleteSiteRule,
  onSaveSiteLogin,
  onDeleteSiteLogin,
  onInstallBrowserIntegration,
  onOpenInstaller
}) => {
  const { t, language, setLanguage } = useI18n();
  const [tab, setTab] = useState<"conn" | "capture" | "logins" | "proxy" | "av">("conn");
  const [cfg, setCfg] = useState<AppSettings>({
    ...settings,
    language: settings.language || language
  });
  const [extensionsStr, setExtensionsStr] = useState(
    (settings.captureExtensions || []).join(", ")
  );
  const [extIdInput, setExtIdInput] = useState("");
  const [statusMsg, setStatusMsg] = useState("");

  // Estado para nova Exceção por Site
  const [ruleHost, setRuleHost] = useState("");
  const [ruleMaxConn, setRuleMaxConn] = useState(4);
  const [ruleIgnore, setRuleIgnore] = useState(false);

  // Estado para novo Login de Site
  const [loginHost, setLoginHost] = useState("");
  const [loginUser, setLoginUser] = useState("");
  const [loginPass, setLoginPass] = useState("");
  const [loginType, setLoginType] = useState<"basic" | "bearer" | "ftp">("basic");

  const handleLanguageSelect = (nextLang: AppLanguage) => {
    setCfg((prev) => ({ ...prev, language: nextLang }));
    setLanguage(nextLang);
  };

  const handleSaveAll = async () => {
    const extList = extensionsStr
      .split(/[,;\s]+/)
      .map((s) => s.trim().toLowerCase().replace(/^\./, ""))
      .filter(Boolean);
    const updated: AppSettings = {
      ...cfg,
      language: cfg.language || language,
      captureExtensions: extList
    };
    await onSaveSettings(updated);
    setStatusMsg(t.settingsModal.savedSuccessMsg);
  };

  const handleAddRule = async () => {
    if (!ruleHost.trim()) return;
    await onSaveSiteRule({
      id: 0,
      hostPattern: ruleHost.trim(),
      maxConnections: ruleMaxConn,
      ignoreCapture: ruleIgnore,
      retryLimit: 10,
      retryIntervalSec: 30
    });
    setRuleHost("");
  };

  const handleAddLogin = async () => {
    if (!loginHost.trim() || !loginUser.trim()) return;
    await onSaveSiteLogin({
      id: 0,
      hostPattern: loginHost.trim(),
      username: loginUser.trim(),
      password: loginPass,
      authType: loginType
    });
    setLoginHost("");
    setLoginUser("");
    setLoginPass("");
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-window large" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <div>
            <h3>{t.settingsModal.title}</h3>
            <span className="modal-subtitle">{t.settingsModal.subtitle}</span>
          </div>
          <button className="close-btn" onClick={onClose}>
            ✕
          </button>
        </div>

        <div className="modal-tabs">
          <button
            className={`tab-btn ${tab === "conn" ? "active" : ""}`}
            onClick={() => setTab("conn")}
          >
            {t.settingsModal.tabConn}
          </button>
          <button
            className={`tab-btn ${tab === "capture" ? "active" : ""}`}
            onClick={() => setTab("capture")}
          >
            {t.settingsModal.tabCapture}
          </button>
          <button
            className={`tab-btn ${tab === "logins" ? "active" : ""}`}
            onClick={() => setTab("logins")}
          >
            {t.settingsModal.tabLogins(siteLogins.length)}
          </button>
          <button
            className={`tab-btn ${tab === "proxy" ? "active" : ""}`}
            onClick={() => setTab("proxy")}
          >
            {t.settingsModal.tabProxy}
          </button>
          <button
            className={`tab-btn ${tab === "av" ? "active" : ""}`}
            onClick={() => setTab("av")}
          >
            {t.settingsModal.tabAv}
          </button>
        </div>

        <div className="modal-body">
          {statusMsg && <div className="alert-banner success">{statusMsg}</div>}

          {tab === "conn" && (
            <>
              <div className="form-group" style={{ marginBottom: "14px" }}>
                <label>{t.settingsModal.languageLabel}</label>
                <select
                  value={cfg.language || language}
                  onChange={(e) => handleLanguageSelect(e.target.value as AppLanguage)}
                >
                  <option value="pt-BR">{t.settingsModal.languagePt}</option>
                  <option value="en-US">{t.settingsModal.languageEn}</option>
                </select>
              </div>

              <div className="alert-banner warn">{t.settingsModal.connWarnBanner}</div>

              <div className="form-grid-3">
                <div className="form-group">
                  <label>{t.settingsModal.maxConnLabel}</label>
                  <select
                    value={cfg.maxConnectionsPerDownload}
                    onChange={(e) =>
                      setCfg({ ...cfg, maxConnectionsPerDownload: Number(e.target.value) })
                    }
                  >
                    {[1, 2, 4, 8, 16, 24, 32].map((n) => (
                      <option key={n} value={n}>
                        {t.settingsModal.connectionsUnit(n)}
                      </option>
                    ))}
                  </select>
                </div>

                <div className="form-group">
                  <label>{t.settingsModal.minSplitLabel}</label>
                  <input
                    type="number"
                    min={64}
                    value={Math.round(cfg.minSplitSizeBytes / 1024)}
                    onChange={(e) =>
                      setCfg({ ...cfg, minSplitSizeBytes: Number(e.target.value) * 1024 })
                    }
                  />
                </div>

                <div className="form-group">
                  <label>{t.settingsModal.globalSpeedLabel}</label>
                  <input
                    type="number"
                    min={0}
                    value={Math.round(cfg.globalSpeedLimitBps / 1024)}
                    onChange={(e) =>
                      setCfg({ ...cfg, globalSpeedLimitBps: Number(e.target.value) * 1024 })
                    }
                  />
                </div>
              </div>

              <hr className="divider" />
              <h4>{t.settingsModal.siteExceptionsTitle}</h4>
              <div className="inline-group">
                <input
                  type="text"
                  placeholder={t.settingsModal.siteHostPlaceholder}
                  value={ruleHost}
                  onChange={(e) => setRuleHost(e.target.value)}
                />
                <input
                  type="number"
                  min={1}
                  max={32}
                  style={{ width: "110px" }}
                  value={ruleMaxConn}
                  onChange={(e) => setRuleMaxConn(Number(e.target.value))}
                  title={t.settingsModal.siteMaxConnTitle}
                />
                <label className="small-check">
                  <input
                    type="checkbox"
                    checked={ruleIgnore}
                    onChange={(e) => setRuleIgnore(e.target.checked)}
                  />
                  {t.settingsModal.siteIgnoreCheck}
                </label>
                <button className="btn primary" onClick={handleAddRule}>
                  {t.settingsModal.addRuleBtn}
                </button>
              </div>

              <table className="mini-table">
                <thead>
                  <tr>
                    <th>{t.settingsModal.colHostPattern}</th>
                    <th>{t.settingsModal.colMaxConn}</th>
                    <th>{t.settingsModal.colCapture}</th>
                    <th>{t.settingsModal.colAction}</th>
                  </tr>
                </thead>
                <tbody>
                  {siteRules.map((r) => (
                    <tr key={r.id || r.hostPattern}>
                      <td>{r.hostPattern}</td>
                      <td>{r.maxConnections}</td>
                      <td>
                        {r.ignoreCapture
                          ? t.settingsModal.ignoreBrowserDownloads
                          : t.settingsModal.captureActive}
                      </td>
                      <td>
                        <button className="mini-btn" onClick={() => onDeleteSiteRule(r.id)}>
                          🗑
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}

          {tab === "capture" && (
            <>
              <div className="checkbox-row">
                <label>
                  <input
                    type="checkbox"
                    checked={cfg.startWithWindows !== false}
                    onChange={(e) => setCfg({ ...cfg, startWithWindows: e.target.checked })}
                  />
                  <b>{t.settingsModal.startWithWindowsCheck}</b>
                </label>
              </div>

              <div className="checkbox-row">
                <label>
                  <input
                    type="checkbox"
                    checked={cfg.monitorClipboard}
                    onChange={(e) => setCfg({ ...cfg, monitorClipboard: e.target.checked })}
                  />
                  <b>{t.settingsModal.monitorClipboardCheck}</b>
                </label>
              </div>

              <div className="form-group">
                <label>{t.settingsModal.ignoreModifierLabel}</label>
                <select
                  value={cfg.ignoreModifierKey}
                  onChange={(e) => setCfg({ ...cfg, ignoreModifierKey: e.target.value })}
                >
                  <option value="ALT">{t.settingsModal.altDefaultOption}</option>
                  <option value="CTRL">CTRL</option>
                  <option value="SHIFT">SHIFT</option>
                </select>
              </div>

              <div className="form-group">
                <label>{t.settingsModal.captureExtensionsLabel}</label>
                <textarea
                  rows={3}
                  value={extensionsStr}
                  onChange={(e) => setExtensionsStr(e.target.value)}
                />
              </div>

              <hr className="divider" />
              <h4>{t.settingsModal.nativeMessagingTitle}</h4>
              <p className="muted-text">
                {t.settingsModal.nativeMessagingDescPrefix}
                <code>browser-extension/</code>
                {t.settingsModal.nativeMessagingDescSuffix}
              </p>
              <div className="inline-group">
                <input
                  type="text"
                  placeholder={t.settingsModal.extIdPlaceholder}
                  value={extIdInput}
                  onChange={(e) => setExtIdInput(e.target.value)}
                />
                <button
                  className="btn primary"
                  onClick={async () => {
                    const path = await onInstallBrowserIntegration(extIdInput);
                    setStatusMsg(t.settingsModal.nativeHostRegisteredMsg(path));
                  }}
                >
                  {t.settingsModal.registerNativeHostBtn}
                </button>
                {onOpenInstaller && (
                  <button
                    className="btn"
                    type="button"
                    onClick={() => {
                      onClose();
                      onOpenInstaller();
                    }}
                  >
                    {t.settingsModal.openInstallerBtn}
                  </button>
                )}
              </div>
            </>
          )}

          {tab === "logins" && (
            <>
              <h4>{t.settingsModal.loginsTitle}</h4>
              <div className="inline-group">
                <input
                  type="text"
                  placeholder={t.settingsModal.loginHostPlaceholder}
                  value={loginHost}
                  onChange={(e) => setLoginHost(e.target.value)}
                />
                <select
                  value={loginType}
                  onChange={(e) => setLoginType(e.target.value as any)}
                  style={{ width: "120px" }}
                >
                  <option value="basic">HTTP Basic</option>
                  <option value="bearer">Bearer Token</option>
                  <option value="ftp">FTP</option>
                </select>
                <input
                  type="text"
                  placeholder={t.settingsModal.loginUserPlaceholder}
                  value={loginUser}
                  onChange={(e) => setLoginUser(e.target.value)}
                />
                <input
                  type="password"
                  placeholder={t.settingsModal.loginPassPlaceholder}
                  value={loginPass}
                  onChange={(e) => setLoginPass(e.target.value)}
                />
                <button className="btn primary" onClick={handleAddLogin}>
                  {t.settingsModal.saveLoginBtn}
                </button>
              </div>

              <table className="mini-table">
                <thead>
                  <tr>
                    <th>{t.settingsModal.colServer}</th>
                    <th>{t.settingsModal.colType}</th>
                    <th>{t.settingsModal.colUser}</th>
                    <th>{t.settingsModal.colAction}</th>
                  </tr>
                </thead>
                <tbody>
                  {siteLogins.map((l) => (
                    <tr key={l.id || l.hostPattern}>
                      <td>{l.hostPattern}</td>
                      <td>{l.authType.toUpperCase()}</td>
                      <td>{l.username}</td>
                      <td>
                        <button className="mini-btn" onClick={() => onDeleteSiteLogin(l.id)}>
                          🗑
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}

          {tab === "proxy" && (
            <>
              <div className="form-group">
                <label>{t.settingsModal.proxyModeLabel}</label>
                <select
                  value={cfg.proxyType}
                  onChange={(e) => setCfg({ ...cfg, proxyType: e.target.value as any })}
                >
                  <option value="none">{t.settingsModal.proxyNone}</option>
                  <option value="system">{t.settingsModal.proxySystem}</option>
                  <option value="http">{t.settingsModal.proxyHttp}</option>
                  <option value="socks5">{t.settingsModal.proxySocks5}</option>
                  <option value="pac">{t.settingsModal.proxyPac}</option>
                </select>
              </div>

              {cfg.proxyType === "pac" && (
                <div className="form-group">
                  <label>{t.settingsModal.proxyPacLabel}</label>
                  <input
                    type="text"
                    placeholder={t.settingsModal.proxyPacPlaceholder}
                    value={cfg.proxyPacUrl || ""}
                    onChange={(e) => setCfg({ ...cfg, proxyPacUrl: e.target.value })}
                  />
                </div>
              )}

              {(cfg.proxyType === "http" || cfg.proxyType === "socks5") && (
                <>
                  <div className="form-grid-2">
                    <div className="form-group">
                      <label>{t.settingsModal.proxyHostLabel}</label>
                      <input
                        type="text"
                        placeholder="127.0.0.1"
                        value={cfg.proxyHost}
                        onChange={(e) => setCfg({ ...cfg, proxyHost: e.target.value })}
                      />
                    </div>
                    <div className="form-group">
                      <label>{t.settingsModal.proxyPortLabel}</label>
                      <input
                        type="number"
                        value={cfg.proxyPort}
                        onChange={(e) => setCfg({ ...cfg, proxyPort: Number(e.target.value) })}
                      />
                    </div>
                  </div>
                </>
              )}

              {(cfg.proxyType === "http" || cfg.proxyType === "socks5" || cfg.proxyType === "pac") && (
                <div className="form-grid-2">
                  <div className="form-group">
                    <label>{t.settingsModal.proxyUserLabel}</label>
                    <input
                      type="text"
                      value={cfg.proxyUser}
                      onChange={(e) => setCfg({ ...cfg, proxyUser: e.target.value })}
                    />
                  </div>
                  <div className="form-group">
                    <label>{t.settingsModal.proxyPassLabel}</label>
                    <input
                      type="password"
                      value={cfg.proxyPass}
                      onChange={(e) => setCfg({ ...cfg, proxyPass: e.target.value })}
                    />
                  </div>
                </div>
              )}
            </>
          )}

          {tab === "av" && (
            <>
              <div className="form-group">
                <label>{t.settingsModal.defaultSaveDirLabel}</label>
                <input
                  type="text"
                  value={cfg.defaultSaveDir}
                  onChange={(e) => setCfg({ ...cfg, defaultSaveDir: e.target.value })}
                />
              </div>

              <div className="form-group">
                <label>{t.settingsModal.checkpointIntervalLabel}</label>
                <input
                  type="number"
                  min={1}
                  max={30}
                  value={cfg.checkpointIntervalSec}
                  onChange={(e) =>
                    setCfg({ ...cfg, checkpointIntervalSec: Number(e.target.value) })
                  }
                />
              </div>

              <hr className="divider" />
              <div className="checkbox-row">
                <label>
                  <input
                    type="checkbox"
                    checked={cfg.antivirusEnabled}
                    onChange={(e) => setCfg({ ...cfg, antivirusEnabled: e.target.checked })}
                  />
                  <b>{t.settingsModal.antivirusEnabledCheck}</b>
                </label>
              </div>

              <div className="form-group">
                <label>{t.settingsModal.antivirusPathLabel}</label>
                <input
                  type="text"
                  placeholder="C:\Program Files\Windows Defender\MpCmdRun.exe"
                  value={cfg.antivirusPath}
                  onChange={(e) => setCfg({ ...cfg, antivirusPath: e.target.value })}
                />
              </div>

              <div className="form-group">
                <label>{t.settingsModal.antivirusArgsLabel}</label>
                <input
                  type="text"
                  placeholder='-Scan -ScanType 3 -File "[file]"'
                  value={cfg.antivirusArgs}
                  onChange={(e) => setCfg({ ...cfg, antivirusArgs: e.target.value })}
                />
              </div>
            </>
          )}
        </div>

        <div className="modal-footer">
          <button className="btn" onClick={onClose}>
            {t.settingsModal.closeBtn}
          </button>
          <button className="btn primary" onClick={handleSaveAll}>
            {t.settingsModal.saveSettingsBtn}
          </button>
        </div>
      </div>
    </div>
  );
};
