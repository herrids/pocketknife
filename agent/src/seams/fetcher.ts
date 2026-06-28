// AppSourceFetcher is the read seam symmetric with Submitter: given an app id,
// returns the app's current manifest and (when stored) its editable frontend
// source as a packed gzipped tar buffer. hasSource false signals a legacy app
// or a deploy that shipped no source; the caller regenerates the frontend from
// the manifest rather than editing fetched source.
//
// The implementation is selected by the same SUBMIT_MODE env var as Submitter
// (via seams/select.ts). The default ("stub") throws on any attempt to fetch,
// keeping local/offline flows unchanged.

export interface FetchResult {
  manifest: unknown;
  hasSource: boolean;
  sourceBuffer?: Buffer; // present only when hasSource is true
}

export interface AppSourceFetcher {
  fetch(appId: string): Promise<FetchResult>;
}

export class StubFetcher implements AppSourceFetcher {
  async fetch(_appId: string): Promise<FetchResult> {
    throw new Error(
      "StubFetcher.fetch: update mode requires SUBMIT_MODE=http and a running backend. " +
        "Set SUBMIT_MODE=http and GO_BASE_URL to the server's base URL.",
    );
  }
}

export class HttpFetcher implements AppSourceFetcher {
  constructor(private readonly baseUrl: string) {}

  async fetch(appId: string): Promise<FetchResult> {
    const url = `${this.baseUrl}/export/${encodeURIComponent(appId)}`;

    let res: Response;
    try {
      res = await fetch(url);
    } catch (err) {
      throw new Error(
        `export request to ${url} failed: ${err instanceof Error ? err.message : String(err)}`,
      );
    }

    if (!res.ok) {
      const body = await res.json().catch(() => undefined);
      const message = (body as { error?: { message?: string } } | undefined)?.error?.message;
      if (res.status === 404) {
        throw new Error(`app "${appId}" not found on backend (${url}): ${message ?? "not found"}`);
      }
      throw new Error(
        `export request to ${url} failed (${res.status}): ${message ?? "no error detail"}`,
      );
    }

    const data = (await res.json()) as { manifest: unknown; hasSource: boolean };
    if (!data.hasSource) {
      return { manifest: data.manifest, hasSource: false };
    }

    // Fetch the source tar from the companion endpoint.
    const sourceUrl = `${this.baseUrl}/export/${encodeURIComponent(appId)}/source`;
    let sourceRes: Response;
    try {
      sourceRes = await fetch(sourceUrl);
    } catch (err) {
      throw new Error(
        `source fetch from ${sourceUrl} failed: ${err instanceof Error ? err.message : String(err)}`,
      );
    }
    if (!sourceRes.ok) {
      throw new Error(`source fetch from ${sourceUrl} failed (${sourceRes.status})`);
    }

    const sourceBuffer = Buffer.from(await sourceRes.arrayBuffer());
    return { manifest: data.manifest, hasSource: true, sourceBuffer };
  }
}
