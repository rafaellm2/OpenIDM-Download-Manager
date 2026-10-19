import React, { useState } from "react";
import { formatQueueName, useI18n } from "../i18n";
import { Queue } from "../types";
import { formatBytes } from "../utils/format";

interface SchedulerModalProps {
  queues: Queue[];
  onClose: () => void;
  onSaveQueue: (q: Queue) => Promise<void>;
  onDeleteQueue: (id: string) => Promise<void>;
  onStartQueue: (id: string) => Promise<void>;
  onStopQueue: (id: string) => Promise<void>;
  onCancelShutdown: () => Promise<void>;
}

// Modal do Agendador e Gerenciador de Filas do OpenIDM com suporte bilíngue.
export const SchedulerModal: React.FC<SchedulerModalProps> = ({
  queues,
  onClose,
  onSaveQueue,
  onDeleteQueue,
  onStartQueue,
  onStopQueue,
  onCancelShutdown
}) => {
  const { t, language } = useI18n();
  const [selectedId, setSelectedId] = useState<string>(queues[0]?.id || "main");
  const [newQueueName, setNewQueueName] = useState("");
  const [newQueueSync, setNewQueueSync] = useState(false);

  const current = queues.find((q) => q.id === selectedId) || queues[0];
  const [form, setForm] = useState<Queue>(
    current || {
      id: "main",
      name: "Fila Principal",
      isMain: true,
      isSync: false,
      maxConcurrent: 4,
      startTime: "",
      stopTime: "",
      trafficQuotaMB: 0,
      usedTrafficBytes: 0,
      retryIntervalSec: 30,
      maxRetries: 10,
      shutdownOnFinish: false,
      runCommandOnFinish: "",
      active: false
    }
  );
  const [savedMsg, setSavedMsg] = useState("");

  const selectQueue = (q: Queue) => {
    setSelectedId(q.id);
    setForm({ ...q });
    setSavedMsg("");
  };

  const handleCreateQueue = async () => {
    if (!newQueueName.trim()) return;
    const created: Queue = {
      id: `queue_${Date.now()}`,
      name: newQueueName.trim(),
      isMain: false,
      isSync: newQueueSync,
      maxConcurrent: 3,
      startTime: "",
      stopTime: "",
      trafficQuotaMB: 0,
      usedTrafficBytes: 0,
      retryIntervalSec: 30,
      maxRetries: 10,
      shutdownOnFinish: false,
      runCommandOnFinish: "",
      active: false
    };
    await onSaveQueue(created);
    setNewQueueName("");
    setNewQueueSync(false);
    selectQueue(created);
  };

  const handleSave = async () => {
    await onSaveQueue(form);
    setSavedMsg(t.schedulerModal.savedSuccessMsg);
  };

  const handleResetQuota = async () => {
    const updated = { ...form, usedTrafficBytes: 0 };
    setForm(updated);
    await onSaveQueue(updated);
    setSavedMsg(t.schedulerModal.resetQuotaSuccessMsg);
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-window large" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <div>
            <h3>{t.schedulerModal.title}</h3>
            <span className="modal-subtitle">{t.schedulerModal.subtitle}</span>
          </div>
          <button className="close-btn" onClick={onClose}>
            ✕
          </button>
        </div>

        <div className="modal-body scheduler-layout">
          <div className="scheduler-sidebar">
            <div className="sidebar-heading">{t.schedulerModal.registeredQueues}</div>
            {queues.map((q) => (
              <button
                key={q.id}
                className={`sidebar-item ${q.id === form.id ? "active" : ""}`}
                onClick={() => selectQueue(q)}
              >
                <span>
                  {q.isSync ? "🔄 " : "📋 "}
                  {formatQueueName(q, language)}
                </span>
                <span className={`sidebar-count ${q.active ? "running" : ""}`}>
                  {q.active ? t.schedulerModal.queueActive : t.schedulerModal.queueStopped}
                </span>
              </button>
            ))}

            <div className="new-queue-box">
              <div className="sidebar-heading">{t.schedulerModal.newCustomQueue}</div>
              <input
                type="text"
                placeholder={t.schedulerModal.newQueuePlaceholder}
                value={newQueueName}
                onChange={(e) => setNewQueueName(e.target.value)}
              />
              <label className="small-check">
                <input
                  type="checkbox"
                  checked={newQueueSync}
                  onChange={(e) => setNewQueueSync(e.target.checked)}
                />
                {t.schedulerModal.syncQueueCheck}
              </label>
              <button className="btn primary full-width" onClick={handleCreateQueue}>
                {t.schedulerModal.createQueueBtn}
              </button>
            </div>
          </div>

          <div className="scheduler-content">
            {savedMsg && <div className="alert-banner success">{savedMsg}</div>}

            {form.isSync && (
              <div className="alert-banner info">
                🔄 <b>{t.schedulerModal.syncQueueBannerTitle}</b>{" "}
                {t.schedulerModal.syncQueueBannerText}
              </div>
            )}

            <div className="form-grid-2">
              <div className="form-group">
                <label>{t.schedulerModal.startTimeLabel}</label>
                <input
                  type="time"
                  value={form.startTime}
                  onChange={(e) => setForm({ ...form, startTime: e.target.value })}
                />
                <small className="muted-text">{t.schedulerModal.startTimeHint}</small>
              </div>

              <div className="form-group">
                <label>{t.schedulerModal.stopTimeLabel}</label>
                <input
                  type="time"
                  value={form.stopTime}
                  onChange={(e) => setForm({ ...form, stopTime: e.target.value })}
                />
                <small className="muted-text">{t.schedulerModal.stopTimeHint}</small>
              </div>
            </div>

            <div className="form-grid-2">
              <div className="form-group">
                <label>{t.schedulerModal.maxConcurrentLabel}</label>
                <input
                  type="number"
                  min={1}
                  max={16}
                  value={form.maxConcurrent}
                  onChange={(e) => setForm({ ...form, maxConcurrent: Number(e.target.value) })}
                />
              </div>

              <div className="form-group">
                <label>{t.schedulerModal.trafficQuotaLabel}</label>
                <div className="inline-group">
                  <input
                    type="number"
                    min={0}
                    placeholder={t.schedulerModal.trafficQuotaPlaceholder}
                    value={form.trafficQuotaMB}
                    onChange={(e) => setForm({ ...form, trafficQuotaMB: Number(e.target.value) })}
                  />
                  <button
                    className="btn"
                    onClick={handleResetQuota}
                    title={t.schedulerModal.resetQuotaTitle}
                  >
                    {t.schedulerModal.resetQuotaBtn(formatBytes(form.usedTrafficBytes, language))}
                  </button>
                </div>
              </div>
            </div>

            <div className="form-grid-2">
              <div className="form-group">
                <label>{t.schedulerModal.retryIntervalLabel}</label>
                <input
                  type="number"
                  min={5}
                  value={form.retryIntervalSec}
                  onChange={(e) => setForm({ ...form, retryIntervalSec: Number(e.target.value) })}
                />
                <small className="muted-text">{t.schedulerModal.retryIntervalHint}</small>
              </div>

              <div className="form-group">
                <label>{t.schedulerModal.maxRetriesLabel}</label>
                <input
                  type="number"
                  min={0}
                  value={form.maxRetries}
                  onChange={(e) => setForm({ ...form, maxRetries: Number(e.target.value) })}
                />
              </div>
            </div>

            <div className="form-group">
              <label>{t.schedulerModal.runCommandLabel}</label>
              <input
                type="text"
                placeholder={t.schedulerModal.runCommandPlaceholder}
                value={form.runCommandOnFinish}
                onChange={(e) => setForm({ ...form, runCommandOnFinish: e.target.value })}
              />
            </div>

            <div className="checkbox-row">
              <label>
                <input
                  type="checkbox"
                  checked={form.shutdownOnFinish}
                  onChange={(e) => setForm({ ...form, shutdownOnFinish: e.target.checked })}
                />
                <b>{t.schedulerModal.shutdownOnFinishLabel}</b>
              </label>

              <button className="btn warn" onClick={onCancelShutdown}>
                {t.schedulerModal.abortShutdownBtn}
              </button>
            </div>
          </div>
        </div>

        <div className="modal-footer">
          <div className="footer-left">
            {!form.isMain && !form.isSync && (
              <button
                className="btn danger"
                onClick={async () => {
                  await onDeleteQueue(form.id);
                  if (queues.length > 0) selectQueue(queues[0]);
                }}
              >
                {t.schedulerModal.deleteQueueBtn}
              </button>
            )}
          </div>

          <div className="footer-right">
            {form.active ? (
              <button
                className="btn warn"
                onClick={async () => {
                  await onStopQueue(form.id);
                  setForm({ ...form, active: false });
                }}
              >
                {t.schedulerModal.stopQueueNowBtn}
              </button>
            ) : (
              <button
                className="btn success"
                onClick={async () => {
                  await onSaveQueue(form);
                  await onStartQueue(form.id);
                  setForm({ ...form, active: true });
                }}
              >
                {t.schedulerModal.startQueueNowBtn}
              </button>
            )}
            <button className="btn primary" onClick={handleSave}>
              {t.schedulerModal.saveChangesBtn}
            </button>
            <button className="btn" onClick={onClose}>
              {t.schedulerModal.closeBtn}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
