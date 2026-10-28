import { useEffect, useState } from "react";

export type View = "map" | "counties";

const viewFromHash = (): View => (window.location.hash === "#/counties" ? "counties" : "map");

/** The page to show, from the URL hash (#/counties), so links and the back button work. */
export function useHashView(): View {
  const [view, setView] = useState<View>(viewFromHash);
  useEffect(() => {
    const onChange = () => setView(viewFromHash());
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);
  return view;
}
