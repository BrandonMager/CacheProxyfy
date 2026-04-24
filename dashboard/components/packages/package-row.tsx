import { EcosystemBadge } from "@/components/ui/ecosystem-badge";
import { formatBytes } from "@/lib/format";
import type { Package } from "@/types/api";

const formatDate = (iso: string) =>
  new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });

export const PackageRow = ({ pkg }: { pkg: Package }) => (
  <div className="grid grid-cols-[90px_1fr_140px_90px_120px_120px] items-center gap-4 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors">
    <div className="flex items-center justify-center">
      <EcosystemBadge ecosystem={pkg.ecosystem} />
    </div>
    <span className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">
      {pkg.name}
    </span>
    <span className="text-sm text-gray-500 dark:text-gray-400 font-mono truncate">
      {pkg.version}
    </span>
    <span className="text-sm text-gray-500 dark:text-gray-400">
      {formatBytes(pkg.size_bytes)}
    </span>
    <span className="text-sm text-gray-500 dark:text-gray-400">
      {formatDate(pkg.cached_at)}
    </span>
    <span className="text-sm text-gray-500 dark:text-gray-400">
      {pkg.last_hit_at ? formatDate(pkg.last_hit_at) : "—"}
    </span>
  </div>
);
