import type { CVEAlert, Package, Stats } from "@/types/api";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:9090";

async function apiFetch<T>(path: string): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`);
  if (!res.ok) {
    throw new Error(`API error ${res.status}: ${path}`);
  }
  return res.json() as Promise<T>;
}

// GET /api/stats?since=<duration>
// duration: Go duration string e.g. "24h", "168h". Defaults to "24h" when omitted.
export function getStats(since?: string): Promise<Stats> {
  const params = since ? `?since=${encodeURIComponent(since)}` : "";
  return apiFetch<Stats>(`/api/stats${params}`);
}

// GET /api/packages/list[?ecosystem=<eco>]
export function listPackages(ecosystem?: string): Promise<Package[]> {
  const params = ecosystem ? `?ecosystem=${encodeURIComponent(ecosystem)}` : "";
  return apiFetch<Package[]>(`/api/packages/list${params}`);
}

// GET /api/packages?ecosystem=&name=&version=
// Returns the single matching package record.
export function getPackage(
  ecosystem: string,
  name: string,
  version: string
): Promise<Package> {
  const params = new URLSearchParams({ ecosystem, name, version });
  return apiFetch<Package>(`/api/packages?${params}`);
}

// GET /api/packages?ecosystem=&name=
// Returns all cached versions of a package.
export function listVersions(
  ecosystem: string,
  name: string
): Promise<Package[]> {
  const params = new URLSearchParams({ ecosystem, name });
  return apiFetch<Package[]>(`/api/packages?${params}`);
}

// GET /api/cve-alerts?since=<duration>[&ecosystem=<eco>]
// duration: Go duration string e.g. "24h", "168h". Defaults to "24h" when omitted.
export function listCVEAlerts(
  since?: string,
  ecosystem?: string
): Promise<CVEAlert[]> {
  const params = new URLSearchParams();
  if (since) params.set("since", since);
  if (ecosystem) params.set("ecosystem", ecosystem);
  const qs = params.size > 0 ? `?${params}` : "";
  return apiFetch<CVEAlert[]>(`/api/cve-alerts${qs}`);
}
