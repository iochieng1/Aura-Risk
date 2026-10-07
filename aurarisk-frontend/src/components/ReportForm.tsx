import { useState } from "react";
import type { CommunityReport, Location, ReportCategory } from "../types";

const CATEGORIES: { value: ReportCategory; label: string; icon: string }[] = [
  { value: "flooding", label: "Flooding", icon: "🌊" },
  { value: "road_blocked", label: "Road Blocked", icon: "🚧" },
  { value: "water_rising", label: "Water Rising", icon: "📈" },
  { value: "drainage_issue", label: "Drainage Issue", icon: "🕳️" },
];

interface Props {
  location: Location | null;
  /** Rejects with a user-facing message on failure. */
  onSubmit: (category: ReportCategory, note: string) => Promise<CommunityReport>;
}

export default function ReportForm({ location, onSubmit }: Props) {
  const [category, setCategory] = useState<ReportCategory>("flooding");
  const [note, setNote] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!location || !note.trim() || submitting) return;
    setSubmitting(true);
    setError(null);
    setNotice(null);
    try {
      const report = await onSubmit(category, note.trim());
      // Only clear the note once the server has it, so a failure loses nothing.
      setNote("");
      setNotice(
        report.duplicate_of
          ? "Thanks. A similar report was already filed nearby, so yours was linked to it."
          : "Thanks. Your report was submitted and will show as unverified until it is confirmed."
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not submit the report.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div>
      <h3 className="text-sm font-semibold text-gray-700 mb-2">Submit a Report</h3>
      <form onSubmit={handleSubmit} className="space-y-2">
        <div className="grid grid-cols-2 gap-1.5">
          {CATEGORIES.map((c) => (
            <button
              key={c.value}
              type="button"
              onClick={() => setCategory(c.value)}
              className={`flex items-center gap-1 px-2 py-1.5 text-xs rounded-lg border transition ${
                category === c.value
                  ? "bg-blue-50 border-blue-300 text-blue-700 font-medium"
                  : "bg-white border-gray-200 text-gray-600 hover:bg-gray-50"
              }`}
            >
              <span>{c.icon}</span> {c.label}
            </button>
          ))}
        </div>
        <textarea
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="Describe what you see…"
          rows={2}
          className="w-full px-3 py-2 text-xs border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
        />
        <button
          type="submit"
          disabled={!location || !note.trim() || submitting}
          className="w-full py-2 text-xs font-semibold text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-40 disabled:cursor-not-allowed transition"
        >
          {submitting ? "Submitting…" : "Submit Report"}
        </button>
        {error && (
          <p role="alert" className="text-xs text-red-700">
            {error}
          </p>
        )}
        {notice && (
          <p role="status" className="text-xs text-green-700">
            {notice}
          </p>
        )}
      </form>
    </div>
  );
}
