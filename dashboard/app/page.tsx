import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { StatCard } from "@/components/ui/stat-card";
import { EcosystemBadge } from "@/components/ui/ecosystem-badge";
import { formatBytes } from "@/lib/format";
import { getStats, listPackages, listCVEAlerts } from "@/lib/api";
import { Package, HardDrive, Zap, Shield } from "lucide-react";

export default async function Home() {
  const [stats, packages, alerts] = await Promise.all([
    getStats().catch(() => null),
    listPackages().catch(() => []),
    listCVEAlerts().catch(() => []),
  ]);

  const packagesLabel   = stats?.total_packages != null ? String(stats.total_packages) : "—";
  const hitRateLabel    = stats?.hit_rate       != null ? `${(stats.hit_rate * 100).toFixed(1)}%` : "—";
  const bytesSavedLabel = stats?.bytes_saved    != null ? formatBytes(stats.bytes_saved) : "—";
  const alertsLabel     = String(alerts.length);

  const recent = packages.slice(0, 5);

  return (
    <SidebarLayout title="Overview" subtitle="Cache performance for the last 24 hours">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        <StatCard icon={Package}   label="Packages Cached" value={packagesLabel}   color="blue" />
        <StatCard icon={Zap}       label="Cache Hit Rate"  value={hitRateLabel}    color="green" />
        <StatCard icon={HardDrive} label="Bandwidth Saved" value={bytesSavedLabel} color="purple" />
        <StatCard icon={Shield}    label="CVE Alerts"      value={alertsLabel}     color="red" />
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
                </div>
                <span className="text-sm text-gray-500 dark:text-gray-400">{formatBytes(pkg.size_bytes)}</span>
              </div>
            ))
          )}
        </div>
      </div>
    </SidebarLayout>
  );
}
