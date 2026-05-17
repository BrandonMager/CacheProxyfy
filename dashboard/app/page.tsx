import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { StatCard } from "@/components/ui/stat-card";
import { EcosystemBadge } from "@/components/ui/ecosystem-badge";
import { formatBytes, formatCost } from "@/lib/format";
import { getStats, listPackages } from "@/lib/api";
import { Package, HardDrive, Zap, Shield, ShieldBan, DollarSign } from "lucide-react";

export default async function Home() {
  // Read inside the component so Next.js evaluates this at request time during
  // dynamic rendering, not once at module load. Defaults to $0.09/GB (typical
  // CDN egress rate). Override via COST_PER_GB environment variable.
  const costPerGb = Math.max(0, parseFloat(process.env.COST_PER_GB ?? "0.09") || 0.09);
  const [stats, packages] = await Promise.all([
    getStats().catch(() => null),
    listPackages().catch(() => []),
  ]);

  const packagesLabel   = stats?.total_packages   != null ? String(stats.total_packages) : "—";
  const blockedLabel    = stats?.blocked_packages != null ? String(stats.blocked_packages) : "—";
  const hitRateLabel    = stats?.hit_rate         != null ? `${(stats.hit_rate * 100).toFixed(1)}%` : "—";
  const bytesSavedLabel = stats?.bytes_saved      != null ? formatBytes(stats.bytes_saved) : "—";
  const alertsLabel     = stats?.cve_alerts       != null ? String(stats.cve_alerts) : "—";
  const costSavedLabel  = stats?.bytes_saved      != null
    ? formatCost((stats.bytes_saved / (1024 ** 3)) * costPerGb)
    : "—";

  const recent = packages.slice(0, 5);

  return (
    <SidebarLayout title="Overview" subtitle="Cache performance for the last 24 hours">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-6 mb-8">
        <StatCard icon={Package}     label="Packages Cached" value={packagesLabel}   color="blue" />
        <StatCard icon={ShieldBan}   label="Blocked"         value={blockedLabel}    color="orange" />
        <StatCard icon={Zap}         label="Cache Hit Rate"  value={hitRateLabel}    color="green" />
        <StatCard icon={HardDrive}   label="Bandwidth Saved" value={bytesSavedLabel} color="purple" />
        <StatCard icon={Shield}      label="CVE Alerts"      value={alertsLabel}     color="red" />
        <StatCard icon={DollarSign}  label="Est. Cost Saved" value={costSavedLabel}  color="green" sub={`@ $${costPerGb.toFixed(2)}/GB`} />
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-6 shadow-sm">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">Recently Cached</h3>
        <div className="space-y-3">
          {recent.length === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400">No packages cached yet.</p>
          ) : (
            recent.map((pkg) => (
              <div key={pkg.id} className="flex items-center justify-between p-3 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors">
                <div className="flex items-center gap-3">
                  <EcosystemBadge ecosystem={pkg.ecosystem} />
                  <span className="text-sm font-medium text-gray-900 dark:text-gray-100">{pkg.name}</span>
                  <span className="text-xs text-gray-500 dark:text-gray-400">v{pkg.version}</span>
                  {pkg.status === "blocked" && (
                    <span className="inline-flex items-center rounded-full bg-red-100 dark:bg-red-900/30 px-2 py-0.5 text-xs font-medium text-red-700 dark:text-red-400">
                      Blocked
                    </span>
                  )}
                </div>
                <span className="text-sm text-gray-500 dark:text-gray-400">{pkg.status === "blocked" ? "—" : formatBytes(pkg.size_bytes)}</span>
              </div>
            ))
          )}
        </div>
      </div>
    </SidebarLayout>
  );
}
