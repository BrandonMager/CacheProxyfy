import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsRow } from "@/components/settings/settings-row";
import { EcosystemBadge } from "@/components/ui/ecosystem-badge";
import { SeverityBadge } from "@/components/ui/severity-badge";
import { getConfig } from "@/lib/api";

const EnabledBadge = ({ enabled }: { enabled: boolean }) => (
  <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium ${
    enabled
      ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300"
      : "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400"
  }`}>
    <span className={`h-1.5 w-1.5 rounded-full ${enabled ? "bg-green-500" : "bg-gray-400"}`} />
    {enabled ? "Enabled" : "Disabled"}
  </span>
);

const Value = ({ children }: { children: React.ReactNode }) => (
  <span className="text-sm text-gray-700 dark:text-gray-300 font-mono">{children}</span>
);

const formatTTL = (hours: number): string => {
  if (hours === 0) return "0 hours";
  if (hours % 720 === 0) return `${hours / 720} day${hours / 720 !== 1 ? "s" : ""} (${hours}h)`;
  if (hours % 24 === 0) return `${hours / 24} day${hours / 24 !== 1 ? "s" : ""} (${hours}h)`;
  return `${hours} hour${hours !== 1 ? "s" : ""}`;
};

export default async function SettingsPage() {
  const cfg = await getConfig().catch(() => null);

  if (!cfg) {
    return (
      <SidebarLayout title="Settings" subtitle="Active proxy configuration">
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Could not load configuration.
        </p>
      </SidebarLayout>
    );
  }

  return (
    <SidebarLayout title="Settings" subtitle="Active proxy configuration — read from cacheproxyfy.yaml">
      <div className="space-y-6">

        <SettingsSection title="Security" description="CVE scanning and severity policy">
          <SettingsRow label="CVE Scanning" description="Scan packages against the OSV vulnerability database">
            <EnabledBadge enabled={cfg.security.cve_scanning} />
          </SettingsRow>
          <SettingsRow label="Block Severity" description="Requests containing CVEs at or above this level are blocked">
            <SeverityBadge severity={cfg.security.block_severity} />
          </SettingsRow>
          <SettingsRow label="Warn Severity" description="Requests containing CVEs at or above this level are flagged">
            <SeverityBadge severity={cfg.security.warn_severity} />
          </SettingsRow>
        </SettingsSection>

        <SettingsSection title="Proxy" description="Inbound traffic and ecosystem routing">
          <SettingsRow label="Port" description="Port the proxy listens on for package requests">
            <Value>{cfg.proxy.port}</Value>
          </SettingsRow>
          <SettingsRow label="Active Ecosystems" description="Package ecosystems currently routed through the proxy">
            <div className="flex items-center gap-1.5">
              {cfg.proxy.ecosystems.map((eco) => (
                <EcosystemBadge key={eco} ecosystem={eco} />
              ))}
            </div>
          </SettingsRow>
        </SettingsSection>

        <SettingsSection title="Cache" description="Artifact storage backend and TTL">
          <SettingsRow label="Backend" description="Storage backend used for cached artifacts">
            <Value>{cfg.cache.backend}</Value>
          </SettingsRow>
          {cfg.cache.backend === "local" && (
            <SettingsRow label="Local Directory" description="Filesystem path where artifacts are stored">
              <Value>{cfg.cache.local_dir}</Value>
            </SettingsRow>
          )}
          <SettingsRow label="TTL" description="How long artifacts are retained before expiry">
            <Value>{formatTTL(cfg.cache.ttl_hours)}</Value>
          </SettingsRow>
        </SettingsSection>

        {cfg.cache.backend === "s3" && (
          <SettingsSection title="S3" description="S3-compatible object storage configuration">
            <SettingsRow label="Bucket">
              <Value>{cfg.s3.bucket || "—"}</Value>
            </SettingsRow>
            <SettingsRow label="Region">
              <Value>{cfg.s3.region}</Value>
            </SettingsRow>
            <SettingsRow label="Endpoint" description="Custom endpoint for MinIO or LocalStack">
              <Value>{cfg.s3.endpoint || "—"}</Value>
            </SettingsRow>
            <SettingsRow label="Key Prefix">
              <Value>{cfg.s3.key_prefix || "—"}</Value>
            </SettingsRow>
          </SettingsSection>
        )}

        <SettingsSection title="Database" description="PostgreSQL connection">
          <SettingsRow label="Host">
            <Value>{cfg.database.host}</Value>
          </SettingsRow>
          <SettingsRow label="Port">
            <Value>{cfg.database.port}</Value>
          </SettingsRow>
          <SettingsRow label="User">
            <Value>{cfg.database.user}</Value>
          </SettingsRow>
          <SettingsRow label="Database">
            <Value>{cfg.database.dbname}</Value>
          </SettingsRow>
          <SettingsRow label="SSL Mode">
            <Value>{cfg.database.sslmode}</Value>
          </SettingsRow>
        </SettingsSection>

        <SettingsSection title="Redis" description="Cache metadata store">
          <SettingsRow label="Address">
            <Value>{cfg.redis.addr}</Value>
          </SettingsRow>
          <SettingsRow label="Database">
            <Value>{cfg.redis.db}</Value>
          </SettingsRow>
        </SettingsSection>

        <SettingsSection title="Logging">
          <SettingsRow label="Level">
            <Value>{cfg.log.level}</Value>
          </SettingsRow>
          <SettingsRow label="Format">
            <Value>{cfg.log.format}</Value>
          </SettingsRow>
        </SettingsSection>

      </div>
    </SidebarLayout>
  );
}
