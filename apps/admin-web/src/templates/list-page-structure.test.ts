import { test } from 'node:test';
import { deepEqual, equal, notEqual } from 'node:assert/strict';
import { densityRowPadding, filterBarSlots, rowInteraction, rowKeyOpens } from './list-page-structure';

// 本文件钉的是列表页模板的结构位规则（票 admin-web-ux-alignment/03）。模板本体依赖 ESM-only 的 @idpxyz 原语，
// run-tests 的 CommonJS 发射加载不了它（票面完成记录有实测），要钉的规则抬到 list-page-structure.ts 用这里钉。

// Covers: 四个位一个不少、顺序照黄金标准 9.2（排序 → 视图 → 保存视图 → 更多筛选）；全都不传时四个全禁用，
// 每个禁用位都带说明——「留位不留假动作」。
test('Filter Bar 四个位顺序固定，不传时全禁用且各有说明', () => {
  const slots = filterBarSlots({ sort: false, savedViews: false, moreFilters: false });
  deepEqual(
    slots.map((slot) => slot.id),
    ['sort', 'view-mode', 'saved-view', 'more-filters'],
  );
  for (const slot of slots) {
    equal(slot.enabled, false, slot.id);
    equal(typeof slot.disabledReason, 'string', slot.id);
    equal(slot.disabledReason !== '', true, slot.id);
  }
  equal(slots[0].disabledReason, '本页尚未提供排序');
  equal(slots[1].disabledReason, '本页只有表格视图');
});

// Covers: 接了哪个位哪个位就启用、说明消失，其它位不受影响；视图模式位不管接什么都禁用（只有表格视图）；
// 启用与禁用用同一个词，位置和文案不因功能未接而变。
test('接了的位启用、未接的照旧禁用，视图模式永远禁用', () => {
  const all = filterBarSlots({ sort: true, savedViews: true, moreFilters: true });
  deepEqual(
    all.map((slot) => [slot.id, slot.enabled, slot.disabledReason]),
    [
      ['sort', true, undefined],
      ['view-mode', false, '本页只有表格视图'],
      ['saved-view', true, undefined],
      ['more-filters', true, undefined],
    ],
  );
  const none = filterBarSlots({ sort: false, savedViews: false, moreFilters: false });
  deepEqual(
    all.map((slot) => slot.label),
    none.map((slot) => slot.label),
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

// Covers: 键盘上只有 Enter 等价双击（手册「可访问性」表格键盘导航）；Space 在滚动容器里是翻页键、方向键留给浏览器滚动，
// 都不能被行吞掉。
test('只有 Enter 等价双击', () => {
  equal(rowKeyOpens('Enter'), true);
  for (const key of [' ', 'Spacebar', 'ArrowDown', 'ArrowUp', 'Tab', 'Escape', 'a']) {
    equal(rowKeyOpens(key), false, key);
  }
});
