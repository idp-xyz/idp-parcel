import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { completeRegisterRows } from './complete-register';

// 本文件钉「整册已到手」的判据：没有下一页、读口给了总数、到手的行数正等于总数，三样都成立才交出行；页面据此决定摆不摆计数、
// 派生维要不要标「只筛本页」。

const body = (rows: string[], page: { size: number; next: string | null; total: number | null }) => ({ rows, page });
const rowsOf = (answer: { rows: string[] }) => answer.rows;

// Covers: 一页装下整册——行原样交出，空册也算完整（零行、总数零）。
test('一页装下整册时交出行', () => {
  deepEqual(completeRegisterRows(body(['A', 'B'], { size: 200, next: null, total: 2 }), rowsOf), ['A', 'B']);
  deepEqual(completeRegisterRows(body([], { size: 200, next: null, total: 0 }), rowsOf), []);
});

// Covers: 不完整的几种——还有下一页；读口不给总数（不去猜）；末页却行数对不上总数（两次读之间册变了）；还没答出。
test('答不完整或还没答出时交 null', () => {
  equal(completeRegisterRows(body(['A', 'B'], { size: 2, next: 'c2', total: 3 }), rowsOf), null);
  equal(completeRegisterRows(body(['A', 'B'], { size: 200, next: null, total: null }), rowsOf), null);
  equal(completeRegisterRows(body(['A', 'B'], { size: 200, next: null, total: 3 }), rowsOf), null);
  equal(completeRegisterRows<ReturnType<typeof body>, string>(null, rowsOf), null);
});
