// 加载态「按形状出骨架」的纯逻辑（票 admin-web-ux-alignment/06 第 1 条）。形状是 viewState 的属性，由给出状态的一方说
// （目录页在 catalogueViewState 里默认表；模板不猜）；这里只把形状映到骨架件的种类，摆在 state-slot.tsx。
//
// 为什么单独一个 .ts：组件层在 node:test 里钉不到——@idpxyz/ui-patterns 与 ui-primitives 的 exports 只有 import 条件、没有
// require / default，tsconfig.test.json 发射的 CommonJS 一 require 就 ERR_PACKAGE_PATH_NOT_EXPORTED，Node 22 的 require(esm) 也
// 过不了 exports 这一关。要钉的规则（每种形状对应哪一件、不传形状仍是今天的通用脉冲块）抬到这里，组件那半只剩摆。

/**
 * 骨架的形状（手册「Loading」：骨架长得像即将出现的内容）。`block` 是今天的通用脉冲块——不传即它，既有调用方零改动、
 * 看到的也不变。
 */
export type LoadingShape = 'table' | 'list' | 'detail' | 'block';

/** 骨架件的种类：表格（行列）、事件列表、头区加卡片、通用脉冲块。 */
export type SkeletonKind = 'table' | 'event-list' | 'header-and-cards' | 'pulse-block';

/** 形状 → 骨架件。默认（不传）与 `block` 都是通用脉冲块，这是向后兼容的那一格。 */
export function skeletonOf(shape: LoadingShape | undefined): SkeletonKind {
  switch (shape) {
    case 'table':
      return 'table';
    case 'list':
      return 'event-list';
    case 'detail':
      return 'header-and-cards';
    default:
      return 'pulse-block';
  }
}
