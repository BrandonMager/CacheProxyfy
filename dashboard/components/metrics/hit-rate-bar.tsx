import type { Stats } from "@/types/api";

export const HitRateBar = ({ stats }: { stats: Stats | null }) => {
  const hits   = stats?.total_hits   ?? 0;
  const misses = stats?.total_misses ?? 0;
  const total  = hits + misses;
  const hitPct = total > 0 ? (hits / total) * 100 : 0;

  return (
    <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-6 shadow-sm">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
            Cache Request Breakdown
          </h3>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
            {total.toLocaleString()} total request{total !== 1 ? "s" : ""}
          </p>
        </div>
        <span className="text-2xl font-bold text-blue-600 dark:text-blue-400">
          {hitPct.toFixed(1)}%
        </span>
      </div>

      <div className="h-3 w-full rounded-full bg-gray-100 dark:bg-gray-800 overflow-hidden mb-4">
        <div
          className="h-full rounded-full bg-blue-500 transition-all duration-500"
          style={{ width: `${hitPct}%` }}
        />
      </div>

      <div className="flex items-center justify-between text-sm">
        <div className="flex items-center gap-2">
          <span className="h-2.5 w-2.5 rounded-full bg-blue-500" />
          <span className="text-gray-600 dark:text-gray-400">Hits</span>
          <span className="font-semibold text-gray-900 dark:text-gray-100 ml-1">
            {hits.toLocaleString()}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="font-semibold text-gray-900 dark:text-gray-100 mr-1">
            {misses.toLocaleString()}
          </span>
          <span className="text-gray-600 dark:text-gray-400">Misses</span>
          <span className="h-2.5 w-2.5 rounded-full bg-gray-300 dark:bg-gray-600" />
        </div>
      </div>
    </div>
  );
};
