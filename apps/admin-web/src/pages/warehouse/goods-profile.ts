// 商品档案页上要判一判再摆的几处（纯 TypeScript，node:test 直接测）：申报与仓测的尺寸重量按层级对上、某个商品的库存行、
// 代录新修订时从当前修订带出的格。

interface Dimensions {
  length: string;
  width: string;
  height: string;
  unit: string;
}

interface Weight {
  value: string;
  unit: string;
}

interface GoodsMeasurementSource {
  declaredMeasurements?: ReadonlyArray<{ level: string; dimensions?: Dimensions; weight?: Weight }>;
  warehouseMeasurements?: ReadonlyArray<{
    level: string;
    measured: boolean;
    current?: { warehouseCode: string; operator: string; occurredAt: string; dimensions: Dimensions; weight: Weight };
    /** 没仓测过的层级，读口附上货主声明的参照。 */
    declared?: { dimensions?: Dimensions; weight?: Weight };
  }>;
}

export interface MeasurementRow {
  level: string;
  /** 货主申报的尺寸与重量，没申报写空串。 */
  declared: string;
  measured: boolean;
  /** 仓里量过的当前值，没量过写空串。 */
  current: string;
  /** 这一版仓测是谁、在哪个仓、什么时候登记的。 */
  measuredBy?: { warehouseCode: string; operator: string; occurredAt: string };
}

/** 尺寸与重量连成一句；两样都没有答空串，不写零也不写默认值。 */
export function sizeWord(dimensions?: Dimensions, weight?: Weight): string {
  const parts: string[] = [];
  if (dimensions) parts.push(`${dimensions.length}×${dimensions.width}×${dimensions.height} ${dimensions.unit}`);
  if (weight) parts.push(`${weight.value} ${weight.unit}`);
  return parts.join(' · ');
}

/**
 * 申报与仓测按包装层级对在一行：先是申报里有的层级（保持申报的先后），再是只有仓测记录的层级。
 * 申报里没有这一层、仓测读口却附了货主声明的参照时，用参照顶上申报那一格——它们是同一份声明。
 */
export function goodsMeasurementRows(source: GoodsMeasurementSource): MeasurementRow[] {
  const rows = new Map<string, MeasurementRow>();
  for (const declared of source.declaredMeasurements ?? []) {
    rows.set(declared.level, {
      level: declared.level,
      declared: sizeWord(declared.dimensions, declared.weight),
      measured: false,
      current: '',
    });
  }
  for (const level of source.warehouseMeasurements ?? []) {
    const row = rows.get(level.level) ?? { level: level.level, declared: '', measured: false, current: '' };
    if (row.declared === '' && level.declared) row.declared = sizeWord(level.declared.dimensions, level.declared.weight);
    row.measured = level.measured;
    if (level.measured && level.current) {
      row.current = sizeWord(level.current.dimensions, level.current.weight);
      row.measuredBy = {
        warehouseCode: level.current.warehouseCode,
        operator: level.current.operator,
        occurredAt: level.current.occurredAt,
      };
    }
    rows.set(level.level, row);
  }
  return [...rows.values()];
}

/**
 * 库存汇总答的是这位货主名下全部商品；这个商品在各仓各状态的行，按商品编码逐字对。不用包含匹配：商品编码常是另一个编码的一段。
 */
export function goodsStockRows<Row extends { goodsCode: string }>(summary: readonly Row[] | undefined, goodsCode: string): Row[] {
  return (summary ?? []).filter((row) => row.goodsCode === goodsCode);
}

/** 代录新修订：货主与商品编码不动，基于当前修订号，名称、单位与两个跟踪开关从当前修订带出，人在表单上改。 */
export function revisionPrefill(goods: {
  ownerAccount: string;
  goodsCode: string;
  revision: number;
  name: string;
  baseUnit: string;
  trackBatch: boolean;
  trackExpiry: boolean;
}): Record<string, unknown> {
  return {
    ownerAccount: goods.ownerAccount,
    goodsCode: goods.goodsCode,
    basisRevision: String(goods.revision),
    name: goods.name,
    baseUnit: goods.baseUnit,
    trackBatch: goods.trackBatch,
    trackExpiry: goods.trackExpiry,
  };
}
