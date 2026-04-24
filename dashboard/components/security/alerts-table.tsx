"use client"

import { useState } from "react";
import type { CVEAlert } from "@/types/api";
import { SeverityTabs, type SeverityTab } from "./severity-tabs";
import { AlertRow } from "./alert-row";

const COLUMNS = ["Severity", "CVE ID", "Package", "Ecosystem", "Version", "Outcome", "Recorded"];

export const AlertsTable = ({ alerts }: { alerts: CVEAlert[] }) => {
  const [activeTab, setActiveTab] = useState<SeverityTab>("All");

  const filtered =
    activeTab === "All"
      ? alerts
      : alerts.filter((a) => a.severity === activeTab);

  return (
    <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shadow-sm">
      <div className="flex items-center justify-between p-6 border-b border-gray-200 dark:border-gray-800">
        <div>
          <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
            CVE Alerts
          </h3>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
            {filtered.length} alert{filtered.length !== 1 ? "s" : ""}
          </p>
        </div>
        <SeverityTabs active={activeTab} onChange={setActiveTab} />
      </div>

      <div className="grid grid-cols-[90px_160px_1fr_100px_130px_90px_120px] gap-4 px-4 py-2.5 border-b border-gray-100 dark:border-gray-800 bg-gray-50 dark:bg-gray-800/50">
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
        {filtered.length === 0 ? (
          <p className="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
            No alerts found.
          </p>
        ) : (
          filtered.map((alert) => <AlertRow key={alert.id} alert={alert} />)
        )}
      </div>
    </div>
  );
};
