"use client"
import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { StatCard } from "@/components/ui/stat-card";
import { EcosystemBadge } from "@/components/ui/ecosystem-badge";
import { formatBytes } from "@/lib/format";
import { Package, HardDrive, Zap, Shield } from "lucide-react";

export default function Home() {
  return (
    <SidebarLayout title="Overview" subtitle="Cache performance for the last 24 hours">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        <StatCard icon={Package}   label="Packages Cached"  value="—" color="blue" />
        <StatCard icon={Zap}       label="Cache Hit Rate"   value="—" color="green" />
        <StatCard icon={HardDrive} label="Bandwidth Saved"  value="—" color="purple" />
        <StatCard icon={Shield}    label="CVE Alerts"       value="—" color="red" />
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-6 shadow-sm">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">Recently Cached</h3>
        <div className="space-y-3">
          {[
            { name: "lodash", version: "4.17.21", ecosystem: "npm", size: 316680 },
            { name: "is-odd", version: "3.0.1",   ecosystem: "npm", size: 2774 },
          ].map((pkg, i) => (
            <div key={i} className="flex items-center justify-between p-3 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors">
              <div className="flex items-center gap-3">
                <EcosystemBadge ecosystem={pkg.ecosystem} />
                <span className="text-sm font-medium text-gray-900 dark:text-gray-100">{pkg.name}</span>
                <span className="text-xs text-gray-500 dark:text-gray-400">v{pkg.version}</span>
              </div>
              <span className="text-sm text-gray-500 dark:text-gray-400">{formatBytes(pkg.size)}</span>
            </div>
          ))}
        </div>
      </div>
    </SidebarLayout>
  );
}
