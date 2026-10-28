import { useCallback, useEffect, useState } from "react";
import type { CountyStatsResponse } from "@aurarisk/shared";
import { fetchCountyStats, openDataUrl } from "../api/risk";
import { describeError } from "../api/errors";
import { ErrorState } from "./StateMessages";
import { useT } from "../i18n";
import type { MessageKey } from "@aurarisk/shared";

const PERIODS: readonly { days: number; label: MessageKey }[] = [
  { days: 30, label: "dash.last30" },
  { days: 90, label: "dash.last90" },
  { days: 365, label: "dash.last365" },
];

/** The `days` days up to and including today, as UTC dates. */
function periodFor(days: number) {
  const to = new Date();
  const from = new Date(to.getTime() - (days - 1) * 24 * 60 * 60 * 1000);
  const day = (d: Date) => d.toISOString().slice(0, 10);
  return { from: day(from), to: day(to) };
}

function formatHours(hours: number | null, t: ReturnType<typeof useT>["t"]): string {
  if (hours === null) return "–";
  if (hours < 48) return t("dash.hours", { n: hours.toFixed(1) });
  return t("dash.days", { n: (hours / 24).toFixed(1) });
}

/** Public county response dashboard and open data downloads (GET /api/stats/counties). */
export default function CountyDashboard() {
  const [days, setDays] = useState<number>(90);
  const [stats, setStats] = useState<CountyStatsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const period = periodFor(days);
  const { lang, t } = useT();

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setStats(await fetchCountyStats(periodFor(days)));
    } catch (err) {
      console.error("Failed to load county stats", err);
      setError(t("dash.loadFailed", { reason: describeError(err, lang) }));
    } finally {
      setLoading(false);
    }
  }, [days, lang, t]);

  useEffect(() => {
    load();
  }, [load]);

  const totals = stats?.counties.reduce(
    (sum, c) => ({
      verified: sum.verified + c.verified,
      acknowledged: sum.acknowledged + c.acknowledged + c.resolved,
      resolved: sum.resolved + c.resolved,
    }),
    { verified: 0, acknowledged: 0, resolved: 0 }
  );

  return (
    <main className="flex-1 overflow-y-auto bg-gray-50">
      <div className="max-w-5xl mx-auto px-4 py-6 space-y-6">
        <section>
          <h2 className="text-xl font-semibold text-gray-900">{t("dash.title")}</h2>
          <p className="mt-1 text-sm text-gray-600">
            {t("dash.intro")}
          </p>
        </section>

        <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("dash.period")}>
          {PERIODS.map((p) => (
            <button
              key={p.days}
              type="button"
              onClick={() => setDays(p.days)}
              aria-pressed={days === p.days}
              className={`px-3 py-1.5 text-sm rounded-lg border transition focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                days === p.days
                  ? "bg-blue-600 text-white border-blue-600"
                  : "bg-white text-gray-700 border-gray-300 hover:bg-gray-100"
              }`}
            >
              {t(p.label)}
            </button>
          ))}
          <span className="text-xs text-gray-500 ml-auto">
            {t("dash.range", period)}
          </span>
        </div>

        {error && <ErrorState message={error} onRetry={load} />}
        {loading && !stats && (
          <p role="status" className="text-sm text-gray-500">
            {t("dash.loading")}
          </p>
        )}

        {stats && totals && (
          <>
            <dl className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              {[
                [t("dash.verified"), totals.verified],
                [t("dash.acknowledgedTotal"), totals.acknowledged],
                [t("dash.resolved"), totals.resolved],
              ].map(([label, value]) => (
                <div key={label} className="bg-white border border-gray-200 rounded-xl p-4">
                  <dt className="text-xs text-gray-500">{label}</dt>
                  <dd className="text-2xl font-semibold text-gray-900 tabular-nums">{value}</dd>
                </div>
              ))}
            </dl>

            {stats.counties.length === 0 ? (
              <p className="text-sm text-gray-500 italic">{t("dash.none")}</p>
            ) : (
              <div className="overflow-x-auto bg-white border border-gray-200 rounded-xl">
                <table className="min-w-full text-sm">
                  <caption className="sr-only">{t("dash.caption", period)}</caption>
                  <thead className="bg-gray-50 text-left text-xs text-gray-600">
                    <tr>
                      <th scope="col" className="px-3 py-2 font-semibold">{t("dash.colCounty")}</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">{t("dash.colVerified")}</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">{t("dash.colAwaiting")}</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">{t("dash.colAcknowledged")}</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">{t("dash.colResolved")}</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">{t("dash.colMedianAck")}</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">{t("dash.colMedianResolve")}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-100 tabular-nums">
                    {stats.counties.map((c) => (
                      <tr key={c.county}>
                        <th scope="row" className="px-3 py-2 text-left font-medium text-gray-900">{c.county}</th>
                        <td className="px-3 py-2 text-right">{c.verified}</td>
                        <td className="px-3 py-2 text-right">{c.open}</td>
                        <td className="px-3 py-2 text-right">{c.acknowledged}</td>
                        <td className="px-3 py-2 text-right">{c.resolved}</td>
                        <td className="px-3 py-2 text-right">{formatHours(c.median_hours_to_acknowledge, t)}</td>
                        <td className="px-3 py-2 text-right">{formatHours(c.median_hours_to_resolve, t)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}

        <section className="bg-white border border-gray-200 rounded-xl p-4 text-sm text-gray-700 space-y-2">
          <h3 className="font-semibold text-gray-900">{t("dash.openData")}</h3>
          <p>
            {t("dash.openDataBody")}{" "}
            <a className="text-blue-700 underline" href="https://creativecommons.org/licenses/by/4.0/">
              CC BY 4.0
            </a>
          </p>
          <p className="flex gap-3">
            <a className="text-blue-700 underline font-medium" href={openDataUrl("csv", period)}>
              {t("dash.downloadCsv")}
            </a>
            <a className="text-blue-700 underline font-medium" href={openDataUrl("json", period)}>
              {t("dash.downloadJson")}
            </a>
          </p>
        </section>
      </div>
    </main>
  );
}
