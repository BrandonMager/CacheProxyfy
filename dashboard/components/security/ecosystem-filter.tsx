"use client"

import { useRouter, useSearchParams } from "next/navigation";

export const ECOSYSTEM_OPTIONS = ["All", "npm", "pypi", "maven", "go"] as const;
export type EcosystemOption = typeof ECOSYSTEM_OPTIONS[number];

export const EcosystemFilter = ({ active }: { active: EcosystemOption }) => {
  const router = useRouter();
  const params = useSearchParams();

  const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const value = e.target.value as EcosystemOption;
    const next = new URLSearchParams(params.toString());
    if (value === "All") {
      next.delete("ecosystem");
    } else {
      next.set("ecosystem", value);
    }
    router.push(`?${next.toString()}`);
  };

  return (
    <select
      value={active}
      onChange={handleChange}
      className="h-9 rounded-md border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-3 text-sm font-medium text-gray-700 dark:text-gray-300 focus:outline-none focus:ring-2 focus:ring-blue-500"
    >
      {ECOSYSTEM_OPTIONS.map((value) => (
        <option key={value} value={value}>
          {value}
        </option>
      ))}
    </select>
  );
};
