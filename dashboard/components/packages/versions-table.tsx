import Link from "next/link";
import { formatBytes } from "@/lib/format";
import type { Package } from "@/types/api";

const formatDate = (iso: string) =>
  new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });

const COLUMNS = ["Version", "Size", "Cached", "Last Hit"];

export const VersionsTable = ({
  packages,
}: {
  packages: Package[];
}) => (
  <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shadow-sm">
    <div className="grid grid-cols-[1fr_90px_120px_120px] gap-4 px-4 py-2.5 border-b border-gray-100 dark:border-gray-800 bg-gray-50 dark:bg-gray-800/50 rounded-t-xl">
      {COLUMNS.map((col) => (
        <span
          key={col}
          className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide"
        >
          {col}
        </span>
      ))}
    </div>

    <div className="divide-y divide-gray-100 dark:divide-gray-800">
      {packages.length === 0 ? (
        <p className="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
          No versions found.
        </p>
      ) : (
        packages.map((pkg) => (
          <Link
            key={pkg.id}
            href={`/packages/${pkg.ecosystem}/${encodeURIComponent(pkg.name)}/${encodeURIComponent(pkg.version)}`}
            className="grid grid-cols-[1fr_90px_120px_120px] items-center gap-4 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
          >
            <span className="text-sm font-mono text-gray-900 dark:text-gray-100">
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
          </Link>
        ))
      )}
    </div>
  </div>
);
