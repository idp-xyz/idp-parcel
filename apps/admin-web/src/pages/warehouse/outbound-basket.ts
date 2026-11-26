// 出库单列表里勾选的那一篮（纯 TypeScript，node:test 直接测）。
//
// 勾选在这一页只为一件事：挑几张单合成一个波次。常见的做法是搜一个单号勾上、再搜下一个勾上，所以勾选不跟着眼前显示的
// 内容走——切队、检索、换筛选、翻页都留着，只在人点「取消选择」或合并放波次放成了之后清。篮子里记的是行（键 → 行）：
// 勾上的单翻走之后页面手里已经没有它，而「是不是同一个仓、是不是都已预占」要照行来判，「只看已选」也要拿行来摆。
// 往篮子里加减行用 templates/list-selection 的 retainSelectedRows，这里只补它没有的两件事。

/**
 * 眼前这一页重取之后，把篮子里也在这一页上的行换成新取回的：行的状态可能已经变了（别处放了波次），合并放波次能不能按
 * 要照新的判。没有哪一行换了就交回原来那一份——调用方把它当 React 状态，交回新对象会白白重渲一轮。
 */
export function refreshedBasket<Row>(
  basket: ReadonlyMap<string, Row>,
  visible: readonly Row[],
  keyOf: (row: Row) => string,
): ReadonlyMap<string, Row> {
  let next: Map<string, Row> | null = null;
  for (const row of visible) {
    const key = keyOf(row);
    if (!basket.has(key) || basket.get(key) === row) continue;
    next ??= new Map(basket);
    next.set(key, row);
  }
  return next ?? basket;
}

/** 篮子里有几张不在眼前这一页上：批量栏要说清，免得人对着看不见的单按下去而不自知。 */
export function hiddenInBasket<Row>(basket: ReadonlyMap<string, Row>, visible: readonly Row[], keyOf: (row: Row) => string): number {
  const shown = new Set(visible.map(keyOf));
  let hidden = 0;
  for (const key of basket.keys()) if (!shown.has(key)) hidden += 1;
  return hidden;
}

/**
 * 篮子里的单此刻能不能合成一个波次；不能时说为什么。只能照页面手里的行判——勾上之后别处改了状态的单，要等它再出现在
 * 眼前的一页上才知道，放波次的口那时会拒。
 */
export function mergeWaveBlocker(records: ReadonlyArray<{ warehouseCode: string; status: string }>): string | null {
  if (records.length === 0) return '还没有勾选出库单';
  if (new Set(records.map((record) => record.warehouseCode)).size > 1) return '勾选的单不在同一个仓，一个波次只放一个仓的单';
  if (records.some((record) => record.status !== 'RESERVED')) return '勾选的单里有不是已预占的，只有已预占的单能放波次';
  return null;
}

/**
 * 放波次的答复是不是「放成了」：新放成的，或同一个波次号此前已经放成过的。别的答复（状态不符、内容冲突、未配置……）
 * 篮子留着——人改完再放时不必重新勾一遍。
 */
export function waveReleaseTookEffect(outcome: string): boolean {
  return outcome === 'ALLOCATED' || outcome === 'EXISTING_RESULT';
}
