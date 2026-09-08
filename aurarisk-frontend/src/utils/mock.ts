import type { RiskAssessment, CommunityReport, Location } from "../types";
import { scoreToLevel } from "./risk";

export function getMockRisk(location: Location): RiskAssessment {
  const raw = Math.abs(Math.sin(location.lat * 100) * Math.cos(location.lon * 100)) * 100;
  const score = Math.round(raw);
  const level = scoreToLevel(score);

  const tipsByLevel: Record<string, string[]> = {
    Normal: [
      "No immediate flood threat detected.",
      "Keep drainage areas clear of debris.",
    ],
    Advisory: [
      "Monitor local weather updates regularly.",
      "Clear gutters and storm drains near your property.",
      "Review your emergency contact list.",
    ],
    Alert: [
      "Prepare an emergency go-bag with essentials.",
      "Move vehicles to higher ground if possible.",
      "Avoid walking or driving through floodwater.",
      "Charge all devices and keep flashlights ready.",
    ],
    Emergency: [
      "Evacuate immediately if instructed by authorities.",
      "Move to the highest floor of your building.",
      "Do NOT attempt to cross flowing water.",
      "Call emergency services if trapped.",
    ],
  };

  return {
    location,
    score,
    level,
    summary: `Flood risk for ${location.name} is currently ${level.toLowerCase()} with a risk score of ${score}/100.`,
    tips: tipsByLevel[level],
  };
}

export function getMockReports(): CommunityReport[] {
  return [
    {
      id: "r1",
      location: { name: "Downtown Bridge", lat: 14.5995, lon: 120.9842 },
      category: "flooding",
      note: "Water up to knee level on the main road.",
      timestamp: new Date(Date.now() - 1800000).toISOString(),
    },
    {
      id: "r2",
      location: { name: "Market Street", lat: 14.601, lon: 120.986 },
      category: "road_blocked",
      note: "Fallen tree blocking both lanes.",
      timestamp: new Date(Date.now() - 3600000).toISOString(),
    },
    {
      id: "r3",
      location: { name: "Riverside Park", lat: 14.597, lon: 120.982 },
      category: "water_rising",
      note: "River level rising fast, about 1m below bridge.",
      timestamp: new Date(Date.now() - 900000).toISOString(),
    },
  ];
}
