import { render, screen } from "@testing-library/react";
import { PackageRow } from "@/components/packages/package-row";
import type { PackageSummary } from "@/types/api";

const summary: PackageSummary = {
  ecosystem: "pypi",
  name: "requests",
  latest_version: "2.31.0",
  version_count: 4,
  total_size_bytes: 131072,
  last_cached_at: "2024-01-15T10:00:00Z",
  last_hit_at: "2024-01-20T12:00:00Z",
};

describe("PackageRow", () => {
  it("renders the package name and version", () => {
    render(<PackageRow summary={summary} />);

    expect(screen.getByText("requests")).toBeInTheDocument();
    expect(screen.getByText("2.31.0")).toBeInTheDocument();
  });

  it("renders the ecosystem badge", () => {
    render(<PackageRow summary={summary} />);

    expect(screen.getByText("pypi")).toBeInTheDocument();
  });

  it("links to the correct version list page", () => {
    render(<PackageRow summary={summary} />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/packages/pypi/requests");
  });

  it("renders the version count", () => {
    render(<PackageRow summary={summary} />);

    expect(screen.getByText("4")).toBeInTheDocument();
  });

  it("renders formatted total size", () => {
    render(<PackageRow summary={summary} />);

    // 131072 bytes = 128.0 KB
    expect(screen.getByText("128.0 KB")).toBeInTheDocument();
  });

  it("renders a dash when last_hit_at is null", () => {
    render(<PackageRow summary={{ ...summary, last_hit_at: null }} />);

    expect(screen.getByText("—")).toBeInTheDocument();
  });
});
