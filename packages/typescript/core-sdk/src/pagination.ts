/** The shape every cursor-paginated list endpoint in this platform
 * returns: {"items": [...], "nextCursor": "..."} - an empty/absent
 * nextCursor means there is no further page. */
export interface Page<T> {
  items: T[];
  nextCursor?: string;
}

export type PageFetcher<T> = (cursor: string | undefined) => Promise<Page<T>>;

/**
 * Async-iterates every item across every page from fetch - "pagination"
 * as a real SDK responsibility: a caller writes `for await (const x of
 * paginate(fetchPage))` instead of hand-rolling a cursor loop per list
 * endpoint they touch.
 */
export async function* paginate<T>(fetch: PageFetcher<T>): AsyncGenerator<T> {
  let cursor: string | undefined;
  for (;;) {
    const page = await fetch(cursor);
    for (const item of page.items) {
      yield item;
    }
    if (!page.nextCursor) return;
    cursor = page.nextCursor;
  }
}

/** Drains every page from fetch into a single array - use for small
 * lists where holding everything in memory is fine; paginate() is the
 * streaming alternative for large ones. */
export async function collectAll<T>(fetch: PageFetcher<T>): Promise<T[]> {
  const all: T[] = [];
  for await (const item of paginate(fetch)) {
    all.push(item);
  }
  return all;
}
