import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { placeLabel, placeNameIndexOf, placePairLabel } from './place-names';

const now = new Date('2026-11-02T04:00:00Z');

// Covers: 只取此刻生效那一版登的名称——旧版的名称不顶到新版，新版没登就没有名称；没有名称或查不到显编码。
test('按编码取此刻生效那一版的名称', () => {
  const index = placeNameIndexOf(
    [
      { code: 'SYN-NODE-SHA-HUB', from: '2026-01-01T00:00:00Z', to: '2026-11-02T00:00:00Z' },
      { code: 'SYN-NODE-SHA-HUB', displayName: '上海枢纽', from: '2026-11-02T00:00:00Z' },
      { code: 'SYN-NODE-OLD', displayName: '旧名', from: '2026-01-01T00:00:00Z', to: '2026-06-01T00:00:00Z' },
      { code: 'SYN-NODE-OLD', from: '2026-06-01T00:00:00Z' },
      { code: 'SYN-NODE-LATER', displayName: '还没生效', from: '2026-12-01T00:00:00Z' },
    ],
    now,
  );
  equal(placeLabel(index, 'SYN-NODE-SHA-HUB'), '上海枢纽');
  equal(placeLabel(index, 'SYN-NODE-OLD'), 'SYN-NODE-OLD');
  equal(placeLabel(index, 'SYN-NODE-LATER'), 'SYN-NODE-LATER');
  equal(placeLabel(index, 'SYN-NODE-UNKNOWN'), 'SYN-NODE-UNKNOWN');
  equal(placeLabel(index, undefined), '—');
  equal(placePairLabel(index, 'SYN-NODE-SHA-HUB', 'SYN-NODE-UNKNOWN'), '上海枢纽 → SYN-NODE-UNKNOWN');
});
