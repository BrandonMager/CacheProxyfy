import { notFound } from "next/navigation";
import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { CVEDetail } from "@/components/security/cve-detail";
import { getOSVVulnDetail, listCVEAlerts } from "@/lib/api";

// Alerts have been recorded since the proxy went live; 10 years comfortably
// covers "all time" without needing a dedicated all-time API param.
const ALL_TIME_SINCE = "87600h";

export default async function CVEAlertPage({
  params,
}: {
  params: Promise<{ cve_id: string }>;
}) {
  const { cve_id } = await params;
  const cveId = decodeURIComponent(cve_id);

  const [vuln, allAlerts] = await Promise.all([
    getOSVVulnDetail(cveId).catch(() => null),
    listCVEAlerts(ALL_TIME_SINCE).catch(() => []),
  ]);

  const alerts = allAlerts.filter((a) => a.cve_id === cveId);

  if (!vuln && alerts.length === 0) {
    notFound();
  }

  const severity = alerts[0]?.severity ?? "UNKNOWN";

  return (
    <SidebarLayout title={cveId} subtitle="CVE alert details">
      <Breadcrumb
        crumbs={[
          { label: "Security", href: "/security" },
          { label: cveId },
        ]}
      />
      <CVEDetail
        vuln={vuln ?? { id: cveId }}
        alerts={alerts}
        severity={severity}
      />
    </SidebarLayout>
  );
}
