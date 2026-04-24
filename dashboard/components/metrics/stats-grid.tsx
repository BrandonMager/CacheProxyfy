import { Package, Zap, TrendingUp, TrendingDown, HardDrive } from "lucide-react";
import { StatCard } from "@/components/ui/stat-card";
import { formatBytes } from "@/lib/format";
import type { Stats } from "@/types/api";

export const StatsGrid = ({ stats }: { stats: Stats | null }) => {
  const packages  = stats?.total_packages != null ? stats.total_packages.toLocaleString() : "—";
  const hitRate   = stats?.hit_rate       != null ? `${(stats.hit_rate * 100).toFixed(1)}%` : "—";
  const hits      = stats?.total_hits     != null ? stats.total_hits.toLocaleString() : "—";
  const misses    = stats?.total_misses   != null ? stats.total_misses.toLocaleString() : "—";
  const bandwidth = stats?.bytes_saved    != null ? formatBytes(stats.bytes_saved) : "—";

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5 gap-4">
      <StatCard icon={Package}      label="Packages Cached" value={packages}  color="blue"   />
      <StatCard icon={Zap}          label="Hit Rate"        value={hitRate}   color="green"  />
      <StatCard icon={TrendingUp}   label="Total Hits"      value={hits}      color="green"  />
      <StatCard icon={TrendingDown} label="Total Misses"    value={misses}    color="orange" />
      <StatCard icon={HardDrive}    label="Bandwidth Saved" value={bandwidth} color="purple" />
    </div>
  );
};
