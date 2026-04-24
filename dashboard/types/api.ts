export interface Package {
  id: number;
  ecosystem: string;
  name: string;
  version: string;
  checksum: string;
  size_bytes: number;
  cached_at: string;
  last_hit_at: string | null;
}

export interface CVEAlert {
  id: number;
  ecosystem: string;
  name: string;
  version: string;
  cve_id: string;
  severity: "CRITICAL" | "HIGH" | "MEDIUM" | "LOW";
  outcome: string;
  recorded_at: string;
}

export interface Stats {
  total_packages: number;
  total_hits: number;
  total_misses: number;
  bytes_saved: number;
  hit_rate: number;
}
