import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { PackagesTable } from "@/components/packages/packages-table";
import { listPackageSummaries } from "@/lib/api";

const PAGE_SIZE = 25;

export default async function PackagesPage({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  const sp = await searchParams;
  const page = Math.max(1, Number(sp.page) || 1);

  const result = await listPackageSummaries(undefined, page, PAGE_SIZE).catch(() => ({
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
      />
    </SidebarLayout>
  );
}
