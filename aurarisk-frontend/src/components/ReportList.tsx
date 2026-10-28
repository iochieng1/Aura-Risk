import { reportStatusLabel } from "@aurarisk/shared";
import type { CommunityReport } from "../types";
import { useT } from "../i18n";

const ICONS: Record<string, string> = {
  flooding: "🌊",
  road_blocked: "🚧",
  water_rising: "📈",
  drainage_issue: "🕳️",
};

function timeAgo(ts: string, t: ReturnType<typeof useT>["t"]): string {
  const diff = Math.floor((Date.now() - new Date(ts).getTime()) / 60000);
  if (diff < 1) return t("age.justNow");
  if (diff < 60) return t("time.minutesAgo", { n: diff });
  if (diff < 48 * 60) return t("time.hoursAgo", { n: Math.floor(diff / 60) });
  return t("time.daysAgo", { n: Math.floor(diff / (24 * 60)) });
}

export default function ReportList({ reports }: { reports: CommunityReport[] }) {
  const { lang, t } = useT();
  if (!reports.length) {
    return (
      <p className="text-xs text-gray-400 italic">{t("web.noReports")}</p>
    );
  }

  return (
    <div>
      <h3 className="text-sm font-semibold text-gray-700 mb-2">
        {t("web.communityReports", { count: reports.length })}
      </h3>
      <ul className="space-y-2">
        {reports.map((r) => (
          <li
            key={r.id}
            className="flex items-start gap-2 p-2 bg-white border border-gray-100 rounded-lg"
          >
            <span className="text-base mt-0.5">{ICONS[r.category] || "📌"}</span>
            <div className="min-w-0">
              <p className="text-xs text-gray-800 leading-snug">{r.note}</p>
              <p className="text-[10px] text-gray-400 mt-0.5 truncate">
                {r.location.name}
                {r.ward ? ` · ${r.ward}` : ""}
                {r.county ? ` · ${r.county}` : ""} · {timeAgo(r.timestamp, t)} ·{" "}
                <span
                  className={
                    r.status === "verified" ? "text-green-700 font-medium" : undefined
                  }
                >
                  {reportStatusLabel(r, lang)}
                </span>
              </p>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
