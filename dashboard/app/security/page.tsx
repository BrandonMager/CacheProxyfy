import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { AlertsTable } from "@/components/security/alerts-table";
import { listCVEAlerts } from "@/lib/api";
import type { TimeWindow } from "@/components/security/time-filter";
import type { EcosystemOption } from "@/components/security/ecosystem-filter";

const VALID_ECOSYSTEMS = new Set(["npm", "pypi", "maven", "go"]);

function ytdHours(): string {
  const now = new Date();
  const startOfYear = new Date(now.getFullYear(), 0, 1);
  const hours = Math.ceil((now.getTime() - startOfYear.getTime()) / 36e5);
  return `${hours}h`;
}

export default async function SecurityPage({
  searchParams,
}: {
  searchParams: Promise<{ since?: string; ecosystem?: string }>;
}) {
  const { since: raw, ecosystem: rawEco } = await searchParams;
  const window: TimeWindow = (raw === "168h" || raw === "ytd") ? raw : "24h";
  const since = window === "ytd" ? ytdHours() : window;

  const activeEcosystem: EcosystemOption =
    rawEco && VALID_ECOSYSTEMS.has(rawEco) ? (rawEco as EcosystemOption) : "All";
  const ecosystem = activeEcosystem === "All" ? undefined : activeEcosystem;

  const alerts = await listCVEAlerts(since, ecosystem).catch(() => []);

  return (
    <SidebarLayout
      title="Security"
      subtitle="CVE alerts detected across cached packages"
    >
      <AlertsTable alerts={alerts} activeWindow={window} activeEcosystem={activeEcosystem} />
    </SidebarLayout>
  );
}
