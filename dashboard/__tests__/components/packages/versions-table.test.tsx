/**
 * getAllByRole returns *all* elements matching a role — useful when you expect
 * multiple of the same element (e.g. a list of rows).
 *
 * queryByText is like getByText but returns null instead of throwing when the
 * element isn't found — the right tool for asserting something is NOT present.
 *
 * within() scopes queries to a specific subtree of the DOM, so you can assert
 * on the contents of a specific row without worrying about other rows.
 */
import { render, screen, within } from "@testing-library/react";
import { VersionsTable } from "@/components/packages/versions-table";
import type { Package } from "@/types/api";

const base = {
  ecosystem: "pypi",
  name: "requests",
  checksum: "sha256:abc",
  cached_at: "2024-01-15T10:00:00Z",
  last_hit_at: null,
};

const packages: Package[] = [
  { ...base, id: 1, version: "2.31.0", size_bytes: 131072 },
  { ...base, id: 2, version: "2.28.0", size_bytes: 65536, last_hit_at: "2024-01-20T12:00:00Z" },
];

const tableProps = { total: 2, page: 1, pageSize: 25, basePath: "/packages/pypi/requests" };

describe("VersionsTable", () => {
  it("renders a row for every version", () => {
    render(<VersionsTable packages={packages} {...tableProps} />);

    // getAllByRole returns an array — if fewer or more elements are found the
    // test fails, so this doubles as a count assertion.
    const rows = screen.getAllByRole("link");
    expect(rows).toHaveLength(2);
  });

  it("renders all column headers", () => {
    render(<VersionsTable packages={packages} {...tableProps} />);

    expect(screen.getByText("Version")).toBeInTheDocument();
    expect(screen.getByText("Size")).toBeInTheDocument();
    expect(screen.getByText("Cached")).toBeInTheDocument();
    expect(screen.getByText("Last Hit")).toBeInTheDocument();
  });

  it("renders correct metadata for each version", () => {
    render(<VersionsTable packages={packages} {...tableProps} />);

    expect(screen.getByText("2.31.0")).toBeInTheDocument();
    expect(screen.getByText("128.0 KB")).toBeInTheDocument();

    expect(screen.getByText("2.28.0")).toBeInTheDocument();
    expect(screen.getByText("64.0 KB")).toBeInTheDocument();
  });

  it("each row links to the correct detail page", () => {
    render(<VersionsTable packages={packages} {...tableProps} />);

    const rows = screen.getAllByRole("link");

    // within() scopes all queries to just that element, so we can assert on
    // each row independently without the text of other rows interfering.
    expect(rows[0]).toHaveAttribute(
      "href",
      "/packages/pypi/requests/2.31.0"
    );
    expect(rows[1]).toHaveAttribute(
      "href",
      "/packages/pypi/requests/2.28.0"
    );
  });

  it("shows a dash when last_hit_at is null and a date when set", () => {
    render(<VersionsTable packages={packages} {...tableProps} />);

    const rows = screen.getAllByRole("link");

    // Version 2.31.0 has no last hit
    expect(within(rows[0]).getByText("—")).toBeInTheDocument();

    // Version 2.28.0 has a last hit date — check it is not a dash
    expect(within(rows[1]).queryByText("—")).not.toBeInTheDocument();
  });

  it("shows an empty state when there are no versions", () => {
    render(<VersionsTable packages={[]} {...tableProps} total={0} />);

    expect(screen.getByText("No versions found.")).toBeInTheDocument();

    // queryByRole returns null (not throws) when nothing matches — correct
    // tool for asserting absence.
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
