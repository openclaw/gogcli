import { describe, it, expect } from "vitest";
import { detectBot } from "./bot";

describe("detectBot", () => {
  it("treats GoogleImageProxy as real human", () => {
    const result = detectBot("GoogleImageProxy", "66.249.88.1", null);
    expect(result.isBot).toBe(false);
    expect(result.botType).toBe("gmail_proxy");
  });

  it("detects Apple Mail Privacy Protection", () => {
    const result = detectBot("Mozilla/5.0", "17.253.144.10", null);
    expect(result.isBot).toBe(true);
    expect(result.botType).toBe("apple_mpp");
  });

  it("detects Outlook prefetch", () => {
    const result = detectBot("Microsoft Outlook 16.0", "1.2.3.4", null);
    expect(result.isBot).toBe(true);
    expect(result.botType).toBe("outlook_prefetch");
  });

  it("detects rapid opens as prefetch", () => {
    const result = detectBot("Mozilla/5.0", "1.2.3.4", 500);
    expect(result.isBot).toBe(true);
    expect(result.botType).toBe("prefetch");
  });

  it("treats normal opens as human", () => {
    const result = detectBot("Mozilla/5.0 Chrome", "1.2.3.4", 5000);
    expect(result.isBot).toBe(false);
    expect(result.botType).toBeNull();
  });
});
