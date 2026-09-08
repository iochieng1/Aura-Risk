import { useState } from "react";
import type { Location, ReportCategory } from "../types";

const CATEGORIES: { value: ReportCategory; label: string; icon: string }[] = [
  { value: "flooding", label: "Flooding", icon: "🌊" },
  { value: "road_blocked", label: "Road Blocked", icon: "🚧" },
  { value: "water_rising", label: "Water Rising", icon: "📈" },
  { value: "drainage_issue", label: "Drainage Issue", icon: "🕳️" },
];

interface Props {
  location: Location | null;
  onSubmit: (category: ReportCategory, note: string) => void;
}

export default function ReportForm({ location, onSubmit }: Props) {
  const [category, setCategory] = useState<ReportCategory>("flooding");
  const [note, setNote] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!location || !note.trim()) return;
    onSubmit(category, note.trim());
    setNote("");
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
          disabled={!location || !note.trim()}
          className="w-full py-2 text-xs font-semibold text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-40 disabled:cursor-not-allowed transition"
        >
          Submit Report
        </button>
      </form>
    </div>
  );
}
