import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { legalEntityReadVersion, neighborsOf } from './legal-entity-read-cache';

// 钉右栏预读的两件判读：缓存键里的身份版本、方向键下一步会落到的邻行。缓存的存取与作废走真读口，由浏览器核验覆盖。

test('身份版本由登记记录号与状态拼成：更正或停用之后版本变，旧缓存不再命中', () => {
  equal(legalEntityReadVersion({ revision: 1, status: 'EFFECTIVE' }), '1:EFFECTIVE');
  equal(legalEntityReadVersion({ revision: 2, status: 'EFFECTIVE' }), '2:EFFECTIVE');
  equal(legalEntityReadVersion({ revision: 2, status: 'DEACTIVATED' }), '2:DEACTIVATED');
});

const rows = [{ id: 'A' }, { id: 'B' }, { id: 'C' }, { id: 'D' }];
const keyOf = (row: { id: string }) => row.id;

test('邻行取当前排序下的上一行与下一行；首尾只有一侧', () => {
  deepEqual(neighborsOf(rows, keyOf, 'B').map(keyOf), ['A', 'C']);
  deepEqual(neighborsOf(rows, keyOf, 'A').map(keyOf), ['B']);
  deepEqual(neighborsOf(rows, keyOf, 'D').map(keyOf), ['C']);
});

test('没选中、或选中的行已被筛掉时没有邻行，不猜', () => {
  deepEqual(neighborsOf(rows, keyOf, null), []);
  deepEqual(neighborsOf(rows, keyOf, 'Z'), []);
});
