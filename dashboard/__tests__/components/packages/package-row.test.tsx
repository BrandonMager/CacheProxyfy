/**
 * React Testing Library works by rendering your component into a real (jsdom)
 * DOM and letting you query it the same way a user would — by visible text,
 * roles, and labels rather than implementation details like class names or
 * component state.
 *
 * Core API used here:
 *   render()   – mounts the component and returns query helpers
 *   screen     – a global object with the same query helpers (preferred style)
 *   getByRole  – finds an element by its ARIA role; throws if nothing matches
 *   getByText  – finds an element whose text content matches
 */
import { render, screen } from "@testing-library/react";
import { PackageRow } from "@/components/packages/package-row";
import type { Package } from "@/types/api";

const pkg: Package = {
  id: 1,
  ecosystem: "pypi",
  name: "requests",
  version: "2.31.0",
  checksum: "sha256:abc123",
  size_bytes: 131072,
  cached_at: "2024-01-15T10:00:00Z",
  last_hit_at: "2024-01-20T12:00:00Z",
};

describe("PackageRow", () => {
  it("renders the package name and version", () => {
    render(<PackageRow pkg={pkg} />);

    // getByText finds any element whose text content matches.
    // If the element isn't found, the test fails with a helpful message.
    expect(screen.getByText("requests")).toBeInTheDocument();
    expect(screen.getByText("2.31.0")).toBeInTheDocument();
  });

  it("renders the ecosystem badge", () => {
    render(<PackageRow pkg={pkg} />);

    expect(screen.getByText("pypi")).toBeInTheDocument();
  });

  it("links to the correct version list page", () => {
    render(<PackageRow pkg={pkg} />);

    // getByRole finds the element by its semantic ARIA role.
    // A <Link> renders as an <a> tag, which has role="link".
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/packages/pypi/requests");
  });

  it("renders formatted size", () => {
    render(<PackageRow pkg={pkg} />);

    // 131072 bytes = 128.0 KB
    expect(screen.getByText("128.0 KB")).toBeInTheDocument();
  });

  it("renders a dash when last_hit_at is null", () => {
    render(<PackageRow pkg={{ ...pkg, last_hit_at: null }} />);

    expect(screen.getByText("—")).toBeInTheDocument();
  });
});
