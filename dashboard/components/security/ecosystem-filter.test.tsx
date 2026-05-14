import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EcosystemFilter, ECOSYSTEM_OPTIONS } from "./ecosystem-filter";

const mockPush = jest.fn();

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => new URLSearchParams(),
}));

beforeEach(() => mockPush.mockClear());

describe("EcosystemFilter", () => {
  it("renders all ecosystem options", () => {
    render(<EcosystemFilter active="All" />);
    const select = screen.getByRole("combobox");
    const options = Array.from((select as HTMLSelectElement).options).map((o) => o.value);
    expect(options).toEqual([...ECOSYSTEM_OPTIONS]);
  });

  it.each(["npm", "pypi", "maven", "go"] as const)(
    "selecting %s sets ?ecosystem=%s in the URL",
    async (eco) => {
      const user = userEvent.setup();
      render(<EcosystemFilter active="All" />);
      await user.selectOptions(screen.getByRole("combobox"), eco);
      expect(mockPush).toHaveBeenCalledWith(`?ecosystem=${eco}`);
    }
  );

  it("selecting All removes the ecosystem param", async () => {
    jest.resetModules();
    jest.doMock("next/navigation", () => ({
      useRouter: () => ({ push: mockPush }),
      useSearchParams: () => new URLSearchParams("ecosystem=npm"),
    }));
    const { EcosystemFilter: Filter } = await import("./ecosystem-filter");

    const user = userEvent.setup();
    render(<Filter active="npm" />);
    await user.selectOptions(screen.getByRole("combobox"), "All");
    expect(mockPush).toHaveBeenCalledWith("?");
  });

  it("preserves existing params when switching ecosystems", async () => {
    jest.resetModules();
    jest.doMock("next/navigation", () => ({
      useRouter: () => ({ push: mockPush }),
      useSearchParams: () => new URLSearchParams("since=168h"),
    }));
    const { EcosystemFilter: Filter } = await import("./ecosystem-filter");

    const user = userEvent.setup();
    render(<Filter active="All" />);
    await user.selectOptions(screen.getByRole("combobox"), "npm");
    expect(mockPush).toHaveBeenCalledWith("?since=168h&ecosystem=npm");
  });

  it("highlights the active option", () => {
    render(<EcosystemFilter active="pypi" />);
    expect((screen.getByRole("combobox") as HTMLSelectElement).value).toBe("pypi");
  });
});
