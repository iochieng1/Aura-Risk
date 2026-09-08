import { useEffect, useRef } from "react";
import maplibregl from "maplibre-gl";
import type { Location, RiskAssessment, CommunityReport } from "../types";
import { RISK_CONFIG } from "../utils/risk";

const REPORT_ICONS: Record<string, string> = {
  flooding: "🌊",
  road_blocked: "🚧",
  water_rising: "📈",
  drainage_issue: "🕳️",
};

interface Props {
  center: [number, number];
  risk: RiskAssessment | null;
  reports: CommunityReport[];
}

export default function MapView({ center, risk, reports }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);
  const markersRef = useRef<maplibregl.Marker[]>([]);

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;

    const map = new maplibregl.Map({
      container: containerRef.current,
      style: "https://tiles.openfreemap.org/styles/bright",
      center,
      zoom: 11,
    });

    map.addControl(new maplibregl.NavigationControl(), "bottom-right");
    mapRef.current = map;

    return () => {
      map.remove();
      mapRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!mapRef.current) return;
    mapRef.current.flyTo({ center, zoom: 13, duration: 1200 });
  }, [center]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    const sourceId = "risk-zone";
    const layerId = "risk-zone-fill";

    map.on("load", () => {
      if (!risk) return;

      if (!map.getSource(sourceId)) {
        map.addSource(sourceId, {
          type: "geojson",
          data: {
            type: "Feature",
            geometry: {
              type: "Point",
              coordinates: [risk.location.lon, risk.location.lat],
            },
            properties: {},
          },
        });

        map.addLayer({
          id: layerId,
          type: "circle",
          source: sourceId,
          paint: {
            "circle-radius": 80,
            "circle-color": RISK_CONFIG[risk.level].mapColor,
            "circle-opacity": RISK_CONFIG[risk.level].mapOpacity,
            "circle-stroke-width": 2,
            "circle-stroke-color": RISK_CONFIG[risk.level].mapColor,
            "circle-stroke-opacity": 0.6,
          },
        });
      } else {
        (map.getSource(sourceId) as maplibregl.GeoJSONSource).setData({
          type: "Feature",
          geometry: {
            type: "Point",
            coordinates: [risk.location.lon, risk.location.lat],
          },
          properties: {},
        });
        map.setPaintProperty(layerId, "circle-color", RISK_CONFIG[risk.level].mapColor);
        map.setPaintProperty(layerId, "circle-opacity", RISK_CONFIG[risk.level].mapOpacity);
        map.setPaintProperty(layerId, "circle-stroke-color", RISK_CONFIG[risk.level].mapColor);
      }
    });
  }, [risk]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    markersRef.current.forEach((m) => m.remove());
    markersRef.current = [];

    reports.forEach((r) => {
      const el = document.createElement("div");
      el.className = "text-xl cursor-pointer drop-shadow-md";
      el.textContent = REPORT_ICONS[r.category] || "📌";
      el.title = r.note;

      const marker = new maplibregl.Marker({ element: el })
        .setLngLat([r.location.lon, r.location.lat])
        .setPopup(
          new maplibregl.Popup({ offset: 25, closeButton: false }).setHTML(
            `<div style="font-size:12px;max-width:200px">
              <strong>${r.category.replace("_", " ")}</strong><br/>
              ${r.note}
            </div>`
          )
        )
        .addTo(map);

      markersRef.current.push(marker);
    });
  }, [reports]);

  return <div ref={containerRef} className="w-full h-full" />;
}
