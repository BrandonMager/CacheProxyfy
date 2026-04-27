import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { PackagesTable } from "@/components/packages/packages-table";
import { listPackageSummaries } from "@/lib/api";

export default async function PackagesPage() {
  const summaries = await listPackageSummaries().catch(() => []);

  return (
    <SidebarLayout
      title="Packages"
      subtitle="All packages currently stored in the cache"
    >
      <PackagesTable summaries={summaries} />
    </SidebarLayout>
  );
}
