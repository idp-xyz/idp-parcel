import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { filterBarSlots } from './list-page-structure';

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
