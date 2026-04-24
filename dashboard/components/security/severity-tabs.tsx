"use client"

export const SEVERITY_TABS = ["All", "CRITICAL", "HIGH", "MEDIUM", "LOW"] as const;
export type SeverityTab = typeof SEVERITY_TABS[number];

const dotStyles: Record<string, string> = {
  CRITICAL: "bg-red-500",
  HIGH:     "bg-orange-500",
  MEDIUM:   "bg-yellow-500",
  LOW:      "bg-green-500",
};

export const SeverityTabs = ({
  active,
  onChange,
}: {
  active: SeverityTab;
  onChange: (tab: SeverityTab) => void;
}) => (
  <div className="flex gap-1 p-1 rounded-lg bg-gray-100 dark:bg-gray-800">
    {SEVERITY_TABS.map((tab) => (
      <button
        key={tab}
        onClick={() => onChange(tab)}
        className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
          active === tab
            ? "bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 shadow-sm"
            : "text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200"
        }`}
      >
        {tab !== "All" && (
          <span className={`h-1.5 w-1.5 rounded-full ${dotStyles[tab]}`} />
        )}
        {tab}
      </button>
    ))}
  </div>
);
