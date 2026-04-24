import { Suspense } from "react";
import { redirect } from "next/navigation";
import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { StatsGrid } from "@/components/metrics/stats-grid";
import { HitRateBar } from "@/components/metrics/hit-rate-bar";
import { TimeRangeTabs, type Since } from "@/components/metrics/time-range-tabs";
import { getStats } from "@/lib/api";

const VALID_SINCE = new Set<string>(["24h", "168h", "720h"]);

const SINCE_LABEL: Record<Since, string> = {
  "24h":  "Last 24 hours",
  "168h": "Last 7 days",
  "720h": "Last 30 days",
};

export default async function MetricsPage({
  searchParams,
}: {
  searchParams: Promise<{ since?: string }>;
}) {
  const { since: raw } = await searchParams;
  if (raw !== undefined && !VALID_SINCE.has(raw)) redirect("/metrics?since=24h");
  const since: Since = (raw as Since) ?? "24h";

  const stats = await getStats(since).catch(() => null);

  return (
    <SidebarLayout title="Metrics" subtitle="Cache performance statistics">
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium text-gray-500 dark:text-gray-400">
            {SINCE_LABEL[since]}
          </p>
          <Suspense>
            <TimeRangeTabs active={since} />
          </Suspense>
        </div>

        <StatsGrid stats={stats} />
        <HitRateBar stats={stats} />
      </div>
    </SidebarLayout>
  );
}
