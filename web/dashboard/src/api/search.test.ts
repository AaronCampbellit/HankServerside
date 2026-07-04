import { describe, expect, it, vi } from "vitest";
import { SearchClient } from "./search";
import type { ApiTransport } from "./client";
import type { FileServerClient } from "./fileServer";
import type { HomeAssistantClient } from "./homeAssistant";

describe("SearchClient", () => {
  it("merges cloud, file, and Home Assistant search results", async () => {
    const request = vi.fn(async <T>() => ({
      query: "kitchen",
      results: [{ type: "page", title: "Kitchen Notes", url: "/dashboard/profile-notes" }],
    }) as T);
    const files = {
      search: vi.fn(async () => ({
        items: [{ name: "kitchen-plan.pdf", path: "/Docs/kitchen-plan.pdf", is_directory: false }],
      })),
    };
    const homeAssistant = {
      fetchStates: vi.fn(async () => [
        { entity_id: "light.kitchen", state: "on", attributes: { friendly_name: "Kitchen Light" } },
        { entity_id: "sensor.garage", state: "closed", attributes: { friendly_name: "Garage Door" } },
      ]),
    };
    const client = new SearchClient(
      { request: request as unknown as ApiTransport["request"] },
      files as unknown as FileServerClient,
      homeAssistant as unknown as HomeAssistantClient,
    );

    const results = await client.search("kitchen");

    expect(request).toHaveBeenCalledWith("/v1/home/search?q=kitchen", { signal: undefined, timeoutMs: 8000 });
    expect(files.search).toHaveBeenCalledWith("kitchen");
    expect(homeAssistant.fetchStates).toHaveBeenCalled();
    expect(results.map((result) => result.title)).toEqual([
      "Kitchen Notes",
      "kitchen-plan.pdf",
      "Kitchen Light",
    ]);
    expect(results[1]).toMatchObject({
      type: "file",
      subtitle: "/Docs/kitchen-plan.pdf",
      url: "/dashboard/file-server?path=%2FDocs",
    });
    expect(results[2]).toMatchObject({
      type: "homeassistant",
      subtitle: "light.kitchen · on",
      url: "/dashboard/home-assistant?query=light.kitchen",
    });
  });
});
