import { test } from 'node:test';
import { deepEqual, equal, notEqual } from 'node:assert/strict';
import { densityRowPadding, filterBarSlots, rowInteraction, rowKeyOpens } from './list-page-structure';

// 本文件钉的是列表页模板的结构位规则（票 admin-web-ux-alignment/03）。模板本体依赖 ESM-only 的 @idpxyz 原语，
// run-tests 的 CommonJS 发射加载不了它（票面完成记录有实测），要钉的规则抬到 list-page-structure.ts 用这里钉。

// Covers: 没接的位不出现。排序、保存视图、更多筛选接了才有，顺序不变。视图只有表格一种，不占位。
test('没接的过滤位不出现，接了的按排序、保存视图、更多筛选排列', () => {
  deepEqual(filterBarSlots({ sort: false, savedViews: false, moreFilters: false }), []);
  deepEqual(
    filterBarSlots({ sort: true, savedViews: true, moreFilters: true }).map((slot) => [slot.id, slot.enabled, slot.label]),
    [
      ['sort', true, '排序'],
      ['saved-view', true, '保存视图'],
      ['more-filters', true, '更多筛选'],
    ],
  );
  deepEqual(
    filterBarSlots({ sort: true, savedViews: false, moreFilters: false }).map((slot) => slot.id),
    ['sort'],
  );
});

// Covers: 两档密度给出两个不同的 Tailwind 纵向内边距类，且都是 py- 前缀——TableCell 原语自带 py-2，cn（tailwind-merge）
// 只在同一族类之间让位，换成别的前缀就叠加而不是替换，两档会长得一样。
test('密度两档映射到不同的 py- 类', () => {
  equal(densityRowPadding('compact'), 'py-1');
  equal(densityRowPadding('comfortable'), 'py-2.5');
  notEqual(densityRowPadding('compact'), densityRowPadding('comfortable'));
  for (const density of ['compact', 'comfortable'] as const) {
    equal(densityRowPadding(density).startsWith('py-'), true);
  }
});

// Covers: 没接 onRowOpen 的行不进 Tab 序——36 张页今天都没接，它们的行为必须逐字节同今天（票面「无 onRowOpen 时行为与今天同」）；
// 只接 onRowClick 仍是可点的行但不可聚焦，与今天一样。
test('无 onRowOpen 时行不可聚焦，是否可点只看 onRowClick', () => {
  deepEqual(rowInteraction({ click: false, open: false }), { tabIndex: undefined, clickable: false });
  deepEqual(rowInteraction({ click: true, open: false }), { tabIndex: undefined, clickable: true });
});

// Covers: 接了 onRowOpen 的行进 Tab 序（tabIndex 0，不用正数——正数会抢整页的 Tab 顺序），且不论有没有 onRowClick 都算可点：
// 双击本身就是一个指针动作。
test('有 onRowOpen 时行可聚焦且可点', () => {
  deepEqual(rowInteraction({ click: false, open: true }), { tabIndex: 0, clickable: true });
  deepEqual(rowInteraction({ click: true, open: true }), { tabIndex: 0, clickable: true });
});

// Covers: 接了 inspector 的行也进 Tab 序——键盘用户没有单击，要靠聚焦把行送进检查器。
test('有 inspector 时行可聚焦', () => {
  deepEqual(rowInteraction({ click: true, open: false, inspect: true }), { tabIndex: 0, clickable: true });
  deepEqual(rowInteraction({ click: true, open: false, inspect: false }), { tabIndex: undefined, clickable: true });
});

// Covers: 键盘上只有 Enter 等价双击（手册「可访问性」表格键盘导航）；Space 在滚动容器里是翻页键、方向键留给浏览器滚动，
// 都不能被行吞掉。
test('只有 Enter 等价双击', () => {
  equal(rowKeyOpens('Enter'), true);
  for (const key of [' ', 'Spacebar', 'ArrowDown', 'ArrowUp', 'Tab', 'Escape', 'a']) {
    equal(rowKeyOpens(key), false, key);
  }
});
