import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { resolveBreadcrumb } from './breadcrumb';
import { navigationSections, pageTitleById } from '../navigation';

// 本文件钉的是面包屑 `区 › 页` 的反查（票 admin-web-ux-alignment/03 第 1 条）：分区取条目所在分区的标题，
// 页名取 pageTitleById；查无出处的 id 不编分区。

// Covers: 平铺条目命中 → 分区标题 + 页名；页名优先取 pageTitles，缺则退回条目 label；嵌套 children 也能命中。
test('按模块 id 反查分区与页名', () => {
  const sections = [
    { title: '总览', items: [{ id: 'workbench', label: '工作台', icon: 'workbench' }] },
    {
      title: '主数据',
      items: [
        { id: 'a', label: '甲页', icon: 'a' },
        { id: 'b', label: '乙页', icon: 'b', children: [{ id: 'b-1', label: '乙一', icon: 'b' }] },
      ],
    },
  ];
  deepEqual(resolveBreadcrumb('a', sections, { a: '甲页（标题）' }), { section: '主数据', page: '甲页（标题）' });
  deepEqual(resolveBreadcrumb('a', sections, {}), { section: '主数据', page: '甲页' });
  deepEqual(resolveBreadcrumb('b-1', sections, {}), { section: '主数据', page: '乙一' });
});

// Covers: 不在任何分区里的 id 返回 null——面包屑宁可不渲染，也不为查无出处的 id 编一个分区。
test('查无出处的 id 返回 null', () => {
  equal(resolveBreadcrumb('nowhere', [{ title: '总览', items: [] }], { nowhere: '有标题也不算' }), null);
});

// Covers: 真实导航表里每个条目都能反查出面包屑，且页名与 pageTitleById 逐条一致——导航加一个条目、忘了登
// pageTitleById 时这里先响（Layout 的 moduleIdFromHash 也靠这张表认 id）。
test('真实导航表每个条目都能反查，页名与 pageTitleById 一致', () => {
  for (const section of navigationSections) {
    for (const item of section.items) {
      deepEqual(resolveBreadcrumb(item.id, navigationSections, pageTitleById), {
        section: section.title,
        page: pageTitleById[item.id],
      });
    }
  }
});
