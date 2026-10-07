import type { Location, RiskAssessment, CommunityReport, ReportCategory } from "../types";
import SearchBar from "./SearchBar";
import RiskCard from "./RiskCard";
import TipsList from "./TipsList";
import ReportForm from "./ReportForm";
import ReportList from "./ReportList";

interface Props {
  onSelectLocation: (loc: Location) => void;
  risk: RiskAssessment | null;
  reports: CommunityReport[];
  selectedLocation: Location | null;
  loading: boolean;
  loadError: string | null;
  onSubmitReport: (category: ReportCategory, note: string) => Promise<CommunityReport>;
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
  onSubmitReport,
  open,
  onClose,
}: Props) {
  const handleSelect = (loc: Location) => {
    onSelectLocation(loc);
    onClose();
  };

  const content = (
    <aside className="w-full md:w-96 bg-white border-r border-gray-200 flex flex-col overflow-hidden h-full">
      <div className="p-4 border-b border-gray-100">
        <SearchBar onSelect={handleSelect} />
      </div>

      <div className="flex-1 overflow-y-auto sidebar-scroll p-4 space-y-5">
        {loading && (
          <p role="status" className="text-xs text-gray-500">
            Loading flood risk and reports…
          </p>
        )}
        {loadError && (
          <p role="alert" className="text-xs text-red-700 bg-red-50 border border-red-100 rounded-lg p-2">
            {loadError}
          </p>
        )}
        {risk && <RiskCard risk={risk} />}
        {risk && <TipsList tips={risk.tips} />}

        <hr className="border-gray-100" />

        <ReportForm location={selectedLocation} onSubmit={onSubmitReport} />

        <hr className="border-gray-100" />

        {!loading && <ReportList reports={reports} />}
      </div>
    </aside>
  );

  return (
    <>
      <div className="hidden md:flex">{content}</div>

      {open && (
        <div className="md:hidden fixed inset-0 z-40 flex">
          <div
            className="absolute inset-0 bg-black/40 backdrop-blur-sm"
            onClick={onClose}
          />
          <div className="relative w-80 max-w-[85vw] h-full shadow-2xl animate-slide-in">
            {content}
          </div>
        </div>
      )}
    </>
  );
}
