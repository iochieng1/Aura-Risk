interface Props {
  onLocate: () => void;
  onToggleSidebar: () => void;
  sidebarOpen: boolean;
}

export default function Header({ onLocate, onToggleSidebar, sidebarOpen }: Props) {
  return (
    <header className="flex items-center justify-between px-4 py-3 bg-white border-b border-gray-200 shadow-sm z-30 relative">
      <div className="flex items-center gap-2">
        <button
          onClick={onToggleSidebar}
          className="md:hidden p-1.5 rounded-lg hover:bg-gray-100 transition"
          aria-label="Toggle menu"
        >
          <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5 text-gray-700" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            {sidebarOpen ? (
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            ) : (
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h16" />
            )}
          </svg>
        </button>

        <div className="w-8 h-8 rounded-lg bg-blue-600 flex items-center justify-center text-white font-bold text-sm">
          A
        </div>
        <h1 className="text-lg font-semibold tracking-tight">
          AuraRisk <span className="text-gray-400 font-normal text-sm">HydroSense</span>
        </h1>
      </div>
      <button
        onClick={onLocate}
        className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-blue-700 bg-blue-50 rounded-lg hover:bg-blue-100 transition"
        title="Use my location"
      >
        <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M17.657 16.657L13.414 20.9a2 2 0 01-2.828 0l-4.243-4.243a8 8 0 1111.314 0z" />
          <path strokeLinecap="round" strokeLinejoin="round" d="M15 11a3 3 0 11-6 0 3 3 0 016 0z" />
        </svg>
        <span className="hidden sm:inline">My Location</span>
      </button>
    </header>
  );
}
