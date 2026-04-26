import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { VersionsTable } from "@/components/packages/versions-table";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { listVersions } from "@/lib/api";

export default async function PackageVersionsPage({
  params,
}: {
  params: Promise<{ ecosystem: string; name: string }>;
}) {
  const { ecosystem, name } = await params;
  const decodedName = decodeURIComponent(name);

  const versions = await listVersions(ecosystem, decodedName).catch(() => []);
  const count = versions.length;

  return (
    <SidebarLayout
      title={decodedName}
      subtitle={`${ecosystem} · ${count} cached version${count !== 1 ? "s" : ""}`}
    >
      <Breadcrumb
        crumbs={[
          { label: "Packages", href: "/packages" },
          { label: decodedName },
        ]}
      />
      <VersionsTable packages={versions} />
    </SidebarLayout>
  );
}
