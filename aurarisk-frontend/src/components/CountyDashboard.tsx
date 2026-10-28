import { useCallback, useEffect, useState } from "react";
import type { CountyStatsResponse } from "@aurarisk/shared";
import { fetchCountyStats, openDataUrl } from "../api/risk";
import { describeError } from "../api/errors";
import { ErrorState } from "./StateMessages";

const PERIODS = [
  { days: 30, label: "Last 30 days" },
  { days: 90, label: "Last 90 days" },
  { days: 365, label: "Last 12 months" },
] as const;

/** The `days` days up to and including today, as UTC dates. */
function periodFor(days: number) {
  const to = new Date();
  const from = new Date(to.getTime() - (days - 1) * 24 * 60 * 60 * 1000);
  const day = (d: Date) => d.toISOString().slice(0, 10);
  return { from: day(from), to: day(to) };
}

function formatHours(hours: number | null): string {
  if (hours === null) return "–";
  if (hours < 48) return `${hours.toFixed(1)} h`;
  return `${(hours / 24).toFixed(1)} days`;
}

/** Public county response dashboard and open data downloads (GET /api/stats/counties). */
export default function CountyDashboard() {
  const [days, setDays] = useState<number>(90);
  const [stats, setStats] = useState<CountyStatsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const period = periodFor(days);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setStats(await fetchCountyStats(periodFor(days)));
    } catch (err) {
      console.error("Failed to load county stats", err);
      setError(`Couldn't load county statistics. ${describeError(err)}`);
    } finally {
      setLoading(false);
    }
  }, [days]);

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
          <h2 className="text-xl font-semibold text-gray-900">How counties respond to flood reports</h2>
          <p className="mt-1 text-sm text-gray-600">
            Verified community reports per county, whether the county government has acknowledged or resolved
            them, and the median time it took. Times count from when the report was made.
          </p>
        </section>

        <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Period">
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
              {p.label}
            </button>
          ))}
          <span className="text-xs text-gray-500 ml-auto">
            {period.from} to {period.to}
          </span>
        </div>

        {error && <ErrorState message={error} onRetry={load} />}
        {loading && !stats && (
          <p role="status" className="text-sm text-gray-500">
            Loading county statistics…
          </p>
        )}

        {stats && totals && (
          <>
            <dl className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              {[
                ["Verified reports", totals.verified],
                ["Acknowledged by a county", totals.acknowledged],
                ["Resolved", totals.resolved],
              ].map(([label, value]) => (
                <div key={label} className="bg-white border border-gray-200 rounded-xl p-4">
                  <dt className="text-xs text-gray-500">{label}</dt>
                  <dd className="text-2xl font-semibold text-gray-900 tabular-nums">{value}</dd>
                </div>
              ))}
            </dl>

            {stats.counties.length === 0 ? (
              <p className="text-sm text-gray-500 italic">No verified reports with a county in this period.</p>
            ) : (
              <div className="overflow-x-auto bg-white border border-gray-200 rounded-xl">
                <table className="min-w-full text-sm">
                  <caption className="sr-only">County response, {period.from} to {period.to}</caption>
                  <thead className="bg-gray-50 text-left text-xs text-gray-600">
                    <tr>
                      <th scope="col" className="px-3 py-2 font-semibold">County</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">Verified</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">Awaiting</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">Acknowledged</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">Resolved</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">Median to acknowledge</th>
                      <th scope="col" className="px-3 py-2 font-semibold text-right">Median to resolve</th>
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
                        <td className="px-3 py-2 text-right">{formatHours(c.median_hours_to_acknowledge)}</td>
                        <td className="px-3 py-2 text-right">{formatHours(c.median_hours_to_resolve)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}

        <section className="bg-white border border-gray-200 rounded-xl p-4 text-sm text-gray-700 space-y-2">
          <h3 className="font-semibold text-gray-900">Open data</h3>
          <p>
            Download the verified reports for this period. Coordinates are rounded to about 1 km, and notes,
            photos, place names and anything about the reporter are left out. Licensed{" "}
            <a className="text-blue-700 underline" href="https://creativecommons.org/licenses/by/4.0/">
              CC BY 4.0
            </a>
            : credit "AuraRisk community flood reports".
          </p>
          <p className="flex gap-3">
            <a className="text-blue-700 underline font-medium" href={openDataUrl("csv", period)}>
              Download CSV
            </a>
            <a className="text-blue-700 underline font-medium" href={openDataUrl("json", period)}>
              Download JSON
            </a>
          </p>
        </section>
      </div>
    </main>
  );
}
