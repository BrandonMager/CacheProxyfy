import { EcosystemBadge } from "@/components/ui/ecosystem-badge";
import { SeverityBadge } from "@/components/ui/severity-badge";
import { OutcomeBadge } from "./outcome-badge";
import type { CVEAlert } from "@/types/api";

const formatDate = (iso: string) =>
  new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });

export const AlertRow = ({ alert }: { alert: CVEAlert }) => (
  <div className="grid grid-cols-[90px_160px_1fr_100px_130px_90px_120px] items-center gap-4 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors">
    <div className="flex items-center">
      <SeverityBadge severity={alert.severity} />
    </div>
    <span className="text-sm font-mono text-gray-700 dark:text-gray-300 truncate">
      {alert.cve_id}
    </span>
    <span className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">
      {alert.name}
    </span>
    <div className="flex items-center">
      <EcosystemBadge ecosystem={alert.ecosystem} />
    </div>
    <span className="text-sm text-gray-500 dark:text-gray-400 font-mono truncate">
      {alert.version}
    </span>
    <div className="flex items-center">
      <OutcomeBadge outcome={alert.outcome} />
    </div>
    <span className="text-sm text-gray-500 dark:text-gray-400">
      {formatDate(alert.recorded_at)}
    </span>
  </div>
);
