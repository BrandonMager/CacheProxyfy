import Link from "next/link";
import { EcosystemBadge } from "@/components/ui/ecosystem-badge";
import { formatBytes } from "@/lib/format";
import type { PackageSummary } from "@/types/api";

const formatDate = (iso: string) =>
  new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });

export const PackageRow = ({ summary }: { summary: PackageSummary }) => (
  <Link
    href={`/packages/${summary.ecosystem}/${encodeURIComponent(summary.name)}`}
    className="grid grid-cols-[90px_1fr_160px_60px_100px_120px_120px] items-center gap-4 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
  >
    <div className="flex items-center justify-center">
      <EcosystemBadge ecosystem={summary.ecosystem} />
    </div>
    <span className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">
      {summary.name}
    </span>
    <span className="text-sm text-gray-500 dark:text-gray-400 font-mono truncate">
      {summary.latest_version}
    </span>
    <span className="text-sm text-gray-500 dark:text-gray-400 text-center">
      {summary.version_count}
    </span>
    <span className="text-sm text-gray-500 dark:text-gray-400">
      {formatBytes(summary.total_size_bytes)}
    </span>
    <span className="text-sm text-gray-500 dark:text-gray-400">
      {formatDate(summary.last_cached_at)}
    </span>
    <span className="text-sm text-gray-500 dark:text-gray-400">
      {summary.last_hit_at ? formatDate(summary.last_hit_at) : "—"}
    </span>
  </Link>
);
