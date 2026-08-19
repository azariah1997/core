/// The shape every cursor-paginated list endpoint in this platform
/// returns: {"items": [...], "nextCursor": "..."} - an empty/null
/// nextCursor means there is no further page.
class Page<T> {
  final List<T> items;
  final String? nextCursor;
  const Page(this.items, this.nextCursor);
}

typedef PageFetcher<T> = Future<Page<T>> Function(String? cursor);

/// Streams every item across every page from fetch - "pagination" as a
/// real SDK responsibility: a caller writes `await for (final x in
/// paginate(fetchPage))` instead of hand-rolling a cursor loop per list
/// endpoint they touch.
Stream<T> paginate<T>(PageFetcher<T> fetch) async* {
  String? cursor;
  for (;;) {
    final page = await fetch(cursor);
    for (final item in page.items) {
      yield item;
    }
    if (page.nextCursor == null || page.nextCursor!.isEmpty) return;
    cursor = page.nextCursor;
  }
}

/// Drains every page from fetch into a single list - use for small
/// lists where holding everything in memory is fine; paginate() is the
/// streaming alternative for large ones.
Future<List<T>> collectAll<T>(PageFetcher<T> fetch) => paginate(fetch).toList();
