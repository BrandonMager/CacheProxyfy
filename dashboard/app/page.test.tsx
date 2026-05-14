import { render, screen } from "@testing-library/react";
import Home from "./page";
import { getStats, listPackages, listCVEAlerts } from "../lib/api";

jest.mock("../lib/api");
jest.mock("../components/layout/sidebar-layout", () => ({
  SidebarLayout: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const mockGetStats     = getStats     as jest.Mock;
const mockListPackages = listPackages as jest.Mock;

const baseStats = {
  total_packages: 10,
  blocked_packages: 1,
  total_hits: 8,
  total_misses: 2,
  bytes_saved: 1024,
  hit_rate: 0.8,
  cve_alerts: 0,
};

beforeEach(() => {
  jest.clearAllMocks();
  mockListPackages.mockResolvedValue([]);
});

const STAT_LABELS = [
  "Packages Cached",
  "Blocked",
  "Cache Hit Rate",
  "Bandwidth Saved",
  "CVE Alerts",
];

describe("API calls", () => {
  it("makes exactly 2 calls — /api/stats and /api/packages/list — and never /api/cve-alerts", async () => {
    mockGetStats.mockResolvedValue(baseStats);
    await Home();
    expect(getStats).toHaveBeenCalledTimes(1);
    expect(listPackages).toHaveBeenCalledTimes(1);
    expect(listCVEAlerts).not.toHaveBeenCalled();
  });
});

describe("API failure", () => {
  it("renders without crashing and shows — on every stat card", async () => {
    mockGetStats.mockRejectedValue(new Error("backend down"));
    render(await Home()); // throws → test fails, satisfying "does not crash"

    for (const label of STAT_LABELS) {
      const heading = screen.getByText(label);
      expect(heading.nextElementSibling).toHaveTextContent("—");
    }
  });
});

describe("CVE Alerts stat card", () => {
  it("shows the cve_alerts count when the backend is reachable", async () => {
    mockGetStats.mockResolvedValue({ ...baseStats, cve_alerts: 7 });
    render(await Home());
    const label = screen.getByText("CVE Alerts");
    expect(label.nextElementSibling).toHaveTextContent("7");
  });

  it("shows — when the backend is unreachable", async () => {
    mockGetStats.mockRejectedValue(new Error("network error"));
    render(await Home());
    const label = screen.getByText("CVE Alerts");
    expect(label.nextElementSibling).toHaveTextContent("—");
  });

  it("reads from stats.cve_alerts — listCVEAlerts is never called", async () => {
    mockGetStats.mockResolvedValue({ ...baseStats, cve_alerts: 13 });
    render(await Home());
    expect(listCVEAlerts).not.toHaveBeenCalled();
    const label = screen.getByText("CVE Alerts");
    expect(label.nextElementSibling).toHaveTextContent("13");
  });
});
