export default function TipsList({ tips }: { tips: string[] }) {
  if (!tips.length) return null;
  return (
    <div>
      <h3 className="text-sm font-semibold text-gray-700 mb-2">Preparedness Tips</h3>
      <ul className="space-y-1.5">
        {tips.map((tip, i) => (
          <li key={i} className="flex items-start gap-2 text-xs text-gray-600">
            <span className="mt-0.5 w-1.5 h-1.5 rounded-full bg-blue-400 shrink-0" />
            {tip}
          </li>
        ))}
      </ul>
    </div>
  );
}
