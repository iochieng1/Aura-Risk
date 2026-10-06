import type { Location } from "../types";
import { searchPlaces } from "@aurarisk/shared";

export function searchLocations(query: string): Promise<Location[]> {
  return searchPlaces(query);
}
