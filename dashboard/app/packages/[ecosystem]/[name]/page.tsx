import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { VersionsTable } from "@/components/packages/versions-table";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { listVersions } from "@/lib/api";

const PAGE_SIZE = 25;

export default async function PackageVersionsPage({
  params,
  searchParams,
}: {
  params: Promise<{ ecosystem: string; name: string }>;
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  const { ecosystem, name } = await params;
  const sp = await searchParams;
  const decodedName = decodeURIComponent(name);
  const page = Math.max(1, Number(sp.page) || 1);

  const result = await listVersions(ecosystem, decodedName, page, PAGE_SIZE).catch(() => ({
    items: [],
    total: 0,
    page,
    page_size: PAGE_SIZE,
  }));

  const basePath = `/packages/${ecosystem}/${encodeURIComponent(decodedName)}`;

  return (
    <SidebarLayout
      title={decodedName}
      subtitle={`${ecosystem} · ${result.total} cached version${result.total !== 1 ? "s" : ""}`}
    >
      <Breadcrumb
        crumbs={[
          { label: "Packages", href: "/packages" },
          { label: decodedName },
        ]}
      />
      <VersionsTable
        packages={result.items}
        total={result.total}
        page={result.page}
        pageSize={result.page_size}
        basePath={basePath}
      />
    </SidebarLayout>
  );
}
