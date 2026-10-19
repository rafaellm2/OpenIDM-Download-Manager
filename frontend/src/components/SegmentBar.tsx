import React from "react";
import { useI18n } from "../i18n";
import { Segment } from "../types";
import { formatBytes } from "../utils/format";

interface SegmentBarProps {
  segments: Segment[];
  totalSize: number;
  downloadedSize: number;
  height?: number;
  showLegend?: boolean;
}

// Renderiza o mapa visual de Segmentação Dinâmica do OpenIDM:
// Marcador rosa = início de segmento | Azul = bytes baixados | Escuro = bytes pendentes.
export const SegmentBar: React.FC<SegmentBarProps> = ({
  segments,
  totalSize,
  downloadedSize,
  height = 18,
  showLegend = false
}) => {
  const { language, t } = useI18n();

  if (!segments || segments.length === 0 || totalSize <= 0) {
    const pct = totalSize > 0 ? Math.min(100, (downloadedSize * 100) / totalSize) : 0;
    return (
      <div className="segbar-wrapper">
        <div className="segbar-track" style={{ height }}>
          <div className="segbar-fill-simple" style={{ width: `${pct}%` }} />
        </div>
      </div>
    );
  }

  return (
    <div className="segbar-wrapper">
      <div className="segbar-track" style={{ height }}>
        {segments.map((seg, idx) => {
          const segLen = Math.max(0, seg.end - seg.start);
          const widthPct = (segLen * 100) / totalSize;
          const downloadedLen = Math.min(segLen, Math.max(0, seg.current - seg.start));
          const fillPct = segLen > 0 ? (downloadedLen * 100) / segLen : 0;

          return (
            <div
              key={`${seg.start}-${idx}`}
              className="segbar-slice"
              style={{ width: `${widthPct}%` }}
              title={t.segmentBar.sliceTitle(
                idx + 1,
                seg.workerId || "-",
                formatBytes(seg.start, language),
                formatBytes(seg.end, language),
                language === "pt-BR"
                  ? fillPct.toFixed(1).replace(".", ",")
                  : fillPct.toFixed(1),
                seg.reusedConn
              )}
            >
              {/* Marcador rosa de início de segmento do OpenIDM */}
              {seg.status === "downloading" && <div className="segbar-marker-pink" />}
              <div
                className={`segbar-slice-fill ${
                  seg.reusedConn ? "segbar-fill-reused" : "segbar-fill-downloaded"
                }`}
                style={{ width: `${fillPct}%` }}
              />
            </div>
          );
        })}
      </div>

      {showLegend && (
        <div className="segbar-legend">
          <span>
            <i className="legend-dot pink" /> {t.segmentBar.legendActiveStart}
          </span>
          <span>
            <i className="legend-dot blue" /> {t.segmentBar.legendDownloaded}
          </span>
          <span>
            <i className="legend-dot cyan" /> {t.segmentBar.legendReused}
          </span>
          <span>
            <i className="legend-dot dark" /> {t.segmentBar.legendPending}
          </span>
        </div>
      )}
    </div>
  );
};
