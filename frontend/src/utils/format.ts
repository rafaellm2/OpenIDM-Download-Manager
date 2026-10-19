import { getActiveLanguage } from "../i18n";
import { AppLanguage, DownloadStatus } from "../types";

// Formata percentual respeitando o separador decimal do idioma ativo (vírgula em PT-BR, ponto em EN-US).
export function formatPercent(pct: number, decimals = 1, lang: AppLanguage = getActiveLanguage()): string {
  const safe = isFinite(pct) ? pct : 0;
  const formatted = safe.toFixed(decimals);
  return lang === "en-US" ? `${formatted}%` : `${formatted.replace(".", ",")}%`;
}

// Formata quantidade de bytes em B, KB, MB, GB ou TB conforme o idioma ativo (PT-BR ou EN-US).
export function formatBytes(bytes: number, lang: AppLanguage = getActiveLanguage()): string {
  if (!bytes || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const k = 1024;
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), units.length - 1);
  const val = bytes / Math.pow(k, i);
  const numStr = val.toFixed(i === 0 ? 0 : 2);
  const localizedNum = lang === "en-US" ? numStr : numStr.replace(".", ",");
  return `${localizedNum} ${units[i]}`;
}

// Formata velocidade em B/s, KB/s, MB/s.
export function formatSpeed(bps: number, lang: AppLanguage = getActiveLanguage()): string {
  if (!bps || bps <= 0 || !isFinite(bps)) {
    return lang === "en-US" ? "0.00 KB/s" : "0,00 KB/s";
  }
  return `${formatBytes(bps, lang)}/s`;
}

// Formata velocidade com largura estável para downloads ativos sem piscar traço ("—").
export function formatSpeedStable(
  bps: number,
  isActive = true,
  lang: AppLanguage = getActiveLanguage()
): string {
  if (!bps || bps <= 0 || !isFinite(bps)) {
    if (!isActive) return "0 B/s";
    return lang === "en-US" ? "0.00 KB/s" : "0,00 KB/s";
  }
  return `${formatBytes(bps, lang)}/s`;
}

// Formata tempo estimado restante (ETA) em horas, minutos e segundos.
export function formatETA(seconds: number): string {
  if (!seconds || seconds <= 0 || !isFinite(seconds)) return "—";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

// Traduz o status técnico do download para exibição amigável conforme o idioma ativo (PT-BR ou EN-US).
export function statusLabel(
  status: DownloadStatus,
  lang: AppLanguage = getActiveLanguage()
): string {
  if (lang === "en-US") {
    switch (status) {
      case "queued":
        return "Queued";
      case "probing":
        return "Connecting...";
      case "downloading":
        return "Downloading";
      case "paused":
        return "Paused";
      case "assembling":
        return "Assembling...";
      case "completed":
        return "Completed";
      case "error":
        return "Error";
      case "cancelled":
        return "Stopped";
      default:
        return status;
    }
  }
  switch (status) {
    case "queued":
      return "Na Fila";
    case "probing":
      return "Conectando...";
    case "downloading":
      return "Baixando";
    case "paused":
      return "Pausado";
    case "assembling":
      return "Montando...";
    case "completed":
      return "Concluído";
    case "error":
      return "Erro";
    case "cancelled":
      return "Parado";
    default:
      return status;
  }
}

// Mantém alias statusLabelPT para compatibilidade, delegando para statusLabel no idioma ativo.
export function statusLabelPT(status: DownloadStatus): string {
  return statusLabel(status, getActiveLanguage());
}

// Formata data ISO para exibição curta conforme o idioma ativo (PT-BR ou EN-US).
export function formatDate(iso: string, lang: AppLanguage = getActiveLanguage()): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return d.toLocaleString(lang === "en-US" ? "en-US" : "pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

export function formatDatePT(iso: string): string {
  return formatDate(iso, getActiveLanguage());
}
