import { REPORT_CATEGORIES } from "@aurarisk/shared";
import type { CommunityReport, Location, RiskAssessment } from "../types";

export const MAP_LIST_ID = "map-contents";

interface Props {
  location: Location | null;
  risk: RiskAssessment | null;
  reports: CommunityReport[];
}

const categoryLabel = (value: string) => REPORT_CATEGORIES.find((c) => c.value === value)?.label ?? value;

/**
 * What the map shows, as a list, for screen-reader users who can't use the
 * map canvas. Visually hidden; reached from the skip link at the top.
 */
export default function MapListAlternative({ location, risk, reports }: Props) {
  return (
    <section id={MAP_LIST_ID} tabIndex={-1} aria-label="Map contents as a list" className="sr-only focus:not-sr-only">
      <h2>Map contents</h2>
      {!location ? (
        <p>No location selected. Search for a place to show it on the map.</p>
      ) : (
        <>
          <p>
            Map centered on {location.name}.
            {risk && ` Flood risk there is ${risk.level}, score ${risk.score} out of 100.`}
          </p>
          {reports.length === 0 ? (
            <p>No community reports are marked on the map.</p>
          ) : (
            <>
              <p>
                {reports.length === 1
                  ? "1 community report is marked on the map:"
                  : `${reports.length} community reports are marked on the map:`}
              </p>
              <ul>
                {reports.map((r) => (
                  <li key={r.id}>
                    {categoryLabel(r.category)} at {r.location.name}, reported{" "}
                    {new Date(r.timestamp).toLocaleString()}
                    {r.status === "verified" ? ", verified" : ", unverified"}: {r.note}
                  </li>
                ))}
              </ul>
            </>
          )}
        </>
      )}
    </section>
  );
}
