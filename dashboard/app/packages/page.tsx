import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { PackagesTable } from "@/components/packages/packages-table";
import { type EcosystemTab } from "@/components/packages/ecosystem-tabs";
import { listPackageSummaries } from "@/lib/api";

const PAGE_SIZE = 25;

export default async function PackagesPage({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  const sp = await searchParams;
  const page = Math.max(1, Number(sp.page) || 1);
  const ecosystem = typeof sp.ecosystem === "string" ? sp.ecosystem : undefined;
  const activeEcosystem = (ecosystem ?? "All") as EcosystemTab;

  const result = await listPackageSummaries(ecosystem, page, PAGE_SIZE).catch(() => ({
    items: [],
    total: 0,
    page,
    page_size: PAGE_SIZE,
  }));

  return (
    <SidebarLayout
      title="Packages"
      subtitle="All packages currently stored in the cache"
    >
      <PackagesTable
        summaries={result.items}
        total={result.total}
        page={result.page}
        pageSize={result.page_size}
        activeEcosystem={activeEcosystem}
      />
    </SidebarLayout>
  );
}
