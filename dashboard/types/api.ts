export interface Package {
  id: number;
  ecosystem: string;
  name: string;
  version: string;
  status: "cached" | "blocked";
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

// OSVVulnDetail mirrors the subset of fields returned by OSV's
// GET https://api.osv.dev/v1/vulns/{id} endpoint that the CVE detail
// page renders. Our own DB only stores id/severity/outcome, so the
// description and stats shown here come straight from OSV.
export interface OSVVulnDetail {
  id: string;
  summary?: string;
  details?: string;
  published?: string;
  modified?: string;
  aliases?: string[];
  references?: { type: string; url: string }[];
  severity?: { type: string; score: string }[];
  affected?: {
    package?: { name: string; ecosystem: string };
    ranges?: { type: string; events: { introduced?: string; fixed?: string }[] }[];
  }[];
}

export interface PackageSummary {
  ecosystem: string;
  name: string;
  latest_version: string;
  version_count: number;
  total_size_bytes: number;
  last_cached_at: string;
  last_hit_at: string | null;
  has_blocked: boolean;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface Stats {
  total_packages: number;
  blocked_packages: number;
  total_hits: number;
  total_misses: number;
  bytes_saved: number;
  hit_rate: number;
  cve_alerts: number;
}

export interface ConfigResponse {
  proxy: {
    port: number;
    ecosystems: string[];
  };
  cache: {
    backend: string;
    local_dir: string;
    ttl_hours: number;
  };
  s3: {
    bucket: string;
    region: string;
    endpoint: string;
    key_prefix: string;
  };
  redis: {
    addr: string;
    db: number;
  };
  database: {
    host: string;
    port: number;
    user: string;
    dbname: string;
    sslmode: string;
  };
  security: {
    cve_scanning: boolean;
    block_severity: string;
    warn_severity: string;
  };
  log: {
    level: string;
    format: string;
  };
}
