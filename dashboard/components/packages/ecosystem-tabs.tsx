"use client"

export const ECOSYSTEM_TABS = ["All", "npm", "pypi", "maven"] as const;
export type EcosystemTab = typeof ECOSYSTEM_TABS[number];

export const EcosystemTabs = ({
  active,
  onChange,
}: {
  active: EcosystemTab;
  onChange: (tab: EcosystemTab) => void;
}) => (
  <div className="flex gap-1 p-1 rounded-lg bg-gray-100 dark:bg-gray-800">
    {ECOSYSTEM_TABS.map((tab) => (
      <button
        key={tab}
        onClick={() => onChange(tab)}
        className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
          active === tab
            ? "bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 shadow-sm"
            : "text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200"
        }`}
      >
        {tab}
      </button>
    ))}
  </div>
);
