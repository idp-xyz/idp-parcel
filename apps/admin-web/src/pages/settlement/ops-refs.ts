// 账务区把系统引用翻成运营看得懂的字的几个纯函数（管理台改版第一步，设计原则一「以运单为中心」）。抽成纯函数是为了
// 让「哪一格翻得出、翻不出时显什么」能在 node:test 里钉住；页面只管取数与摆放。

/** 包裹号在各种引用里的形：`DPCL-` 加 Base32 大写字母数字。 */
const parcelPattern = /\bDPCL-[A-Z0-9]+\b/;

/**
 * 从一条引用里认出包裹号。费用的来源事实与确认依据都长成 `<族>/<包裹号>/<事件>/<版本>`，关务的对象范围直接就是包裹号；
 * 认不出答 undefined，由调用方决定显什么——不猜、不截别的段充数。
 */
export function parcelFromRef(ref: string | undefined): string | undefined {
  if (!ref) return undefined;
  return parcelPattern.exec(ref)?.[0];
}

/**
 * 费用来源事实的「族」的中文：引用首段说这笔费用从哪类事实算出来。只翻族名，后面的事件与版本留在技术详情。
 * 词取各上下文 CONTEXT 的原词（网点收寄、签收终局、运输交接）。
 */
export const sourceFactFamilyLabels: Record<string, string> = {
  'network-intake': '网点收寄',
  'final-outcome': '签收终局',
  'transport-handover': '运输交接',
  'route-plan': '路由计划',
};

/** 包裹号认不出来时，用费用来源的中文说法顶上（如「网点收寄」）；族也认不出就原样回显整条引用。 */
export function sourceFactLabel(ref: string | undefined): string | undefined {
  if (!ref) return undefined;
  const family = ref.split('/')[0] ?? ref;
  return sourceFactFamilyLabels[family] ?? ref;
}

/**
 * 费用项目码 → 中文。费用项目码本身是租户取值（演示租户的形是 `SYN-FEE-ITEM-<种类>-<序号>`），这里按码里的种类段翻，
 * 不写死任何租户的整码；认不出种类段的码原样上屏，并由调用方记下（codeLabel 会提醒一次）。费用项目目录上线后这张表
 * 应换成读那本册的名称。
 */
export const feeItemKindLabels: Record<string, string> = {
  FREIGHT: '运费',
  'TRUNK-COST': '干线成本',
  'LAST-MILE': '末端派送费',
  'LAST-MILE-COST': '末端派送成本',
  CUSTOMS: '清关费',
  'CUSTOMS-COST': '清关成本',
  'COD-SERVICE': '代收服务费',
  FUEL: '燃油附加费',
  INSURANCE: '保价费',
  STORAGE: '仓储费',
  RETURN: '退运费',
  HANDLING: '操作费',
};

const feeItemPattern = /^(?:[A-Z]+-)?FEE-ITEM-(.+?)(?:-\d+)?$/;

export function feeItemLabel(code: string): string {
  const kind = feeItemPattern.exec(code)?.[1];
  if (!kind) return code;
  return feeItemKindLabels[kind] ?? code;
}

/** 一笔费用上计价重量与它的来源的几格。读口照评价转写，评价没答的缺席。 */
export type PricedWeightFields = {
  pricedWeight?: string;
  pricedWeightUnit?: string;
  weightSource?: string;
  actualWeight?: string;
  actualWeightUnit?: string;
  dimensionSides?: string[];
  dimensionUnit?: string;
  /** 出自实测且评价恰好采用一条称重量方时，那条的来源身份。 */
  measurementSource?: string;
};

/**
 * 计价重量：评价进位之后、据以查表的那一个。读口没给就答 undefined，不拿实重顶上——取较大者的价卡上，复测只把尺寸量大
 * 时实重不变而计价重量变了，顶上去的那个数是错的。
 */
export function pricedWeightText(row: PricedWeightFields): string | undefined {
  if (!row.pricedWeight) return undefined;
  return [row.pricedWeight, row.pricedWeightUnit].filter(Boolean).join(' ');
}

/** 一笔费用上「被替代」的两格。读口照册转写，没被替代时两格都缺席。 */
export type SupersessionFields = {
  superseded?: boolean;
  supersededBy?: string;
};

/**
 * 「被替代为」：有后继显示后继费用号；已被替代而没有后继，是新评价合计为零、没有新预估可接，写明不再确认——留空的话
 * 运营会以为这笔预估还能确认。没被替代答 undefined。
 */
export function supersededByText(row: SupersessionFields): string | undefined {
  if (row.supersededBy) return row.supersededBy;
  if (row.superseded) return '重估为零额，不再确认';
  return undefined;
}

/**
 * 计价重量从哪来：评价用的实重与尺寸，以及这两样出自实测还是申报。sourceLabel 把来源码翻成中文。尺寸三边由长到短。
 * 申报没报尺寸时写「未报尺寸」而不是留空——留空分不清是没报还是读口没给。评价没有测量事实（没有来源码）答 undefined。
 */
export function weightBasisText(
  row: PricedWeightFields,
  sourceLabel: (code: string) => string,
): string | undefined {
  if (!row.weightSource) return undefined;
  const parts: string[] = [];
  if (row.actualWeight) parts.push(`实重 ${[row.actualWeight, row.actualWeightUnit].filter(Boolean).join(' ')}`);
  if (row.dimensionSides && row.dimensionSides.length > 0) {
    parts.push(`尺寸 ${[row.dimensionSides.join('×'), row.dimensionUnit].filter(Boolean).join(' ')}`);
  } else {
    parts.push('未报尺寸');
  }
  // 只在出自实测且带来源身份时点出那条称重量方。申报没有这一格；没有来源身份时也不补空词，否则会写出「称重量方 undefined」。
  if (row.weightSource === 'actual-measurement' && row.measurementSource) {
    parts.push(`${sourceLabel('MEASUREMENT_SOURCE')} ${row.measurementSource}`);
  }
  return `${sourceLabel(row.weightSource)}：${parts.join('，')}`;
}

/** 一笔调整上用来拼技术详情的几格：既有「种类 方向 金额」，再加上与费用行同一组的计价重量与来源。 */
export type AdjustmentFields = PricedWeightFields & {
  kind: string;
  direction: string;
  settlementAmount: string;
};

/**
 * 技术详情里的一笔调整。重量与来源读不出时只留「种类 方向 金额」——补空括号或 undefined 会让手工计价纠错看起来像坏数据。
 * 商业让利没有评价，同样只留这三格。
 */
export function adjustmentText(adjustment: AdjustmentFields, label: (code: string) => string): string {
  const head = `${label(adjustment.kind)} ${label(adjustment.direction)} ${adjustment.settlementAmount}`;
  const extra = [pricedWeightText(adjustment), weightBasisText(adjustment, label)].filter(
    (part): part is string => part !== undefined,
  );
  if (extra.length === 0) return head;
  return `${head} ${extra.join('，')}`;
}

/**
 * 从已确认费用里读出「结算账户 → 结算相对方」的对应：对账单只带结算账户号，客户名要经这一跳再查参与方册。这是把册上
 * 已登记的关联读出来，不是从客户属性推断——未被任何已确认费用引用的账户查不到，照实留给调用方显账户号。
 */
export function accountCounterparties(
  charges: readonly { settlementAccount?: string; counterparty?: string }[],
): Map<string, string> {
  const map = new Map<string, string>();
  for (const charge of charges) {
    if (charge.settlementAccount && charge.counterparty && !map.has(charge.settlementAccount)) {
      map.set(charge.settlementAccount, charge.counterparty);
    }
  }
  return map;
}
