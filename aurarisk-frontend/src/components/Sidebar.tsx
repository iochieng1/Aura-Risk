import { useEffect, useRef } from "react";
import type { Location, RiskAssessment, CommunityReport, NewReportDetails } from "../types";
import SearchBar from "./SearchBar";
import RiskCard from "./RiskCard";
import TipsList from "./TipsList";
import ReportForm from "./ReportForm";
import ReportList from "./ReportList";
import { EmptyState, ErrorState, LoadingState, OfflineBanner } from "./StateMessages";
import { useT } from "../i18n";

/** The mobile drawer, which the header's menu button opens. */
export const SIDEBAR_ID = "sidebar-drawer";

interface Props {
  onSelectLocation: (loc: Location) => void;
  risk: RiskAssessment | null;
  reports: CommunityReport[];
  selectedLocation: Location | null;
  loading: boolean;
  loadError: string | null;
  /** Reports failed to load, so an empty list would be misleading. */
  reportsFailed: boolean;
  onRetry: () => void;
  online: boolean;
  /** A message about the last action, e.g. a failed geolocation lookup. */
  notice: string | null;
  onSubmitReport: (details: NewReportDetails) => Promise<CommunityReport>;
  open: boolean;
  onClose: () => void;
}

export default function Sidebar({
  onSelectLocation,
  risk,
  reports,
  selectedLocation,
  loading,
  loadError,
  reportsFailed,
  onRetry,
  online,
  notice,
  onSubmitReport,
  open,
  onClose,
}: Props) {
  const drawer = useRef<HTMLDivElement>(null);
  const { t } = useT();

  // The mobile drawer is modal: focus moves into it, and Escape closes it.
  useEffect(() => {
    if (!open) return;
    drawer.current?.querySelector<HTMLElement>("input, button")?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  const handleSelect = (loc: Location) => {
    onSelectLocation(loc);
    onClose();
  };

  const content = (
    <aside
      aria-label={t("web.sidebarLabel")}
      className="w-full md:w-96 bg-white border-r border-gray-200 flex flex-col overflow-hidden h-full"
    >
      <div className="p-4 border-b border-gray-100">
        <SearchBar onSelect={handleSelect} online={online} />
      </div>

      <div className="flex-1 overflow-y-auto sidebar-scroll p-4 space-y-5">
        {!online && <OfflineBanner />}
        {notice && (
          <p role="status" className="text-xs text-gray-700 bg-gray-50 border border-gray-200 rounded-lg p-3">
            {notice}
          </p>
        )}
        {!selectedLocation && !loading && <EmptyState />}
        {loading && <LoadingState />}
        {loadError && !loading && <ErrorState message={loadError} onRetry={online ? onRetry : undefined} />}
        {risk && !loading && <RiskCard risk={risk} />}
        {risk && !loading && <TipsList tips={risk.tips} />}

        <hr className="border-gray-100" />

        <ReportForm location={selectedLocation} onSubmit={onSubmitReport} />

        <hr className="border-gray-100" />

        {selectedLocation && !loading && !reportsFailed && <ReportList reports={reports} />}
      </div>
    </aside>
  );

  return (
    <>
      <div className="hidden md:flex">{content}</div>

      {open && (
        <div id={SIDEBAR_ID} className="md:hidden fixed inset-0 z-40 flex" role="dialog" aria-modal="true" aria-label={t("web.sidebarLabel")}>
          <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={onClose} aria-hidden="true" />
          <div ref={drawer} className="relative w-80 max-w-[85vw] h-full shadow-2xl animate-slide-in motion-reduce:animate-none">
            {content}
          </div>
        </div>
      )}
    </>
  );
}
