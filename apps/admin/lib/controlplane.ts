import "server-only";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { readFile } from "node:fs/promises";
import path from "node:path";

const execFileAsync = promisify(execFile);

// Phase 29's data sources beyond core-api's own API: these deliberately
// reach outside the SDK, for reasons that don't apply to any other page
// in this portal -
//   - platform.json is version-controlled build tooling output
//     (docs/control/build_workbook.py), not production application
//     data - reading it is no different from this app reading its own
//     package.json.
//   - Prometheus and git are themselves platform infrastructure, the
//     literal subject matter of a "platform control plane," not a
//     shortcut around a service API that owns this data instead.
// "Do not bypass service APIs by querying production databases
// directly" (the roadmap's real rule, Phase 25) is about not reaching
// into Postgres behind core-api's back - neither of these is that.

const REPO_ROOT = path.resolve(process.cwd(), "..", "..");
const PROMETHEUS_URL = process.env.PROMETHEUS_URL || "http://localhost:9090";

// --- platform.json (roadmap, modules/DB ownership, dependency graph, events) ---

export type PlatformStat = { label: string; value: string; note: string };
export type RoadmapEntry = { phase: number; status: string; name: string; summary: string; commit: string };
export type ModuleEntry = {
  name: string;
  service: string;
  responsibility: string;
  storage: string;
  accessModel: string;
  introducedPhase: string;
};
export type ComponentEntry = { name: string; description: string; type: string; dependsOn: string[]; system: string };
export type EventEntry = { name: string; address: string };

export type PlatformData = {
  generatedAt: string;
  stats: PlatformStat[];
  roadmap: RoadmapEntry[];
  modules: ModuleEntry[];
  components: ComponentEntry[];
  events: EventEntry[];
  endpointCount: number;
};

export async function getPlatformData(): Promise<PlatformData> {
  const raw = await readFile(path.join(REPO_ROOT, "docs", "control", "platform.json"), "utf-8");
  return JSON.parse(raw) as PlatformData;
}

// --- Prometheus alerts (real, live-evaluated - see infra/observability/alerts.yml) ---

export type PrometheusAlert = {
  labels: Record<string, string>;
  annotations: Record<string, string>;
  state: "inactive" | "pending" | "firing";
  activeAt?: string;
};

export async function getPrometheusAlerts(): Promise<PrometheusAlert[]> {
  try {
    const res = await fetch(`${PROMETHEUS_URL}/api/v1/alerts`, { cache: "no-store", signal: AbortSignal.timeout(3000) });
    if (!res.ok) return [];
    const body = await res.json();
    return (body?.data?.alerts ?? []) as PrometheusAlert[];
  } catch {
    // Prometheus not running locally is a real, common state (it's an
    // optional part of `make local-up`, not always up) - an empty list
    // is the honest answer, not an error the whole page should crash on.
    return [];
  }
}

// --- recent changes (real git log of this actual checkout) ---

export type RecentChange = { sha: string; author: string; date: string; message: string };

export async function getRecentChanges(limit = 15): Promise<RecentChange[]> {
  try {
    const { stdout } = await execFileAsync(
      "git",
      ["-C", REPO_ROOT, "log", `-${limit}`, "--pretty=format:%h%x1f%an%x1f%ad%x1f%s", "--date=short"],
      { timeout: 5000 },
    );
    return stdout
      .split("\n")
      .filter(Boolean)
      .map((line) => {
        const [sha, author, date, message] = line.split("\x1f");
        return { sha, author, date, message };
      });
  } catch {
    // Not a git checkout, or git isn't on PATH - the same honest "dev"
    // fallback config.Version already has on the Go side for this exact
    // situation, just surfaced as an empty list here instead.
    return [];
  }
}
