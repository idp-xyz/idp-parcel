// 入库预报的收货进度：预报了多少、收了多少、还差多少（纯 TypeScript，node:test 直接测）。
//
// 预报读口分开答「预报的行」与「累计实收」。按商品入库的两边都按商品编码对；同一商品在预报里可以分几行（不同批号），
// 累计实收只按商品答，所以进度按商品合计，不逐行摊。按整箱入库的按箱号对，一箱要么收了要么没收。

export interface PreAdviceProgressRow {
  /** 商品编码或箱号。 */
  key: string;
  /** 商品的单位，或这一箱里装了什么。 */
  detail: string;
  advised: number;
  received: number;
}

export interface PreAdviceProgress {
  byCarton: boolean;
  rows: PreAdviceProgressRow[];
  advised: number;
  received: number;
  /** 预报了还没收到的合计；多收不抵少收。 */
  outstanding: number;
}

interface ProgressSource {
  inboundMode: string;
  goodsLines?: ReadonlyArray<{ goodsCode: string; unit: string; quantity: number }>;
  cartons?: ReadonlyArray<{ cartonNumber: string; lines: ReadonlyArray<{ goodsCode: string; quantity: number }> }>;
  cumulativeReceived?: ReadonlyArray<{ goodsCode?: string; cartonNumber?: string; quantity: number }>;
}

export function preAdviceProgress(source: ProgressSource): PreAdviceProgress {
  const byCarton = source.inboundMode === 'BY_CARTON';
  const rows = new Map<string, PreAdviceProgressRow>();
  const row = (key: string, detail: string): PreAdviceProgressRow => {
    const existing = rows.get(key);
    if (existing) return existing;
    const created = { key, detail, advised: 0, received: 0 };
    rows.set(key, created);
    return created;
  };

  if (byCarton) {
    for (const carton of source.cartons ?? []) {
      row(carton.cartonNumber, carton.lines.map((line) => `${line.goodsCode}×${line.quantity}`).join('、')).advised += 1;
    }
  } else {
    for (const line of source.goodsLines ?? []) row(line.goodsCode, line.unit).advised += line.quantity;
  }
  // 实收里有而预报里没有的也列出来（预报 0）：那是多出来的货，不该从进度里消失。
  for (const item of source.cumulativeReceived ?? []) {
    const key = byCarton ? item.cartonNumber : item.goodsCode;
    if (!key) continue;
    row(key, '').received += item.quantity;
  }

  const list = [...rows.values()];
  return {
    byCarton,
    rows: list,
    advised: list.reduce((sum, item) => sum + item.advised, 0),
    received: list.reduce((sum, item) => sum + item.received, 0),
    outstanding: list.reduce((sum, item) => sum + Math.max(0, item.advised - item.received), 0),
  };
}
