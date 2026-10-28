import { forwardRef } from "react";
import { isLang, LANGUAGES } from "@aurarisk/shared";
import { SIDEBAR_ID } from "./Sidebar";
import type { View } from "../hooks/useHashView";
import { useT } from "../i18n";

interface Props {
  view: View;
  onLocate: () => void;
  locating: boolean;
  onToggleSidebar: () => void;
  sidebarOpen: boolean;
}

// The ref is the menu button, so focus can return to it when the drawer closes.
const Header = forwardRef<HTMLButtonElement, Props>(function Header(
  { view, onLocate, locating, onToggleSidebar, sidebarOpen },
  menuButton
) {
  const { lang, setLang, t } = useT();
  return (
    <header className="flex items-center justify-between gap-2 px-4 py-3 bg-white border-b border-gray-200 shadow-sm z-30 relative">
      <div className="flex items-center gap-2 min-w-0">
        <button
          ref={menuButton}
          type="button"
          onClick={onToggleSidebar}
          className={`md:hidden p-1.5 rounded-lg hover:bg-gray-100 transition ${view === "map" ? "" : "hidden"}`}
          aria-label={sidebarOpen ? t("web.closePanel") : t("web.openPanel")}
          aria-expanded={sidebarOpen}
          aria-controls={sidebarOpen ? SIDEBAR_ID : undefined}
        >
          <svg aria-hidden="true" xmlns="http://www.w3.org/2000/svg" className="w-5 h-5 text-gray-700" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            {sidebarOpen ? (
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            ) : (
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h16" />
            )}
          </svg>
        </button>

        <div aria-hidden="true" className="w-8 h-8 shrink-0 rounded-lg bg-blue-600 flex items-center justify-center text-white font-bold text-sm">
          A
        </div>
        <h1 className="text-lg font-semibold tracking-tight sr-only sm:not-sr-only">
          AuraRisk <span className="text-gray-400 font-normal text-sm">HydroSense</span>
        </h1>
      </div>
      <nav aria-label={t("web.pages")} className="flex items-center gap-2">
        <a
          href={view === "map" ? "#/counties" : "#/"}
          className="px-2 sm:px-3 py-1.5 text-sm font-medium text-gray-700 rounded-lg hover:bg-gray-100 transition"
        >
          {view === "map" ? (
            <>
              <span className="sm:hidden">{t("web.counties")}</span>
              <span className="hidden sm:inline">{t("web.countyDashboard")}</span>
            </>
          ) : (
            t("web.floodMap")
          )}
        </a>
        <select
          value={lang}
          onChange={(e) => isLang(e.target.value) && setLang(e.target.value)}
          aria-label={t("ui.language")}
          className="px-1.5 py-1.5 text-sm text-gray-700 bg-white border border-gray-300 rounded-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
        >
          {LANGUAGES.map((l) => (
            <option key={l.value} value={l.value} lang={l.value}>
              {l.label}
            </option>
          ))}
        </select>
        {view === "map" && (
          <button
            type="button"
            onClick={onLocate}
            disabled={locating}
            aria-label={locating ? t("web.findingLocation") : t("web.useMyLocation")}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-blue-700 bg-blue-50 rounded-lg hover:bg-blue-100 disabled:opacity-60 transition"
            title={t("web.useMyLocation")}
          >
            <svg aria-hidden="true" xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M17.657 16.657L13.414 20.9a2 2 0 01-2.828 0l-4.243-4.243a8 8 0 1111.314 0z" />
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 11a3 3 0 11-6 0 3 3 0 016 0z" />
            </svg>
            <span className="hidden sm:inline">{locating ? t("web.locating") : t("web.myLocation")}</span>
          </button>
        )}
      </nav>
    </header>
  );
});

export default Header;
