import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TimeFilter } from "./time-filter";

const mockPush = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => new URLSearchParams(),
}));

beforeEach(() => mockPush.mockClear());

describe("TimeFilter", () => {
  it("renders all three window buttons", () => {
    render(<TimeFilter active="24h" />);
    expect(screen.getByRole("button", { name: "24h" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "1 week" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "YTD" })).toBeInTheDocument();
  });

  it("clicking 1 week sets ?since=168h", async () => {
    const user = userEvent.setup();
    render(<TimeFilter active="24h" />);
    await user.click(screen.getByRole("button", { name: "1 week" }));
    expect(mockPush).toHaveBeenCalledWith("?since=168h");
  });

  it("clicking YTD sets ?since=ytd", async () => {
    const user = userEvent.setup();
    render(<TimeFilter active="24h" />);
    await user.click(screen.getByRole("button", { name: "YTD" }));
    expect(mockPush).toHaveBeenCalledWith("?since=ytd");
  });

  it("clicking 24h removes the since param", async () => {
    jest.resetModules();
    jest.doMock("next/navigation", () => ({
      useRouter: () => ({ push: mockPush }),
      useSearchParams: () => new URLSearchParams("since=168h"),
    }));
    const { TimeFilter: Filter } = await import("./time-filter");

    const user = userEvent.setup();
    render(<Filter active="168h" />);
    await user.click(screen.getByRole("button", { name: "24h" }));
    expect(mockPush).toHaveBeenCalledWith("?");
  });

  it("preserves existing params when switching windows", async () => {
    jest.resetModules();
    jest.doMock("next/navigation", () => ({
      useRouter: () => ({ push: mockPush }),
      useSearchParams: () => new URLSearchParams("ecosystem=npm"),
    }));
    const { TimeFilter: Filter } = await import("./time-filter");

    const user = userEvent.setup();
    render(<Filter active="24h" />);
    await user.click(screen.getByRole("button", { name: "1 week" }));
    expect(mockPush).toHaveBeenCalledWith("?ecosystem=npm&since=168h");
  });

  it("highlights the active button", () => {
    render(<TimeFilter active="ytd" />);
    const ytdBtn = screen.getByRole("button", { name: "YTD" });
    expect(ytdBtn.className).toContain("bg-white");
  });
});
