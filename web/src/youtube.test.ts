import { describe, expect, it } from "vitest";
import type { Niche } from "./api/types";
import { sortNiches } from "./components/Measured";
import { parseOutcome } from "./pages/steps/YouTubeConnection";

function niche(name: string, heuristicScore: number, demand: number | null, competition: number | null): Niche {
  return {
    name,
    description: "",
    subNiches: [],
    contentOpportunities: [],
    suggestedFormats: [],
    rationale: "",
    heuristics: {
      audiencePotential: "medium",
      monetizationPotential: "medium",
      competitionEstimate: "medium",
      productionDifficulty: "medium",
      sourceAvailability: "medium",
      originalityPotential: "medium",
    },
    heuristicScore,
    measured: {
      trendScore: null,
      momentumPercent: null,
      demandScore: demand,
      competitionScore: competition,
      searchInterest: null,
      metrics: null,
      collectedAt: null,
      sources: [],
    },
  };
}

describe("sortNiches", () => {
  const list = [niche("Explored", 10, 20, 90), niche("A", 80, null, null), niche("B", 60, 70, 30), niche("C", 50, 40, 10)];

  it("keeps the server order for heuristic sorting", () => {
    expect(sortNiches(list, "heuristic").map((n) => n.name)).toEqual(["Explored", "A", "B", "C"]);
  });

  it("sorts by measured demand, unmeasured last", () => {
    expect(sortNiches(list, "demand").map((n) => n.name)).toEqual(["B", "C", "Explored", "A"]);
  });

  it("sorts by lowest measured competition, unmeasured last", () => {
    expect(sortNiches(list, "competition").map((n) => n.name)).toEqual(["C", "B", "Explored", "A"]);
  });

  it("does not mutate the input", () => {
    sortNiches(list, "demand");
    expect(list[0]?.name).toBe("Explored");
  });
});

describe("parseOutcome", () => {
  it("accepts only known callback outcomes", () => {
    expect(parseOutcome("connected")).toBe("connected");
    expect(parseOutcome("already_connected")).toBe("already_connected");
    expect(parseOutcome("<script>")).toBeNull();
    expect(parseOutcome(null)).toBeNull();
  });
});
