import { describe, expect, it, vi } from "vitest";
import { mutateThenRefresh } from "./mutationRefresh";

describe("mutation refresh orchestration", () => {
  it("returns the committed mutation result when refresh fails", async () => {
    const mutation = vi.fn().mockResolvedValue({ ok: true });
    const refresh = vi.fn().mockRejectedValue(new Error("temporary failure"));

    await expect(mutateThenRefresh(mutation, refresh)).resolves.toEqual({
      ok: true,
    });
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("still rejects when the mutation itself fails", async () => {
    const mutation = vi.fn().mockRejectedValue(new Error("mutation failed"));
    const refresh = vi.fn().mockResolvedValue(undefined);

    await expect(mutateThenRefresh(mutation, refresh)).rejects.toThrow(
      "mutation failed",
    );
    expect(refresh).not.toHaveBeenCalled();
  });
});
