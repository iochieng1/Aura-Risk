import type { CommunityReport } from "../types";

const ICONS: Record<string, string> = {
  flooding: "🌊",
  road_blocked: "🚧",
  water_rising: "📈",
  drainage_issue: "🕳️",
};

function timeAgo(ts: string): string {
  const diff = Math.floor((Date.now() - new Date(ts).getTime()) / 60000);
  if (diff < 1) return "just now";
  if (diff < 60) return `${diff}m ago`;
  return `${Math.floor(diff / 60)}h ago`;
}

export default function ReportList({ reports }: { reports: CommunityReport[] }) {
  if (!reports.length) {
    return (
      <p className="text-xs text-gray-400 italic">No reports in this area yet.</p>
    );
  }

  return (
    <div>
      <h3 className="text-sm font-semibold text-gray-700 mb-2">
        Community Reports ({reports.length})
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
                {r.location.name} · {timeAgo(r.timestamp)}
              </p>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
