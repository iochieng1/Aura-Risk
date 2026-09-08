import type { RiskLevel } from "../types";
import { RISK_CONFIG } from "../utils/risk";

export default function StatusBadge({ level }: { level: RiskLevel }) {
  const c = RISK_CONFIG[level];
  return (
    <span
      className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold ${c.bg} ${c.text} border ${c.border}`}
    >
      {level === "Emergency" && "🔴 "}
      {level === "Alert" && "🟠 "}
      {level === "Advisory" && "🟡 "}
      {level === "Normal" && "🟢 "}
      {c.label}
    </span>
  );
}
