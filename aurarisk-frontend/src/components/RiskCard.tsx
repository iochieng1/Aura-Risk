import { describeFreshness, WEATHER_ATTRIBUTION } from "@aurarisk/shared";
import type { RiskAssessment } from "../types";
import StatusBadge from "./StatusBadge";
import { RISK_CONFIG } from "../utils/risk";
import { useNow } from "../hooks/useNow";
import { useT } from "../i18n";

export default function RiskCard({ risk }: { risk: RiskAssessment }) {
  const c = RISK_CONFIG[risk.level];
  const now = useNow();
  const { lang, t } = useT();
  const freshness = describeFreshness(risk, now, lang);
  const fetchedAt = risk.source_timestamps?.weather_fetched_at;

  return (
    <section className={`rounded-xl border p-4 ${c.bg} ${c.border}`}>
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-sm font-semibold text-gray-700">{t("web.floodRisk")}</h2>
        <StatusBadge level={risk.level} />
      </div>

      <p className="flex items-end gap-2 mb-2">
        <span className={`text-3xl font-bold ${c.text}`}>{risk.score}</span>
        <span className="text-sm text-gray-600 mb-1">/ 100</span>
      </p>

      <p className="text-xs text-gray-700 leading-relaxed">{risk.summary}</p>

      {freshness && (
        <p
          className={`mt-3 text-xs ${
            freshness.outdated ? "text-amber-900 bg-amber-50 border border-amber-200 rounded-lg p-2" : "text-gray-600"
          }`}
        >
          {freshness.outdated && <span aria-hidden="true">⚠️ </span>}
          {fetchedAt ? (
            <time dateTime={risk.source_timestamps?.weather_observed_at || fetchedAt} title={new Date(fetchedAt).toLocaleString()}>
              {freshness.label}
            </time>
          ) : (
            freshness.label
          )}
          {freshness.outdated && `. ${t("freshness.mayHaveChanged")}`}
        </p>
      )}

      {risk.confidence && (
        <details className="mt-2 text-xs text-gray-600">
          <summary className="cursor-pointer">
            {t("web.confidence", { level: t(`confidence.${risk.confidence.level}`) })}
          </summary>
          {risk.confidence.reasons.length > 0 && (
            <ul className="mt-1 ml-4 list-disc space-y-0.5">
              {risk.confidence.reasons.map((reason) => (
                <li key={reason}>{reason}</li>
              ))}
            </ul>
          )}
        </details>
      )}

      <p className="mt-3 text-[11px] text-gray-600">
        <a href={WEATHER_ATTRIBUTION.url} target="_blank" rel="noreferrer" className="underline">
          {t("weather.attribution")}
        </a>{" "}
        (
        <a href={WEATHER_ATTRIBUTION.licenceUrl} target="_blank" rel="noreferrer" className="underline">
          {WEATHER_ATTRIBUTION.licence}
        </a>
        )
      </p>
    </section>
  );
}
