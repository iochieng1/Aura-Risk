import type { Location } from "../types";

interface NominatimResult {
  display_name: string;
  lat: string;
  lon: string;
}

export async function searchLocations(query: string): Promise<Location[]> {
  if (query.trim().length < 3) return [];

  const url = `https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(
    query
  )}&limit=5&addressdetails=0`;

  const res = await fetch(url, {
    headers: { "Accept-Language": "en" },
  });

  if (!res.ok) return [];

  const data: NominatimResult[] = await res.json();

  return data.map((r) => ({
    name: r.display_name,
    lat: parseFloat(r.lat),
    lon: parseFloat(r.lon),
  }));
}
