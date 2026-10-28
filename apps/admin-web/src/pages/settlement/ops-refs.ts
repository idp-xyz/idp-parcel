// 账务区把系统引用翻成运营看得懂的字的几个纯函数（管理台改版第一步，设计原则一「以运单为中心」）。抽成纯函数是为了
// 让「哪一格翻得出、翻不出时显什么」能在 node:test 里钉住；页面只管取数与摆放。

/** 包裹号（运单号）在各种引用里的形：`DPCL-` 加 Base32 大写字母数字。 */
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

/** 运单号认不出来时，用费用来源的中文说法顶上（如「网点收寄」）；族也认不出就原样回显整条引用。 */
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
