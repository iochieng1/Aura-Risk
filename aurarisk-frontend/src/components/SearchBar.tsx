import { useState, useRef, useEffect } from "react";
import type { Location } from "../types";
import { searchLocations } from "../api/nominatim";

interface Props {
  onSelect: (loc: Location) => void;
}

export default function SearchBar({ onSelect }: Props) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Location[]>([]);
  const [open, setOpen] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const wrapper = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (wrapper.current && !wrapper.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  function handleChange(value: string) {
    setQuery(value);
    clearTimeout(timer.current);
    if (value.length < 3) {
      setResults([]);
      setOpen(false);
      return;
    }
    timer.current = setTimeout(async () => {
      const locs = await searchLocations(value);
      setResults(locs);
      setOpen(locs.length > 0);
    }, 400);
  }

  function handleSelect(loc: Location) {
    setQuery(loc.name.split(",")[0]);
    setOpen(false);
    onSelect(loc);
  }

  return (
    <div ref={wrapper} className="relative">
      <input
        type="text"
        value={query}
        onChange={(e) => handleChange(e.target.value)}
        placeholder="Search a location…"
        className="w-full px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent bg-white"
      />
      {open && (
        <ul className="absolute z-30 w-full mt-1 bg-white border border-gray-200 rounded-lg shadow-lg max-h-60 overflow-y-auto">
          {results.map((loc, i) => (
            <li key={i}>
              <button
                onClick={() => handleSelect(loc)}
                className="w-full text-left px-3 py-2 text-sm hover:bg-blue-50 transition truncate"
                title={loc.name}
              >
                {loc.name}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
