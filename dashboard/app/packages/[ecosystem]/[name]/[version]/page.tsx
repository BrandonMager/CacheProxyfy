import { notFound } from "next/navigation";
import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { PackageDetail } from "@/components/packages/package-detail";
import { getPackage } from "@/lib/api";

export default async function PackageDetailPage({
  params,
}: {
  params: Promise<{ ecosystem: string; name: string; version: string }>;
}) {
  const { ecosystem, name, version } = await params;
  const decodedName = decodeURIComponent(name);
  const decodedVersion = decodeURIComponent(version);

  const pkg = await getPackage(ecosystem, decodedName, decodedVersion).catch(
    () => null
  );

  if (!pkg) notFound();

  return (
    <SidebarLayout
      title={decodedName}
      subtitle={`${ecosystem} · ${decodedVersion}`}
    >
      <PackageDetail pkg={pkg} />
    </SidebarLayout>
  );
}
