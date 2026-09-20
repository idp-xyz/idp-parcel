import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { skeletonOf, type LoadingShape } from './loading-shape';

// 本文件钉的是加载态形状 → 骨架件的映射（票 admin-web-ux-alignment/06 第 1 条）：四种形状各有其件、不传形状仍是
// 今天的通用脉冲块。组件那半（StateSlot 真渲染出 table / 事件列表）在 node:test 里钉不到，理由见 loading-shape.ts 文件头。

// Covers: 四种形状全覆盖——表格出表格骨架、列表出事件列表骨架、详情出头区加卡片、block 是通用脉冲块。
test('形状映到对应的骨架件', () => {
  const expected: Record<LoadingShape, ReturnType<typeof skeletonOf>> = {
    table: 'table',
    list: 'event-list',
    detail: 'header-and-cards',
    block: 'pulse-block',
  };
  for (const [shape, kind] of Object.entries(expected) as [LoadingShape, ReturnType<typeof skeletonOf>][]) {
    equal(skeletonOf(shape), kind);
  }
});

// Covers: 不传形状 = block——既有 `{ kind: 'loading' }` 的调用方看到的仍是通用脉冲块，向后兼容。
test('不传形状即通用脉冲块', () => {
  equal(skeletonOf(undefined), 'pulse-block');
  equal(skeletonOf(undefined), skeletonOf('block'));
});
