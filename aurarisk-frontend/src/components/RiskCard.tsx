import type { RiskAssessment } from "../types";
import StatusBadge from "./StatusBadge";
import { RISK_CONFIG } from "../utils/risk";

export default function RiskCard({ risk }: { risk: RiskAssessment }) {
  const c = RISK_CONFIG[risk.level];

  return (
    <div className={`rounded-xl border p-4 ${c.bg} ${c.border}`}>
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-sm font-semibold text-gray-700">Flood Risk</h2>
        <StatusBadge level={risk.level} />
      </div>

      <div className="flex items-end gap-2 mb-2">
        <span className={`text-3xl font-bold ${c.text}`}>{risk.score}</span>
        <span className="text-sm text-gray-500 mb-1">/ 100</span>
      </div>

      <p className="text-xs text-gray-600 leading-relaxed">{risk.summary}</p>
    </div>
  );
}
