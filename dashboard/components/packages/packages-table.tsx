"use client"

import { useRouter } from "next/navigation";
import type { PackageSummary } from "@/types/api";
import { EcosystemTabs, type EcosystemTab } from "./ecosystem-tabs";
import { PackageRow } from "./package-row";
import { Pagination } from "@/components/ui/pagination";

const COLUMNS = ["Ecosystem", "Name", "Latest Cached", "Versions", "Total Size", "Last Cached", "Last Hit"];

interface PackagesTableProps {
  summaries: PackageSummary[];
  total: number;
  page: number;
  pageSize: number;
  activeEcosystem: EcosystemTab;
}

export const PackagesTable = ({ summaries, total, page, pageSize, activeEcosystem }: PackagesTableProps) => {
  const router = useRouter();

  const handleTabChange = (tab: EcosystemTab) => {
    const params = new URLSearchParams({ page: "1", page_size: String(pageSize) });
    if (tab !== "All") params.set("ecosystem", tab);
    router.push(`/packages?${params}`);
  };

  return (
    <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shadow-sm">
      <div className="flex items-center justify-between p-6 border-b border-gray-200 dark:border-gray-800">
        <div>
          <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
            Cached Packages
          </h3>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
            {total} package{total !== 1 ? "s" : ""}
          </p>
        </div>
        <EcosystemTabs active={activeEcosystem} onChange={handleTabChange} />
      </div>

      <div className="grid grid-cols-[90px_1fr_160px_60px_100px_120px_120px] gap-4 px-4 py-2.5 border-b border-gray-100 dark:border-gray-800 bg-gray-50 dark:bg-gray-800/50 rounded-t-none">
        {COLUMNS.map((col) => (
          <span
            key={col}
            className={`text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide ${
              col === "Ecosystem" || col === "Versions" ? "text-center" : ""
            }`}
          >
            {col}
          </span>
        ))}
      </div>

      <div className="divide-y divide-gray-100 dark:divide-gray-800">
        {summaries.length === 0 ? (
          <p className="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
            No packages found.
          </p>
        ) : (
          summaries.map((s) => (
            <PackageRow key={`${s.ecosystem}:${s.name}`} summary={s} />
          ))
        )}
      </div>

      <div className="border-t border-gray-100 dark:border-gray-800 px-4">
        <Pagination
          page={page}
          pageSize={pageSize}
          total={total}
          buildHref={(p) => {
            const params = new URLSearchParams({ page: String(p), page_size: String(pageSize) });
            if (activeEcosystem !== "All") params.set("ecosystem", activeEcosystem);
            return `/packages?${params}`;
          }}
        />
      </div>
    </div>
  );
};
