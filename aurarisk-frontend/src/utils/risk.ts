import type { RiskLevel } from "../types";
import { RISK_LEVEL_COLORS } from "@aurarisk/shared";

export { scoreToLevel } from "@aurarisk/shared";

export const RISK_CONFIG: Record<
  RiskLevel,
  { label: string; bg: string; text: string; border: string; mapColor: string; mapOpacity: number }
> = {
  Normal: {
    label: "Normal",
    bg: "bg-green-100",
    text: "text-green-800",
    border: "border-green-300",
    mapColor: RISK_LEVEL_COLORS.Normal,
    mapOpacity: 0.15,
  },
  Advisory: {
    label: "Advisory",
    bg: "bg-yellow-100",
    text: "text-yellow-800",
    border: "border-yellow-300",
    mapColor: RISK_LEVEL_COLORS.Advisory,
    mapOpacity: 0.25,
  },
  Alert: {
    label: "Alert",
    bg: "bg-orange-100",
    text: "text-orange-800",
    border: "border-orange-300",
    mapColor: RISK_LEVEL_COLORS.Alert,
    mapOpacity: 0.35,
  },
  Emergency: {
    label: "Emergency",
    bg: "bg-red-100",
    text: "text-red-800",
    border: "border-red-300",
    mapColor: RISK_LEVEL_COLORS.Emergency,
    mapOpacity: 0.45,
  },
};
