import Link from "next/link";

interface PaginationProps {
  page: number;
  pageSize: number;
  total: number;
  buildHref: (page: number) => string;
}

export function Pagination({ page, pageSize, total, buildHref }: PaginationProps) {
  const totalPages = Math.ceil(total / pageSize);

  if (totalPages <= 1) return null;

  return (
    <div className="flex items-center justify-between px-2 py-4 text-sm text-gray-600">
      <span>
        {Math.min((page - 1) * pageSize + 1, total)}–{Math.min(page * pageSize, total)} of {total}
      </span>
      <div className="flex gap-2">
        {page > 1 ? (
          <Link
            href={buildHref(page - 1)}
            className="px-3 py-1 rounded border border-gray-300 hover:bg-gray-100"
          >
            Previous
          </Link>
        ) : (
          <span className="px-3 py-1 rounded border border-gray-200 text-gray-400 cursor-not-allowed">
            Previous
          </span>
        )}
        <span className="px-3 py-1">
          {page} / {totalPages}
        </span>
        {page < totalPages ? (
          <Link
            href={buildHref(page + 1)}
            className="px-3 py-1 rounded border border-gray-300 hover:bg-gray-100"
          >
            Next
          </Link>
        ) : (
          <span className="px-3 py-1 rounded border border-gray-200 text-gray-400 cursor-not-allowed">
            Next
          </span>
        )}
      </div>
    </div>
  );
}
