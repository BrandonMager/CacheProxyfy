"use client"

import { useRouter, useSearchParams } from "next/navigation";

export const TIME_WINDOWS = [
  { label: "24h",    value: "24h"  },
  { label: "1 week", value: "168h" },
  { label: "YTD",    value: "ytd"  },
] as const;

export type TimeWindow = typeof TIME_WINDOWS[number]["value"];

export const TimeFilter = ({ active }: { active: TimeWindow }) => {
  const router = useRouter();
  const params = useSearchParams();

  const handleClick = (value: TimeWindow) => {
    const next = new URLSearchParams(params.toString());
    if (value === "24h") {
      next.delete("since");
    } else {
      next.set("since", value);
    }
    router.push(`?${next.toString()}`);
  };

  return (
    <div className="flex gap-1 p-1 rounded-lg bg-gray-100 dark:bg-gray-800">
      {TIME_WINDOWS.map(({ label, value }) => (
        <button
          key={value}
          onClick={() => handleClick(value)}
          className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
            active === value
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
