import { SidebarLayout } from "@/components/layout/sidebar-layout";
import { PackagesTable } from "@/components/packages/packages-table";
import { listPackages } from "@/lib/api";

export default async function PackagesPage() {
  const packages = await listPackages().catch(() => []);

  return (
    <SidebarLayout
      title="Packages"
      subtitle="All packages currently stored in the cache"
    >
      <PackagesTable packages={packages} />
    </SidebarLayout>
  );
}
