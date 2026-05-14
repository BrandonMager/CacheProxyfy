import { resolveEcosystem, resolveTimeWindow, resolveSince, ytdHours } from "./page";

describe("resolveEcosystem", () => {
  it.each(["npm", "pypi", "maven", "go"] as const)(
    "accepts valid ecosystem %s",
    (eco) => {
      expect(resolveEcosystem(eco)).toBe(eco);
    }
  );

  it.each(["invalid", "CRITICAL", "all", "NPM", "", "go "])(
    "rejects %j and falls back to All",
    (raw) => {
      expect(resolveEcosystem(raw)).toBe("All");
    }
  );

  it("treats undefined as All", () => {
    expect(resolveEcosystem(undefined)).toBe("All");
  });
});

describe("resolveTimeWindow", () => {
  it("accepts 168h", () => expect(resolveTimeWindow("168h")).toBe("168h"));
  it("accepts ytd",  () => expect(resolveTimeWindow("ytd")).toBe("ytd"));

  it.each(["24h", "bad", "", undefined])(
    "falls back to 24h for %j",
    (raw) => expect(resolveTimeWindow(raw)).toBe("24h")
  );
});

describe("resolveSince", () => {
  it("passes 24h through unchanged", () => {
    expect(resolveSince("24h")).toBe("24h");
  });

  it("passes 168h through unchanged", () => {
    expect(resolveSince("168h")).toBe("168h");
  });

  it("converts ytd to an hour count, never the literal string", () => {
    const result = resolveSince("ytd");
    expect(result).not.toBe("ytd");
    expect(result).toMatch(/^\d+h$/);
  });
});

describe("combined filter resolution", () => {
  it("resolves ecosystem and since independently — both passed to the API", () => {
    expect(resolveEcosystem("npm")).toBe("npm");
    expect(resolveSince(resolveTimeWindow("168h"))).toBe("168h");
  });

  it("ytd + ecosystem both resolve without interfering", () => {
    const since = resolveSince(resolveTimeWindow("ytd"));
    const ecosystem = resolveEcosystem("go");
    expect(since).toMatch(/^\d+h$/);
    expect(ecosystem).toBe("go");
  });

  it("invalid ecosystem falls back to All while time window resolves normally", () => {
    expect(resolveEcosystem("bad")).toBe("All");
    expect(resolveSince(resolveTimeWindow("168h"))).toBe("168h");
  });
});

describe("ytdHours", () => {
  it("returns a positive hour count", () => {
    const result = ytdHours();
    expect(result).toMatch(/^\d+h$/);
    const hours = parseInt(result, 10);
    expect(hours).toBeGreaterThan(0);
  });

  it("is at most 8784 hours (longest possible year)", () => {
    const hours = parseInt(ytdHours(), 10);
    expect(hours).toBeLessThanOrEqual(8784);
  });

  it("reflects elapsed days since Jan 1 of the current year", () => {
    const now = new Date();
    const startOfYear = new Date(now.getFullYear(), 0, 1);
    const expectedHours = Math.ceil((now.getTime() - startOfYear.getTime()) / 36e5);
    expect(parseInt(ytdHours(), 10)).toBe(expectedHours);
  });
});
