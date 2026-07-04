import { apiClient, type ApiTransport } from "./client";
import { fileServerClient, type FileEntry, type FileServerClient } from "./fileServer";
import { entityName, homeAssistantClient, type HomeAssistantClient, type HomeAssistantEntity } from "./homeAssistant";
import { arrayFrom } from "./normalize";

export type SearchResultType = "page" | "note" | "quick_link" | "app" | "member" | "file" | "homeassistant";

export type SearchResult = {
  type: SearchResultType | string;
  title: string;
  subtitle?: string;
  url: string;
  external?: boolean;
};

export type SearchResponse = {
  query: string;
  results: SearchResult[];
};

export class SearchClient {
  constructor(
    private readonly api: ApiTransport = apiClient,
    private readonly files: FileServerClient = fileServerClient,
    private readonly homeAssistant: HomeAssistantClient = homeAssistantClient,
  ) {}

  async search(query: string, signal?: AbortSignal): Promise<SearchResult[]> {
    const q = query.trim();
    if (!q) return [];
    const [cloudResults, agentResults] = await Promise.allSettled([
      this.searchCloud(q, signal),
      this.searchAgent(q),
    ]);
    return uniqueSearchResults([
      ...(cloudResults.status === "fulfilled" ? cloudResults.value : []),
      ...(agentResults.status === "fulfilled" ? agentResults.value : []),
    ]).slice(0, 24);
  }

  private async searchCloud(query: string, signal?: AbortSignal): Promise<SearchResult[]> {
    const payload = await this.api.request<SearchResponse>(
      `/v1/home/search?q=${encodeURIComponent(query)}`,
      { signal, timeoutMs: 8000 },
    );
    return arrayFrom<SearchResult>(payload?.results);
  }

  private async searchAgent(query: string): Promise<SearchResult[]> {
    const [fileResults, entities] = await Promise.allSettled([
      this.files.search(query),
      this.homeAssistant.fetchStates(),
    ]);
    return [
      ...(fileResults.status === "fulfilled" ? fileSearchResults(fileResults.value.items || fileResults.value.entries || []) : []),
      ...(entities.status === "fulfilled" ? homeAssistantSearchResults(entities.value, query) : []),
    ];
  }
}

export const searchClient = new SearchClient();

function fileSearchResults(items: FileEntry[]): SearchResult[] {
  return arrayFrom<FileEntry>(items).slice(0, 8).map((item) => {
    const title = fileName(item);
    const path = item.path || `/${title}`;
    const targetPath = item.is_directory ? path : parentPath(path);
    return {
      type: "file",
      title,
      subtitle: path,
      url: `/dashboard/file-server?path=${encodeURIComponent(targetPath)}`,
    };
  });
}

function homeAssistantSearchResults(entities: HomeAssistantEntity[], query: string): SearchResult[] {
  const needle = query.toLowerCase();
  return arrayFrom<HomeAssistantEntity>(entities)
    .filter((entity) => homeAssistantText(entity).includes(needle))
    .slice(0, 8)
    .map((entity) => ({
      type: "homeassistant",
      title: entityName(entity),
      subtitle: `${entity.entity_id} · ${entity.state}`,
      url: `/dashboard/home-assistant?query=${encodeURIComponent(entity.entity_id)}`,
    }));
}

function homeAssistantText(entity: HomeAssistantEntity): string {
  const attrs = entity.attributes || {};
  return [
    entity.entity_id,
    entity.state,
    attrs.friendly_name,
    attrs.device_class,
    attrs.area_id,
    attrs.unit_of_measurement,
  ].filter(Boolean).join(" ").toLowerCase();
}

function fileName(item: FileEntry): string {
  return item.name || item.path.split("/").filter(Boolean).pop() || "/";
}

function parentPath(path: string): string {
  const parts = path.split("/").filter(Boolean);
  parts.pop();
  return parts.length ? `/${parts.join("/")}` : "/";
}

function uniqueSearchResults(results: SearchResult[]): SearchResult[] {
  const seen = new Set<string>();
  const unique: SearchResult[] = [];
  for (const result of results) {
    const key = `${result.type}:${result.url}:${result.title}`;
    if (seen.has(key)) continue;
    seen.add(key);
    unique.push(result);
  }
  return unique;
}
