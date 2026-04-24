import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { AlertsTable } from "@/components/security/alerts-table";
import { listCVEAlerts } from "@/lib/api";

export default async function SecurityPage() {
  const alerts = await listCVEAlerts().catch(() => []);

  return (
    <SidebarLayout
      title="Security"
      subtitle="CVE alerts detected across cached packages"
    >
      <AlertsTable alerts={alerts} />
    </SidebarLayout>
  );
}
