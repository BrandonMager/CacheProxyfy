/**
 * getByText with a regex is useful when you know part of the text but not the
 * full formatted string — e.g. a date that varies by locale or timezone.
 *
 * toBeInTheDocument() is a jest-dom custom matcher (from jest.setup.ts) that
 * asserts the element exists in the rendered DOM.
 *
 * getAllByText returns an array of all matching elements. Use it when the same
 * text appears in multiple places (e.g. a value shown in both a header and a
 * metadata row). This avoids the "Found multiple elements" error from getByText.
 */
import { render, screen } from "@testing-library/react";
import { PackageDetail } from "@/components/packages/package-detail";
import type { Package } from "@/types/api";

const pkg: Package = {
  id: 10,
  ecosystem: "pypi",
  name: "requests",
  version: "2.31.0",
  checksum: "sha256:abc123def456",
  size_bytes: 131072,
  cached_at: "2024-01-15T10:00:00Z",
  last_hit_at: "2024-01-20T12:00:00Z",
};

describe("PackageDetail", () => {
  it("renders the ecosystem badge", () => {
    render(<PackageDetail pkg={pkg} />);

    // "pypi" appears in both the badge (header) and the Ecosystem metadata row.
    // getAllByText returns an array — asserting length 2 confirms both locations
    // are present, which is more precise than just checking one exists.
    expect(screen.getAllByText("pypi")).toHaveLength(2);
  });

  it("renders the package name and version in the header", () => {
    render(<PackageDetail pkg={pkg} />);

    // Same pattern: "requests" and "2.31.0" each appear in the header and in
    // their respective metadata rows.
    expect(screen.getAllByText("requests")).toHaveLength(2);
    expect(screen.getAllByText("2.31.0")).toHaveLength(2);
  });

  it("renders all metadata field labels", () => {
    render(<PackageDetail pkg={pkg} />);

    expect(screen.getByText("Ecosystem")).toBeInTheDocument();
    expect(screen.getByText("Name")).toBeInTheDocument();
    expect(screen.getByText("Version")).toBeInTheDocument();
    expect(screen.getByText("Size")).toBeInTheDocument();
    expect(screen.getByText("Checksum")).toBeInTheDocument();
    expect(screen.getByText("Cached at")).toBeInTheDocument();
    expect(screen.getByText("Last hit")).toBeInTheDocument();
  });

  it("renders the checksum", () => {
    render(<PackageDetail pkg={pkg} />);

    expect(screen.getByText("sha256:abc123def456")).toBeInTheDocument();
  });

  it("renders the formatted size", () => {
    render(<PackageDetail pkg={pkg} />);

    // 131072 bytes = 128.0 KB
    expect(screen.getByText("128.0 KB")).toBeInTheDocument();
  });

  it("renders a dash for last hit when last_hit_at is null", () => {
    render(<PackageDetail pkg={{ ...pkg, last_hit_at: null }} />);

    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("renders a date string when last_hit_at is set", () => {
    render(<PackageDetail pkg={pkg} />);

    // When last_hit_at is populated, no dash should appear.
    expect(screen.queryByText("—")).not.toBeInTheDocument();
  });
});
