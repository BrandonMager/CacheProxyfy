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

export interface PackageSummary {
  ecosystem: string;
  name: string;
  latest_version: string;
  version_count: number;
  total_size_bytes: number;
  last_cached_at: string;
  last_hit_at: string | null;
}

export interface Stats {
  total_packages: number;
  total_hits: number;
  total_misses: number;
  bytes_saved: number;
  hit_rate: number;
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
