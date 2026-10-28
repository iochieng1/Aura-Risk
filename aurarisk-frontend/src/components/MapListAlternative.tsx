import { categoryLabel, reportStatusLabel, riskLevelLabel } from "@aurarisk/shared";
import type { CommunityReport, Location, RiskAssessment } from "../types";
import { useT } from "../i18n";

export const MAP_LIST_ID = "map-contents";

interface Props {
  location: Location | null;
  risk: RiskAssessment | null;
  reports: CommunityReport[];
}

/**
 * What the map shows, as a list, for screen-reader users who can't use the
 * map canvas. Visually hidden; reached from the skip link at the top.
 */
export default function MapListAlternative({ location, risk, reports }: Props) {
  const { lang, t } = useT();
  return (
    <section id={MAP_LIST_ID} tabIndex={-1} aria-label={t("web.mapList")} className="sr-only focus:not-sr-only">
      <h2>{t("web.mapContents")}</h2>
      {!location ? (
        <p>{t("web.mapNoLocation")}</p>
      ) : (
        <>
          <p>
            {t("web.mapCentered", { place: location.name })}
            {risk && ` ${t("web.mapRisk", { level: riskLevelLabel(lang, risk.level), score: risk.score })}`}
          </p>
          {reports.length === 0 ? (
            <p>{t("web.mapNoReports")}</p>
          ) : (
            <>
              <p>
                {reports.length === 1
                  ? t("web.mapOneReport")
                  : t("web.mapReports", { count: reports.length })}
              </p>
              <ul>
                {reports.map((r) => (
                  <li key={r.id}>
                    {t("web.mapReportItem", {
                      category: categoryLabel(lang, r.category),
                      place: r.location.name,
                      time: new Date(r.timestamp).toLocaleString(lang),
                      status: reportStatusLabel(r, lang),
                      note: r.note,
                    })}
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
