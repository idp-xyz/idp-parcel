import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { catalogueRequestPath } from '../catalogue-query';
import { seriesConditions } from './catalogue-filter';

// 本文件钉的是「只看这条序列」与「搜索词」是两种不同的收窄，不得折成一种：前者落到 seriesId 与 kind 两个精确筛选维，后者落到
// q 的包含匹配。折成一种（把只看做成往搜索框里填标识）会让前缀同名的兄弟序列混进来，而看的人以为自己只看着一条。

const path = '/pricing-reference-series';

// Covers: 只看按（标识 + 种类）精确取——标识不进 q，种类一并带上，同标识另一种类也不混进来。
test('只看一条序列落到标识与种类两个筛选维，不进 q', () => {
  equal(
    catalogueRequestPath(path, seriesConditions({ focus: { seriesId: 'SYN-PRC-FUEL', kind: 'FUEL_RATE' }, keyword: '' }), null),
    '/pricing-reference-series?kind=FUEL_RATE&seriesId=SYN-PRC-FUEL',
  );
});

// Covers: 不只看时不带筛选维——null 是「没选」，不是「选了空串」。
test('没有只看也没有搜索词时是裸路径', () => {
  equal(catalogueRequestPath(path, seriesConditions({ focus: null, keyword: '' }), null), path);
  equal(catalogueRequestPath(path, seriesConditions({ focus: null, keyword: '   ' }), null), path);
});

test('搜索词落到 q', () => {
  equal(catalogueRequestPath(path, seriesConditions({ focus: null, keyword: ' ops-b ' }), null), `${path}?q=ops-b`);
});

// Covers: 两种收窄叠加，筛选维与 q 之间为且——在这条序列里还能按责任方再筛，而不是二选一。
test('只看与搜索词叠加而不是二选一', () => {
  equal(
    catalogueRequestPath(path, seriesConditions({ focus: { seriesId: 'SYN-PRC-FUEL', kind: 'FUEL_RATE' }, keyword: 'ops-b' }), 'c2'),
    '/pricing-reference-series?kind=FUEL_RATE&seriesId=SYN-PRC-FUEL&q=ops-b&after=c2',
  );
});
