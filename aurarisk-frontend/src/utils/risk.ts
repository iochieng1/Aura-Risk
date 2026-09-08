import type { RiskLevel } from "../types";

export const RISK_CONFIG: Record<
  RiskLevel,
  { label: string; bg: string; text: string; border: string; mapColor: string; mapOpacity: number }
> = {
  Normal: {
    label: "Normal",
    bg: "bg-green-100",
    text: "text-green-800",
    border: "border-green-300",
    mapColor: "#22c55e",
    mapOpacity: 0.15,
  },
  Advisory: {
    label: "Advisory",
    bg: "bg-yellow-100",
    text: "text-yellow-800",
    border: "border-yellow-300",
    mapColor: "#eab308",
    mapOpacity: 0.25,
  },
  Alert: {
    label: "Alert",
    bg: "bg-orange-100",
    text: "text-orange-800",
    border: "border-orange-300",
    mapColor: "#f97316",
    mapOpacity: 0.35,
  },
  Emergency: {
    label: "Emergency",
    bg: "bg-red-100",
    text: "text-red-800",
    border: "border-red-300",
    mapColor: "#ef4444",
    mapOpacity: 0.45,
  },
};

export function scoreToLevel(score: number): RiskLevel {
  if (score >= 75) return "Emergency";
  if (score >= 50) return "Alert";
  if (score >= 25) return "Advisory";
  return "Normal";
}
