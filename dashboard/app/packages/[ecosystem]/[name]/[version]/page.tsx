import { notFound } from "next/navigation";
import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { PackageDetail } from "@/components/packages/package-detail";
import { CVEAlertsSection } from "@/components/packages/cve-alerts-section";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { getPackage, listPackageCVEAlerts } from "@/lib/api";

export default async function PackageDetailPage({
  params,
}: {
  params: Promise<{ ecosystem: string; name: string; version: string }>;
}) {
  const { ecosystem, name, version } = await params;
  const decodedName = decodeURIComponent(name);
  const decodedVersion = decodeURIComponent(version);

  const [pkg, alerts] = await Promise.all([
    getPackage(ecosystem, decodedName, decodedVersion).catch(() => null),
    listPackageCVEAlerts(ecosystem, decodedName, decodedVersion).catch(() => []),
  ]);

  if (!pkg) notFound();

  return (
    <SidebarLayout
      title={decodedName}
      subtitle={`${ecosystem} · ${decodedVersion}`}
    >
      <Breadcrumb
        crumbs={[
          { label: "Packages", href: "/packages" },
          { label: decodedName, href: `/packages/${ecosystem}/${encodeURIComponent(decodedName)}` },
          { label: decodedVersion },
        ]}
      />
      <PackageDetail pkg={pkg} />
      <CVEAlertsSection alerts={alerts} />
    </SidebarLayout>
  );
}
