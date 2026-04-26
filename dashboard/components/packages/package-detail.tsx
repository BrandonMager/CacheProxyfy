import { EcosystemBadge } from "@/components/ui/ecosystem-badge";
import { formatBytes } from "@/lib/format";
import type { Package } from "@/types/api";

const formatDate = (iso: string) =>
  new Date(iso).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });

const Row = ({ label, value }: { label: string; value: React.ReactNode }) => (
  <div className="grid grid-cols-[160px_1fr] gap-4 py-3 border-b border-gray-100 dark:border-gray-800 last:border-0">
    <span className="text-sm font-medium text-gray-500 dark:text-gray-400">
      {label}
    </span>
    <span className="text-sm text-gray-900 dark:text-gray-100 font-mono break-all">
      {value}
    </span>
  </div>
);

export const PackageDetail = ({ pkg }: { pkg: Package }) => (
  <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shadow-sm">
    <div className="flex items-center gap-3 p-6 border-b border-gray-200 dark:border-gray-800">
      <EcosystemBadge ecosystem={pkg.ecosystem} />
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
        {pkg.name}
      </h2>
      <span className="text-lg text-gray-400 dark:text-gray-500">
        {pkg.version}
      </span>
    </div>

    <div className="px-6">
      <Row label="Ecosystem" value={pkg.ecosystem} />
      <Row label="Name" value={pkg.name} />
      <Row label="Version" value={pkg.version} />
      <Row label="Size" value={formatBytes(pkg.size_bytes)} />
      <Row label="Checksum" value={pkg.checksum} />
      <Row label="Cached at" value={formatDate(pkg.cached_at)} />
      <Row
        label="Last hit"
        value={pkg.last_hit_at ? formatDate(pkg.last_hit_at) : "—"}
      />
    </div>
  </div>
);
