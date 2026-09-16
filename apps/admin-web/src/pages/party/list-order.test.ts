import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { codeFilterOptions, matchesSearch } from './list-order';

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
