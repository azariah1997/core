import 'package:test/test.dart';
import 'package:core_sdk/core_sdk.dart';

void main() {
  test('paginate walks every page in order', () async {
    final pages = <String, Page<int>>{
      '': Page([1, 2], 'p2'),
      'p2': Page([3, 4], 'p3'),
      'p3': Page([5], null),
    };
    Future<Page<int>> fetch(String? cursor) async => pages[cursor ?? '']!;

    final got = await paginate(fetch).toList();
    expect(got, [1, 2, 3, 4, 5]);
  });

  test('collectAll stops at a null cursor', () async {
    var calls = 0;
    Future<Page<String>> fetch(String? cursor) async {
      calls++;
      return const Page(['only'], null);
    }

    final items = await collectAll(fetch);
    expect(items, ['only']);
    expect(calls, 1);
  });
}
