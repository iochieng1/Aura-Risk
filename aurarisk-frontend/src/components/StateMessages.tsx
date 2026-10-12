/** Shown before any location is chosen. */
export function EmptyState() {
  return (
    <section className="rounded-xl border border-dashed border-gray-300 p-4 text-center">
      <h2 className="text-sm font-semibold text-gray-700">Check flood risk for a place</h2>
      <p className="mt-1 text-xs text-gray-600">
        Search for a town or address above, or use <strong>My Location</strong>, to see its flood risk and
        recent community reports.
      </p>
    </section>
  );
}

/** Placeholder with the risk card's shape while it loads. */
export function LoadingState() {
  return (
    <div role="status" aria-live="polite" className="rounded-xl border border-gray-200 p-4">
      <span className="sr-only">Loading flood risk and reports…</span>
      <div aria-hidden="true" className="animate-pulse motion-reduce:animate-none space-y-3">
        <div className="flex justify-between">
          <div className="h-3 w-20 rounded bg-gray-200" />
          <div className="h-4 w-16 rounded-full bg-gray-200" />
        </div>
        <div className="h-8 w-14 rounded bg-gray-200" />
        <div className="h-3 w-full rounded bg-gray-200" />
        <div className="h-3 w-2/3 rounded bg-gray-200" />
      </div>
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div role="alert" className="text-xs text-red-800 bg-red-50 border border-red-200 rounded-lg p-3">
      <p>{message}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-2 px-3 py-1 font-semibold text-red-800 bg-white border border-red-300 rounded-md hover:bg-red-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
        >
          Try again
        </button>
      )}
    </div>
  );
}

export function OfflineBanner() {
  return (
    <p role="status" className="text-xs text-amber-900 bg-amber-50 border border-amber-200 rounded-lg p-3">
      You're offline. Flood risk, reports and search need a connection; they will reload when you're back
      online.
    </p>
  );
}
