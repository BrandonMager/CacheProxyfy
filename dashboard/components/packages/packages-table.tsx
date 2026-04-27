"use client"

import { useState } from "react";
import type { PackageSummary } from "@/types/api";
import { EcosystemTabs, type EcosystemTab } from "./ecosystem-tabs";
import { PackageRow } from "./package-row";

const COLUMNS = ["Ecosystem", "Name", "Latest Cached", "Versions", "Total Size", "Last Cached", "Last Hit"];

export const PackagesTable = ({ summaries }: { summaries: PackageSummary[] }) => {
  const [activeTab, setActiveTab] = useState<EcosystemTab>("All");

  const filtered =
    activeTab === "All"
      ? summaries
      : summaries.filter((s) => s.ecosystem === activeTab);

  return (
    <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shadow-sm">
      <div className="flex items-center justify-between p-6 border-b border-gray-200 dark:border-gray-800">
        <div>
          <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
            Cached Packages
          </h3>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
            {filtered.length} package{filtered.length !== 1 ? "s" : ""}
          </p>
        </div>
        <EcosystemTabs active={activeTab} onChange={setActiveTab} />
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
        {filtered.length === 0 ? (
          <p className="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
            No packages found.
          </p>
        ) : (
          filtered.map((s) => (
            <PackageRow key={`${s.ecosystem}:${s.name}`} summary={s} />
          ))
        )}
      </div>
    </div>
  );
};
