"use client"

import { useRouter } from "next/navigation";

export const TIME_RANGE_TABS = [
  { label: "24h", since: "24h"  },
  { label: "7d",  since: "168h" },
  { label: "30d", since: "720h" },
] as const;

export type Since = typeof TIME_RANGE_TABS[number]["since"];

export const TimeRangeTabs = ({ active }: { active: Since }) => {
  const router = useRouter();

  return (
    <div className="flex gap-1 p-1 rounded-lg bg-gray-100 dark:bg-gray-800">
      {TIME_RANGE_TABS.map(({ label, since }) => (
        <button
          key={since}
          onClick={() => router.push(`/metrics?since=${since}`)}
          className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
            active === since
              ? "bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 shadow-sm"
              : "text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200"
          }`}
        >
          {label}
        </button>
      ))}
    </div>
  );
};
