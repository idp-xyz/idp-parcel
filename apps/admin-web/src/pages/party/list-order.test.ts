import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { catalogueSortOf, codeFilterOptions, matchesSearch, sortableOr } from './list-order';

// 本文件钉的是参与方模块三份列表模块（法人 / 参与方身份 / 参与方关系）共用的筛选形状（票 admin-web-group-legal-
// entities/13 第 8 条统一）：筛选下拉的值是「全部」或词表里的一个码，码与词都从词表派生，任何列表模块不另抄一份封闭集。

// Covers: 选项表 = 「全部」在首位 + 词表的每个码沿登记顺序各一项，词就是词表里的词——词表扩一格，下拉自动多一项。
test('筛选选项从词表派生、「全部」在首位', () => {
  const table = { REGISTERED: '已登记', EFFECTIVE: '已生效' } as const;
  deepEqual(codeFilterOptions(table, '全部状态'), [
    { value: 'ALL', label: '全部状态' },
    { value: 'REGISTERED', label: '已登记' },
    { value: 'EFFECTIVE', label: '已生效' },
  ]);
  deepEqual(codeFilterOptions({}, '全部'), [{ value: 'ALL', label: '全部' }]);
});

// Covers: 搜索是包含匹配、不分大小写；空白搜索不筛；可选格按空串参与，名称未知的行不抛也不因此命中。
test('搜索包含匹配、空白不筛、可选格按空串参与', () => {
  deepEqual(matchesSearch('le-0', ['SYN-LE-01', undefined]), true);
  deepEqual(matchesSearch('   ', [undefined]), true);
  deepEqual(matchesSearch('x', [undefined, '']), false);
});

// Covers: 表头排序编成读口的 sort——只认对照里点了名的列，倒序带 `-`；编出来正是读口缺省序时不写（交 undefined），不在对照里的列
// 也交 undefined；原型链上的同名成员不被当成列。
test('catalogueSortOf 只下推读口点了名的维', () => {
  const columns = { 'effective-from': 'effectiveFrom', 'registered-at': 'registeredAt' };
  equal(catalogueSortOf({ key: 'effective-from', dir: 1 }, columns, '-registeredAt'), 'effectiveFrom');
  equal(catalogueSortOf({ key: 'effective-from', dir: -1 }, columns, '-registeredAt'), '-effectiveFrom');
  equal(catalogueSortOf({ key: 'registered-at', dir: -1 }, columns, '-registeredAt'), undefined);
  equal(catalogueSortOf({ key: 'registered-at', dir: -1 }, columns), '-registeredAt');
  equal(catalogueSortOf({ key: 'party-name', dir: 1 }, columns, '-registeredAt'), undefined);
  equal(catalogueSortOf({ key: 'toString', dir: 1 }, columns, '-registeredAt'), undefined);
});

// Covers: 记下的排序落在不可排的列上退回缺省序；可排的照旧。
test('sortableOr 不可排的列退回缺省序', () => {
  const columns = { 'registered-at': 'registeredAt' };
  const fallback = { key: 'registered-at', dir: -1 } as const;
  deepEqual(sortableOr({ key: 'party-name', dir: 1 }, columns, fallback), fallback);
  deepEqual(sortableOr({ key: 'registered-at', dir: 1 }, columns, fallback), { key: 'registered-at', dir: 1 });
});
