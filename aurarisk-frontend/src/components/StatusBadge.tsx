import type { RiskLevel } from "../types";
import { riskLevelLabel } from "@aurarisk/shared";
import { RISK_CONFIG } from "../utils/risk";
import { useT } from "../i18n";

export default function StatusBadge({ level }: { level: RiskLevel }) {
  const c = RISK_CONFIG[level];
  const { lang } = useT();
  return (
    <span
      className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold ${c.bg} ${c.text} border ${c.border}`}
    >
      {level === "Emergency" && "🔴 "}
      {level === "Alert" && "🟠 "}
      {level === "Advisory" && "🟡 "}
      {level === "Normal" && "🟢 "}
      {riskLevelLabel(lang, level)}
    </span>
  );
}
