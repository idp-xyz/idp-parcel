import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  currentSearchKey,
  searchableThreshold,
  searchItemsOf,
  unknownLabelOf,
  usesSearchList,
  valueOfSearchKey,
} from './searchable-options';

const options = [
  { value: 'SYN-LE-01', label: '合成运营方一号 · SYN-LE-01 · 已生效' },
  { value: 'SYN-LE-02', label: '合成运营方二号 · SYN-LE-02 · 已生效' },
];

test('候选不超过阈值用原生下拉，超过才换成带搜索的选单', () => {
  equal(usesSearchList(0), false);
  equal(usesSearchList(searchableThreshold), false);
  equal(usesSearchList(searchableThreshold + 1), true);
});

test('条目：未选在最前，候选照序；搜索键同时含名字与标识，键唯一', () => {
  const items = searchItemsOf(options, 'SYN-LE-01');
  deepEqual(items.map((item) => item.value), ['', 'SYN-LE-01', 'SYN-LE-02']);
  equal(items[1].key.includes('合成运营方一号'), true);
  equal(items[1].key.includes('SYN-LE-01'), true);
  equal(new Set(items.map((item) => item.key)).size, items.length);
});

test('当前值是手填的、候选里没有：保留为一项，括注可选', () => {
  const items = searchItemsOf(options, 'MANUAL-1', '手填，不在读面上');
  deepEqual(items.map((item) => item.value), ['', 'MANUAL-1', 'SYN-LE-01', 'SYN-LE-02']);
  equal(items[1].label, 'MANUAL-1（手填，不在读面上）');
  equal(searchItemsOf(options, 'MANUAL-1')[1].label, 'MANUAL-1');
  equal(unknownLabelOf('X'), 'X');
  // 值是对象与版本合成的键时，调用方给整句显示，不把键露出来。
  equal(searchItemsOf(options, '["CC","v1"]', undefined, 'CC · v1（手填）')[1].label, 'CC · v1（手填）');
});

test('键与值互相找回：当前值、空值、不认识的键', () => {
  const items = searchItemsOf(options, 'SYN-LE-02');
  const key = currentSearchKey(items, 'SYN-LE-02');
  equal(valueOfSearchKey(items, key), 'SYN-LE-02');
  equal(currentSearchKey(items, ''), '未选');
  equal(valueOfSearchKey(items, '未选'), '');
  equal(valueOfSearchKey(items, '不存在的键'), '');
});
