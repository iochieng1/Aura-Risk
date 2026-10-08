import { useState, useRef, useEffect, type FormEvent } from "react";
import type { Location } from "../types";
import { MIN_PLACE_QUERY_LENGTH } from "@aurarisk/shared";
import { searchLocations } from "../api/nominatim";

interface Props {
  onSelect: (loc: Location) => void;
}

// Nominatim's usage policy forbids search-as-you-type, so searches run only
// on submit (Enter or the Search button).
export default function SearchBar({ onSelect }: Props) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Location[]>([]);
  const [open, setOpen] = useState(false);
  const [searching, setSearching] = useState(false);
  const [searched, setSearched] = useState("");
  const searchSeq = useRef(0);
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
    setOpen(false);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const q = query.trim();
    if (q.length < MIN_PLACE_QUERY_LENGTH) return;
    const seq = ++searchSeq.current;
    setSearching(true);
    const locs = await searchLocations(q);
    // Ignore a slower earlier search that finishes after a newer one.
    if (seq !== searchSeq.current) return;
    setSearching(false);
    setResults(locs);
    setSearched(q);
    setOpen(true);
  }

  function handleSelect(loc: Location) {
    setQuery(loc.name.split(",")[0]);
    setOpen(false);
    onSelect(loc);
  }

  const tooShort = query.trim().length < MIN_PLACE_QUERY_LENGTH;

  return (
    <div ref={wrapper} className="relative">
      <form role="search" onSubmit={handleSubmit} className="flex gap-2">
        <input
          type="search"
          value={query}
          onChange={(e) => handleChange(e.target.value)}
          placeholder="Search a location…"
          aria-label="Search a location"
          className="flex-1 min-w-0 px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent bg-white"
        />
        <button
          type="submit"
          disabled={tooShort || searching}
          className="px-3 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition"
        >
          {searching ? "Searching…" : "Search"}
        </button>
      </form>
      {open && (
        <div className="absolute z-30 w-full mt-1 bg-white border border-gray-200 rounded-lg shadow-lg">
          {results.length === 0 ? (
            <p className="px-3 py-2 text-sm text-gray-600" role="status">
              No matches for “{searched}”. Try a nearby town.
            </p>
          ) : (
            <ul className="max-h-60 overflow-y-auto">
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
          <p className="px-3 py-1 text-[11px] text-gray-500 border-t border-gray-100">
            Search ©{" "}
            <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noreferrer" className="underline">
              OpenStreetMap contributors
            </a>
          </p>
        </div>
      )}
    </div>
  );
}
