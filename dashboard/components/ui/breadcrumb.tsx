import Link from "next/link";

type Crumb = {
  label: string;
  href?: string;
};

export const Breadcrumb = ({ crumbs }: { crumbs: Crumb[] }) => (
  <nav className="flex items-center gap-1.5 text-sm text-gray-500 dark:text-gray-400 mb-6">
    {crumbs.map((crumb, i) => (
      <span key={i} className="flex items-center gap-1.5">
        {i > 0 && <span className="text-gray-300 dark:text-gray-600">/</span>}
        {crumb.href ? (
          <Link
            href={crumb.href}
            className="hover:text-gray-900 dark:hover:text-gray-100 transition-colors"
          >
            {crumb.label}
          </Link>
        ) : (
          <span className="text-gray-900 dark:text-gray-100 font-medium">
            {crumb.label}
          </span>
        )}
      </span>
    ))}
  </nav>
);
