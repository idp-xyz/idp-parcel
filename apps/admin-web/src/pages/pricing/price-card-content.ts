// 价卡详情的解析：把读口交出的规范化文档（GET /pricing-price-cards/content 的 document）翻成运营看得懂的结构。
//
// 纯函数、零依赖组件库，node:test 直接跑。文档是服务端原样交出的 JSON，形状以 domain 的方案快照为准
// （internal/parcelpricing/domain/plan_snapshot.go），这里只读不写、不重算任何金额：「怎么收」是对着规则念出来，
// 不是替评价器再算一遍。
//
// 看不懂就降级，不丢项也不崩：每一条规则单独翻译，翻到一半遇到词典里没有的特征、比较、计算方法、价表族，整条规则
// 改成「这一项暂不支持展示」并带上原文；文档里出现本模块不认的顶层键（将来新增的规则种类）同样各占一行。降级是
// 对「文档比这份代码新」的诚实回答——把看不懂的东西悄悄省掉，运营会以为这张卡没有这一项。
//
// 数值一律按十进制字符串处理（coefficient 是字符串、scale 是小数位数），不经浮点数：金额 120.99 不会变成 120.98999。

import { chargeLineName } from './presentation';
import { chargeCodeLabel } from '../../vocabulary/status';
import {
  addressNatureValueLabels,
  aggregationLabels,
  aggregationSubjectLabels,
  amountRoundingPointLabels,
  comparisonLabels,
  currencySuffixLabels,
  featureLabels,
  lengthUnitLabels,
  membershipLabel,
  priceCardPurposeLabels,
  priceCardStatusLabels,
  priceDirectionLabels,
  referenceCatalogueKindLabels,
  referenceSeriesKindLabels,
  rateKeyNames,
  roundingModeLabels,
  roundingNoneLabel,
  volumeUnitLabels,
  weightUnitLabels,
  type PriceCardStatusCode,
} from '../../vocabulary/price-card';

// ---- 结果形状 ----

export interface FeeItem {
  /** 稳定键（给列表渲染用）。 */
  key: string;
  kind: 'base' | 'fixed' | 'surcharge' | 'bound' | 'unsupported';
  /** 费用：中文名。 */
  name: string;
  /** 怎么收：一行一句。 */
  how: string[];
  /** 什么时候收：一行一句。 */
  when: string[];
  /** 备注：互斥组、减项、按件累计、条件最低计价重量等。 */
  notes: string[];
  /** 只有看不懂的项带：原文，供折叠展开。 */
  raw?: unknown;
}

export interface CardHeader {
  planId: string;
  version: string;
  scope: string;
  /** 售价 / 成本 / 内部价。 */
  direction: string;
  /** 客户收费 / 供应商成本 / 内部结算价。 */
  purpose: string;
  /** 币种码，如 CNY；呈现面可再译成「人民币（CNY）」。 */
  currency: string;
  /** 重量单位的中文词，如 千克。 */
  weightUnit: string;
  /** 每件计价 / 按票聚合计价 / 按主单聚合计价。 */
  aggregation: string;
  /** 适用期原始时刻；endsAt 缺即持续有效（文档里用零值时刻表达）。 */
  period: { startsAt: string; endsAt: string | null };
}

export interface ReferenceRow {
  /** 种类词：燃油费率序列 / 分区目录 / 偏远档位目录…… */
  kindLabel: string;
  /** 名称：参考数据在登记册上的标识（租户给它起的名）。 */
  name: string;
  /** 这张卡拿它做什么。 */
  usedFor: string[];
  /** 能跳去的管理台页面；没有对应页面就是 null，只显示不跳。 */
  target: 'reference-series' | null;
}

export interface Refusal {
  clause: string;
  when: string;
}

export interface PriceCardContentView {
  header: CardHeader;
  feeItems: FeeItem[];
  /** 重量规则一句话；文档没有重量策略时为 null。 */
  weightRule: string | null;
  /** 金额取整一句话；没声明为 null。 */
  amountRounding: string | null;
  /** 总额上下限的金额文字；没声明的一端为 null。 */
  bounds: { minimum: string | null; maximum: string | null };
  /** 拒收条款（不接收的情形）。 */
  refusals: Refusal[];
  references: ReferenceRow[];
  /** 技术详情里的引用标识（价表、重量策略、附加费价表）。序列与目录的标识在 references 里。 */
  technicalRefs: { label: string; value: string }[];
  /** 降级成「暂不支持展示」的项数，页头据此提示。 */
  unsupportedCount: number;
}

// ---- 看不懂就降级 ----

class Unsupported extends Error {}

function fail(reason: string): never {
  throw new Unsupported(reason);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function record(value: unknown, what: string): Record<string, unknown> {
  if (!isRecord(value)) fail(`${what} 不是对象`);
  return value;
}

function text(value: unknown, what: string): string {
  if (typeof value !== 'string' || value === '') fail(`${what} 缺失`);
  return value;
}

function list(value: unknown, what: string): unknown[] {
  if (!Array.isArray(value)) fail(`${what} 不是数组`);
  return value;
}

// ---- 数值与金额 ----

/** 把「系数 / 10^标度」写成最短十进制文本；标度为负时系数后补零。 */
function formatDecimal(coefficient: string, scale: number): string {
  let digits = coefficient;
  let negative = false;
  if (digits.startsWith('-')) {
    negative = true;
    digits = digits.slice(1);
  }
  digits = digits.replace(/^0+(?=\d)/, '');
  if (scale < 0) {
    digits += '0'.repeat(-scale);
    scale = 0;
  }
  let integer = digits;
  let fraction = '';
  if (scale > 0) {
    digits = digits.padStart(scale + 1, '0');
    integer = digits.slice(0, digits.length - scale);
    fraction = digits.slice(digits.length - scale).replace(/0+$/, '');
  }
  const body = fraction === '' ? integer : `${integer}.${fraction}`;
  return negative && body !== '0' ? `-${body}` : body;
}

function parseDecimal(value: unknown, what: string): { coefficient: string; scale: number } {
  const decimal = record(value, what);
  const { coefficient, scale } = decimal;
  if (typeof coefficient !== 'string' || !/^-?\d+$/.test(coefficient)) fail(`${what} 的系数不是整数文本`);
  if (typeof scale !== 'number' || !Number.isInteger(scale) || scale < 0) fail(`${what} 的标度不合法`);
  return { coefficient, scale };
}

function decimalText(value: unknown, what: string): string {
  const { coefficient, scale } = parseDecimal(value, what);
  return formatDecimal(coefficient, scale);
}

/** 系数乘 100 再写成文本：0.8 写成 80。 */
function percentOfFactor(value: unknown, what: string): string {
  const { coefficient, scale } = parseDecimal(value, what);
  return formatDecimal(coefficient, scale - 2);
}

function isZero(value: unknown, what: string): boolean {
  return decimalText(value, what) === '0';
}

function moneyText(value: unknown, what: string): string {
  const money = record(value, what);
  const amount = decimalText(money.amount, `${what}.amount`);
  const currency = text(money.currency, `${what}.currency`);
  // 有小数时至少写两位：12.5 元读作 12.50 元，是金额的写法。
  const [integer, fraction] = amount.split('.');
  const shown = fraction === undefined ? integer : `${integer}.${fraction.padEnd(2, '0')}`;
  return `${shown} ${currencySuffixLabels[currency] ?? currency}`;
}

function quantityText(value: unknown, units: Readonly<Record<string, string>>, what: string): string {
  const quantity = record(value, what);
  const unit = text(quantity.unit, `${what}.unit`);
  return `${decimalText(quantity.value, `${what}.value`)} ${units[unit] ?? unit}`;
}

const weightText = (value: unknown, what: string) => quantityText(value, weightUnitLabels, what);
const lengthText = (value: unknown, what: string) => quantityText(value, lengthUnitLabels, what);
const volumeText = (value: unknown, what: string) => quantityText(value, volumeUnitLabels, what);

function weightUnitWord(unit: string): string {
  return weightUnitLabels[unit] ?? unit;
}

// ---- 判定条件 ----

/** 特征是数值（带单位）还是类别（取值相等）。与 domain 的 FeatureSource.measure 对齐。 */
type Measure = 'length' | 'volume' | 'weight' | 'category';

const measureOf: Readonly<Record<string, Measure>> = {
  LONGEST_SIDE: 'length',
  SECOND_LONGEST_SIDE: 'length',
  LENGTH_AND_GIRTH: 'length',
  VOLUME: 'volume',
  ACTUAL_WEIGHT: 'weight',
  VOLUMETRIC_WEIGHT: 'weight',
  CHARGEABLE_WEIGHT: 'weight',
  ZONE: 'category',
  ADDRESS_TYPE: 'category',
  ADDRESS_NATURE: 'category',
  SERVICE_OPTION: 'category',
};

/** 类别取值的呈现：地址性质是封闭词表、译成中文；分区、偏远档位、服务选项是租户的代码，原样显示。 */
function categoryValueText(source: string, value: string): { text: string; isCode: boolean } {
  if (source === 'ADDRESS_NATURE') {
    const word = addressNatureValueLabels[value];
    if (word === undefined) fail(`地址性质取值 ${value} 不在封闭词表里`);
    return { text: word, isCode: false };
  }
  return { text: value, isCode: true };
}

function predicateParts(value: unknown): { source: string; operator: string; subject: string; objectText: string; isCode: boolean } {
  const predicate = record(value, '判定条件');
  const source = text(predicate.source, '特征');
  const operator = text(predicate.operator, '比较');
  const feature = featureLabels[source];
  if (feature === undefined) fail(`特征 ${source} 不认识`);
  const comparison = comparisonLabels[operator];
  if (comparison === undefined) fail(`比较 ${operator} 不认识`);
  const measure = measureOf[source];
  if (measure === 'category') {
    if (operator !== 'EQ') fail('类别特征只能比较是否相等');
    const category = categoryValueText(source, text(predicate.category, `${source} 的取值`));
    return { source, operator, subject: feature, objectText: category.text, isCode: category.isCode };
  }
  if (operator === 'EQ') fail('数值特征不能比较是否相等');
  const threshold =
    measure === 'length'
      ? lengthText(predicate.lengthThreshold, `${source} 的阈值`)
      : measure === 'volume'
        ? volumeText(predicate.volumeThreshold, `${source} 的阈值`)
        : weightText(predicate.weightThreshold, `${source} 的阈值`);
  return { source, operator, subject: feature, objectText: threshold, isCode: false };
}

function predicateText(value: unknown): string {
  const parts = predicateParts(value);
  const comparison = comparisonLabels[parts.operator];
  // 类别取值是代码时与前面的词隔一格（「偏远档位等于 T1」），是中文词时紧挨（「地址性质等于住宅」）。
  return parts.operator === 'EQ'
    ? `${parts.subject}${comparison}${parts.isCode ? ' ' : ''}${parts.objectText}`
    : `${parts.subject}${comparison} ${parts.objectText}`;
}

/** 触发条件翻成一句话。「全部成立」用「，且」连、「任一成立」用「，或」连；同一类别特征的几个「等于」并成「属于」。 */
function triggerText(value: unknown, nested = false): string {
  const trigger = record(value, '触发条件');
  const kind = text(trigger.kind, '触发条件种类');
  if (kind === 'PREDICATE') return predicateText(trigger.predicate);
  if (kind !== 'ALL_OF' && kind !== 'ANY_OF') fail(`触发条件种类 ${kind} 不认识`);
  const operands = list(trigger.operands, '组合条件的子条件');
  if (operands.length === 0) fail('组合条件没有子条件');
  if (kind === 'ANY_OF') {
    const merged = membershipText(operands);
    if (merged !== null) return merged;
  }
  const joined = operands.map((operand) => triggerText(operand, true)).join(kind === 'ALL_OF' ? '，且' : '，或');
  return nested ? `（${joined}）` : joined;
}

/** 任一成立里全是同一个类别特征的「等于」时，合成「偏远档位属于 T1、T2」；否则 null。 */
function membershipText(operands: unknown[]): string | null {
  if (operands.length < 2) return null;
  const members: string[] = [];
  let subject = '';
  let source = '';
  let isCode = false;
  for (const operand of operands) {
    if (!isRecord(operand) || operand.kind !== 'PREDICATE') return null;
    const parts = predicateParts(operand.predicate);
    if (parts.operator !== 'EQ') return null;
    if (source !== '' && source !== parts.source) return null;
    source = parts.source;
    subject = parts.subject;
    isCode = parts.isCode;
    members.push(parts.objectText);
  }
  return `${subject}${membershipLabel}${isCode ? ' ' : ''}${members.join('、')}`;
}

// ---- 价表 ----

interface Bracket {
  minimum: string;
  maximum: string | null;
  amount: string;
  openFromZero: boolean;
}

function bracketText(bracket: Bracket, unit: string): string {
  if (bracket.maximum === null) {
    return bracket.minimum === '0' ? `不分重量段，收 ${bracket.amount}` : `${bracket.minimum} ${unit}及以上收 ${bracket.amount}`;
  }
  return bracket.minimum === '0'
    ? `不足 ${bracket.maximum} ${unit}收 ${bracket.amount}`
    : `${bracket.minimum} ${unit}起、不足 ${bracket.maximum} ${unit}收 ${bracket.amount}`;
}

/** 重量段价表（WEIGHT_ZONE 族）：按类别键（分区或偏远档位）归组，保持文档里的先后。区间是左闭右开。 */
function bracketGroups(table: Record<string, unknown>): { key: string; brackets: Bracket[] }[] {
  const unit = text(table.unit, '价表重量单位');
  const groups = new Map<string, Bracket[]>();
  for (const [index, entry] of list(table.entries, '价表条目').entries()) {
    const row = record(entry, `价表条目 ${index + 1}`);
    const key = text(row.zone, `价表条目 ${index + 1} 的类别键`);
    const minimumWeight = record(row.minimum, '重量段下界');
    if (minimumWeight.unit !== unit) fail('重量段单位与价表单位不一致');
    const maximumWeight = row.maximum === undefined ? null : record(row.maximum, '重量段上界');
    if (maximumWeight !== null && maximumWeight.unit !== unit) fail('重量段单位与价表单位不一致');
    const bracket: Bracket = {
      minimum: decimalText(minimumWeight.value, '重量段下界'),
      maximum: maximumWeight === null ? null : decimalText(maximumWeight.value, '重量段上界'),
      amount: moneyText(row.amount, '重量段金额'),
      openFromZero: maximumWeight === null && isZero(minimumWeight.value, '重量段下界'),
    };
    groups.set(key, [...(groups.get(key) ?? []), bracket]);
  }
  if (groups.size === 0) fail('价表没有条目');
  return [...groups.entries()].map(([key, brackets]) => ({ key, brackets }));
}

type KeyKind = 'zone' | 'remoteTier';

function keyLabel(kind: KeyKind, key: string): string {
  return kind === 'zone' ? `${rateKeyNames.zone} ${key}` : `${key} 档`;
}

/** 附加费价表（WEIGHT_ZONE 族）按分区或偏远档位查价。每个键只有一个不分段的价时压成一行。 */
function surchargeTableLines(table: Record<string, unknown>, kind: KeyKind): string[] {
  if (table.family !== 'WEIGHT_ZONE') fail(`附加费价表族 ${String(table.family)} 暂不支持展示`);
  const unit = weightUnitWord(text(table.unit, '价表重量单位'));
  const keyName = kind === 'zone' ? rateKeyNames.zone : rateKeyNames.remoteTier;
  const groups = bracketGroups(table);
  if (groups.every((group) => group.brackets.length === 1 && group.brackets[0].openFromZero)) {
    return [`按${keyName}查价表：${groups.map((group) => `${keyLabel(kind, group.key)} ${group.brackets[0].amount}`).join('；')}`];
  }
  return [
    `按${keyName}和重量段查价表`,
    ...groups.map((group) => `${keyLabel(kind, group.key)}：${group.brackets.map((bracket) => bracketText(bracket, unit)).join('；')}`),
  ];
}

/** 基础运费的价表，三个价表族各念各的。 */
function baseRateLines(table: Record<string, unknown>): string[] {
  const unit = weightUnitWord(text(table.unit, '价表重量单位'));
  const family = text(table.family, '价表族');
  switch (family) {
    case 'FIRST_CONTINUE': {
      const rows = list(table.firstContinue, '首重续重条目');
      if (rows.length === 0) fail('首重续重价表没有条目');
      return [
        '按首重续重计价，续重不足一档按一档计',
        ...rows.map((row, index) => {
          const rate = record(row, `首重续重条目 ${index + 1}`);
          const zone = text(rate.zone, '分区');
          const first = weightText(rate.firstWeight, '首重');
          const step = weightText(rate.step, '续重步长');
          return `${keyLabel('zone', zone)}：首重 ${first}收 ${moneyText(rate.firstAmount, '首重价')}，之后每 ${step}加收 ${moneyText(rate.stepAmount, '续重价')}`;
        }),
      ];
    }
    case 'WEIGHT_ZONE':
      return [
        '按分区和重量段查价',
        ...bracketGroups(table).map(
          (group) => `${keyLabel('zone', group.key)}：${group.brackets.map((bracket) => bracketText(bracket, unit)).join('；')}`,
        ),
      ];
    case 'UNIT_PRICE': {
      const rows = list(table.unitPrice, '单价条目');
      if (rows.length === 0) fail('单价价表没有条目');
      return [
        '按计价重量乘单价计价',
        ...rows.map((row, index) => {
          const rate = record(row, `单价条目 ${index + 1}`);
          return `${keyLabel('zone', text(rate.zone, '分区'))}：每${unit} ${moneyText(rate.amountPerUnit, '单价')}`;
        }),
      ];
    }
    default:
      return fail(`价表族 ${family} 暂不支持展示`);
  }
}

// ---- 计算方法 ----

interface Context {
  aggregation: string;
  /** 费用依赖（基数）的声明，按标识查。 */
  dependencies: ReadonlyMap<string, Record<string, unknown>>;
}

function fixedAmountText(amount: string, effect: string, unit: string, aggregation: string): string {
  const subject = aggregationSubjectLabels[aggregation] ?? '每票';
  if (effect !== 'ADD' && effect !== 'DEDUCT') fail(`费用方向 ${effect} 不认识`);
  if (unit === 'PER_PIECE') return effect === 'ADD' ? `每件 ${amount}，按件数累计` : `每件减 ${amount}，按件数累计`;
  if (unit !== '') fail(`计收单位 ${unit} 不认识`);
  return effect === 'ADD' ? `${subject}固定 ${amount}` : `${subject}减 ${amount}`;
}

function basisText(dependencyId: string, context: Context): string {
  const dependency = context.dependencies.get(dependencyId);
  if (dependency === undefined) fail(`找不到基数声明 ${dependencyId}`);
  const codes = (value: unknown): string[] => (Array.isArray(value) ? value.map((code) => text(code, '费用代码')) : []);
  const includes = codes(dependency.includes);
  const excludes = codes(dependency.excludes);
  const composition = text(dependency.composition, '基数构成');
  if (composition === 'LISTED_CHARGES') {
    const names = includes.filter((code) => !excludes.includes(code)).map((code) => chargeCodeLabel(code));
    if (names.length === 0) fail('基数没有列出任何费用');
    return names.join('、');
  }
  if (composition === 'ALL_CHARGES') {
    return excludes.length === 0 ? '其他已计收费用之和' : `其他已计收费用之和（不含${excludes.map((code) => chargeCodeLabel(code)).join('、')}）`;
  }
  return fail(`基数构成 ${composition} 不认识`);
}

/** 一条附加费的计算方法念成「怎么收」。 */
function calculationLines(calculationValue: unknown, effect: string, unit: string, context: Context): string[] {
  const calculation = record(calculationValue, '计算方法');
  const method = text(calculation.method, '计算方法种类');
  switch (method) {
    case 'FIXED_AMOUNT':
      return [fixedAmountText(moneyText(calculation.amount, '定额'), effect, unit, context.aggregation)];
    case 'TABLE_LOOKUP': {
      const table = record(calculation.table, '附加费价表');
      const tableKey = calculation.tableKey;
      if (tableKey !== undefined && tableKey !== '' && tableKey !== 'REMOTE_TIER') fail(`查档维度 ${String(tableKey)} 不认识`);
      return surchargeTableLines(table, tableKey === 'REMOTE_TIER' ? 'remoteTier' : 'zone');
    }
    case 'PERCENT_OF_BASIS': {
      const basis = basisText(text(calculation.basis, '基数声明标识'), context);
      if (calculation.seriesKind !== undefined && calculation.seriesKind !== '') {
        const kind = text(calculation.seriesKind, '序列种类');
        const series = referenceSeriesKindLabels[kind];
        if (series === undefined) fail(`序列种类 ${kind} 不认识`);
        const factor = calculation.seriesFactor === undefined ? '100' : percentOfFactor(calculation.seriesFactor, '序列系数');
        const tail = factor === '100' ? '' : `，再乘 ${factor}%`;
        return [`按${basis}的百分比收取，百分比为${series}的当期费率${tail}`];
      }
      return [`按${basis}的 ${decimalText(calculation.percentage, '百分比')}% 收取`];
    }
    case 'SERIES_AMOUNT': {
      const behaviour = text(calculation.outOfWindow, '窗外行为');
      if (behaviour !== 'NOT_CHARGED' && behaviour !== 'PENDING') fail(`窗外行为 ${behaviour} 不认识`);
      return [
        `按「${text(calculation.seriesId, '序列标识')}」当期公布的金额收取`,
        behaviour === 'NOT_CHARGED' ? '当期没有公布金额时不收' : '当期没有公布金额时先不出价，等公布后再判断',
      ];
    }
    case 'GREATER_OF': {
      const operands = list(calculation.operands, '取较大者的操作数');
      if (operands.length < 2) fail('取较大者不足两项');
      return [
        '下面几项取较大的一项：',
        ...operands.flatMap((operand) =>
          calculationLines(operand, effect, unit, context).map((line, index) => (index === 0 ? `· ${line}` : `  ${line}`)),
        ),
      ];
    }
    default:
      return fail(`计算方法 ${method} 暂不支持展示`);
  }
}

// ---- 费用项 ----

function ruleName(rule: Record<string, unknown>): string {
  const code = typeof rule.code === 'string' ? rule.code : '';
  const description = typeof rule.description === 'string' ? rule.description : '';
  return chargeLineName({ code, description });
}

function unsupportedItem(key: string, name: string, raw: unknown): FeeItem {
  return { key, kind: 'unsupported', name, how: ['这一项暂不支持展示'], when: [], notes: [], raw };
}

/** 附加费规则的互斥说明：同组其他规则的名字，优先级相同时「只收较高的一项」。 */
function exclusivityNotes(rule: Record<string, unknown>, rules: Record<string, unknown>[]): string[] {
  const stance = text(rule.exclusivity, '互斥立场');
  if (stance === 'STANDALONE') return [];
  if (stance !== 'GROUPED') fail(`互斥立场 ${stance} 不认识`);
  const group = text(rule.exclusivityGroup, '互斥组');
  const peers = rules.filter((other) => other !== rule && other.exclusivity === 'GROUPED' && other.exclusivityGroup === group);
  if (peers.length === 0) return [];
  const names = peers.map((peer) => `「${ruleName(peer)}」`).join('、');
  const samePriority = peers.every((peer) => (peer.priority ?? 0) === (rule.priority ?? 0));
  return [
    samePriority
      ? `与${names}同组，同时命中只收较高的一项`
      : `与${names}同组，同时命中只收一项：先按优先级（1 最高），同级收较高的一项`,
  ];
}

function surchargeItem(rule: Record<string, unknown>, index: number, rules: Record<string, unknown>[], context: Context): FeeItem {
  const key = `surcharge:${typeof rule.id === 'string' ? rule.id : index}`;
  try {
    const effect = text(rule.effect, '费用方向');
    const unit = typeof rule.unit === 'string' ? rule.unit : '';
    const notes = exclusivityNotes(rule, rules);
    const how = calculationLines(rule.calculation, effect, unit, context);
    if (effect === 'DEDUCT') notes.push('减项：金额从合计里扣除');
    if (unit === 'PER_PIECE' && !how.some((line) => line.includes('按件数累计'))) notes.push('按件数累计');
    if (rule.conditionalMinimumWeight !== undefined) {
      const minimum = record(rule.conditionalMinimumWeight, '条件最低计价重量');
      notes.push(`另有最低计价重量：${triggerText(minimum.condition)}时，计价重量不低于 ${weightText(minimum.minimum, '最低计价重量')}`);
    }
    return { key, kind: 'surcharge', name: ruleName(rule), how, when: [triggerText(rule.condition)], notes };
  } catch (error) {
    if (!(error instanceof Unsupported)) throw error;
    return unsupportedItem(key, ruleName(rule) || '未识别的费用项', rule);
  }
}

function fixedItem(rule: Record<string, unknown>, index: number, context: Context): FeeItem {
  const key = `fixed:${typeof rule.id === 'string' ? rule.id : index}`;
  try {
    const effect = text(rule.effect, '费用方向');
    const unit = typeof rule.unit === 'string' ? rule.unit : '';
    const subject = aggregationSubjectLabels[context.aggregation] ?? '每票';
    const notes: string[] = [];
    if (effect === 'DEDUCT') notes.push('减项：金额从合计里扣除');
    return {
      key,
      kind: 'fixed',
      name: ruleName(rule),
      how: [fixedAmountText(moneyText(rule.amount, '定额'), effect, unit, context.aggregation)],
      // 固定规则没有触发条件，念出来就是「每件都收」，不往好听里改。
      when: [`${subject}都收`],
      notes,
    };
  } catch (error) {
    if (!(error instanceof Unsupported)) throw error;
    return unsupportedItem(key, ruleName(rule) || '未识别的费用项', rule);
  }
}

// ---- 重量规则 ----

function roundingText(segmentsValue: unknown, what: string): string {
  const segments = list(segmentsValue, what);
  if (segments.length === 0) fail(`${what} 没有取整段`);
  const parsed = segments.map((segmentValue) => {
    const segment = record(segmentValue, what);
    const mode = text(segment.mode, '取整模式');
    const phrase = mode === 'NONE' ? roundingNoneLabel : `${roundingModeLabels[mode] ?? fail(`取整模式 ${mode} 不认识`)} ${weightText(segment.increment, '进位单位')}`;
    return { phrase, upTo: segment.maximum === undefined ? null : weightText(segment.maximum, '取整段上界') };
  });
  if (parsed.length === 1) return parsed[0].phrase;
  // 分段取整：每一段认领其上界以下的重量（左闭右开），最后一段开口。
  let previous: string | null = null;
  const pieces = parsed.map((entry) => {
    const scope: string =
      entry.upTo === null ? `${previous}及以上` : previous === null ? `不足 ${entry.upTo}` : `${previous}起、不足 ${entry.upTo}`;
    previous = entry.upTo;
    return `${scope}${entry.phrase}`;
  });
  return pieces.join('，');
}

function weightRuleText(policyValue: unknown): string {
  const policy = record(policyValue, '重量策略');
  const method = text(policy.method, '计价重量方法');
  const rounding = roundingText(policy.rounding, '计价重量取整');
  if (method === 'ACTUAL_ONLY') return `只按实重计价，不计体积重；计价重量${rounding}。`;
  if (method !== 'MAX') fail(`计价重量方法 ${method} 不认识`);
  const factor = record(policy.volumetricFactor, '体积重系数');
  const divisor = decimalText(factor.divisor, '体积重除数');
  const lengthUnit = text(factor.lengthUnit, '体积重长度单位');
  const volumetricRounding = roundingText(factor.rounding, '体积重取整');
  return (
    `取实重与体积重较大者；体积重 = 长 × 宽 × 高 ÷ ${divisor}（长宽高按${lengthUnitLabels[lengthUnit] ?? lengthUnit}），体积重${volumetricRounding}；` +
    `取较大者后，计价重量${rounding}。`
  );
}

function amountRoundingText(value: unknown): string {
  const rounding = record(value, '金额取整');
  const mode = text(rounding.mode, '金额取整模式');
  const word = roundingModeLabels[mode];
  if (word === undefined) fail(`金额取整模式 ${mode} 不认识`);
  const points = list(rounding.points, '金额取整应用点').map((point) => {
    const label = amountRoundingPointLabels[text(point, '金额取整应用点')];
    return label ?? fail('金额取整应用点不认识');
  });
  if (points.length === 0) fail('金额取整没有应用点');
  const where = points.length === 1 ? `只对${points[0]}取整` : `${points.join('和')}各取整一次`;
  return `金额${word} ${moneyText(rounding.increment, '金额取整进位单位')}，${where}`;
}

// ---- 参考数据 ----

function referenceRows(plan: Record<string, unknown>, feeItems: FeeItem[], surcharges: Record<string, unknown>[]): ReferenceRow[] {
  const rows: ReferenceRow[] = [];
  if (Array.isArray(plan.referenceSeries)) {
    for (const bindingValue of plan.referenceSeries) {
      const binding = isRecord(bindingValue) ? bindingValue : {};
      const kind = typeof binding.kind === 'string' ? binding.kind : '';
      const seriesId = typeof binding.seriesId === 'string' ? binding.seriesId : '';
      const usedFor = surcharges
        .map((rule, index) => ({ rule, index }))
        .filter(({ rule }) => usesSeries(rule.calculation, kind, seriesId))
        .map(({ rule, index }) => feeItems.find((item) => item.key === `surcharge:${typeof rule.id === 'string' ? rule.id : index}`)?.name ?? ruleName(rule));
      rows.push({
        kindLabel: referenceSeriesKindLabels[kind] ?? '未识别的参考数据',
        name: seriesId,
        usedFor: usedFor.length > 0 ? [`用于：${usedFor.join('、')}`] : ['这张卡绑定了它，但没有费用项在读它'],
        target: referenceSeriesKindLabels[kind] === undefined ? null : 'reference-series',
      });
    }
  }
  if (Array.isArray(plan.referenceCatalogues)) {
    for (const linkValue of plan.referenceCatalogues) {
      const link = isRecord(linkValue) ? linkValue : {};
      const kind = typeof link.kind === 'string' ? link.kind : '';
      const catalogueId = typeof link.catalogueId === 'string' ? link.catalogueId : '';
      rows.push({
        kindLabel: referenceCatalogueKindLabels[kind] ?? '未识别的参考数据',
        name: catalogueId,
        usedFor: [kind === 'ZONE' ? '用于：按收件邮编查分区' : kind === 'REMOTE_TIER' ? '用于：按收件邮编查偏远档位' : '用途不明'],
        // 目录今天没有对应的管理台页面，只显示、不跳。
        target: null,
      });
    }
  }
  return rows;
}

function usesSeries(calculation: unknown, kind: string, seriesId: string): boolean {
  if (!isRecord(calculation)) return false;
  if (calculation.seriesKind === kind && kind !== '') return true;
  if (calculation.method === 'SERIES_AMOUNT' && calculation.seriesId === seriesId) return true;
  return Array.isArray(calculation.operands) && calculation.operands.some((operand) => usesSeries(operand, kind, seriesId));
}

function referenceId(value: unknown): string {
  if (!isRecord(value)) return '';
  const id = typeof value.id === 'string' ? value.id : '';
  const version = typeof value.version === 'string' ? value.version : '';
  return id === '' ? '' : version === '' ? id : `${id}@${version}`;
}

// ---- 总入口 ----

/** 本模块认识的顶层键。出现别的键，说明文档比这份代码新（将来新增的规则种类），各占一行降级。 */
const knownPlanKeys = new Set([
  'canonicalization',
  'reference',
  'scope',
  'direction',
  'purpose',
  'baseChargeCode',
  'aggregation',
  'period',
  'rateTable',
  'weightPolicy',
  'rules',
  'surchargeRules',
  'chargeDependencies',
  'referenceSeries',
  'exclusions',
  'amountRounding',
  'referenceCatalogues',
  'totalChargeBounds',
  'manifest',
  'contentDigest',
]);

const emptyHeader: CardHeader = {
  planId: '',
  version: '',
  scope: '',
  direction: '',
  purpose: '',
  currency: '',
  weightUnit: '',
  aggregation: '',
  period: { startsAt: '', endsAt: null },
};

function str(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

/** 文档里「无上界」用零值时刻表达（0001-01-01T00:00:00Z）。 */
function openEnded(endsAt: string): boolean {
  return endsAt === '' || endsAt.startsWith('0001-01-01');
}

function headerOf(plan: Record<string, unknown>): CardHeader {
  const reference = isRecord(plan.reference) ? plan.reference : {};
  const table = isRecord(plan.rateTable) ? plan.rateTable : {};
  const period = isRecord(plan.period) ? plan.period : {};
  const direction = str(plan.direction);
  const purpose = str(plan.purpose);
  const aggregation = str(plan.aggregation);
  const unit = str(table.unit);
  const endsAt = str(period.endsAt);
  return {
    planId: str(reference.id),
    version: str(reference.version),
    scope: str(plan.scope),
    direction: priceDirectionLabels[direction] ?? direction,
    purpose: priceCardPurposeLabels[purpose] ?? purpose,
    currency: str(table.currency),
    weightUnit: unit === '' ? '' : weightUnitWord(unit),
    aggregation: aggregationLabels[aggregation] ?? aggregation,
    period: { startsAt: str(period.startsAt), endsAt: openEnded(endsAt) ? null : endsAt },
  };
}

function baseItem(plan: Record<string, unknown>, context: Context): FeeItem {
  const code = str(plan.baseChargeCode);
  const name = code === '' ? '基础运费' : chargeCodeLabel(code);
  const subject = aggregationSubjectLabels[context.aggregation] ?? '每票';
  try {
    return { key: 'base', kind: 'base', name, how: baseRateLines(record(plan.rateTable, '基础价表')), when: [`${subject}都收`], notes: [] };
  } catch (error) {
    if (!(error instanceof Unsupported)) throw error;
    return unsupportedItem('base', name, plan.rateTable);
  }
}

function boundItems(plan: Record<string, unknown>): { items: FeeItem[]; minimum: string | null; maximum: string | null } {
  if (plan.totalChargeBounds === undefined) return { items: [], minimum: null, maximum: null };
  try {
    const bounds = record(plan.totalChargeBounds, '总额上下限');
    const minimum = bounds.minimum === undefined ? null : moneyText(bounds.minimum, '最低收费');
    const maximum = bounds.maximum === undefined ? null : moneyText(bounds.maximum, '最高收费');
    const items: FeeItem[] = [];
    if (minimum !== null) {
      items.push({
        key: 'bound:minimum',
        kind: 'bound',
        name: chargeCodeLabel('MINIMUM_CHARGE'),
        how: [`合计低于 ${minimum}时，补足到 ${minimum}`],
        when: [`各项费用合计低于 ${minimum}时`],
        notes: ['只补差额，单独列一行；其他费用不改'],
      });
    }
    if (maximum !== null) {
      items.push({
        key: 'bound:maximum',
        kind: 'bound',
        name: chargeCodeLabel('MAXIMUM_CHARGE'),
        how: [`合计高于 ${maximum}时，超出的部分扣减，合计封顶 ${maximum}`],
        when: [`各项费用合计高于 ${maximum}时`],
        notes: ['减项：扣减的差额单独列一行'],
      });
    }
    if (items.length === 0) fail('总额上下限两端都没有');
    return { items, minimum, maximum };
  } catch (error) {
    if (!(error instanceof Unsupported)) throw error;
    return { items: [unsupportedItem('bound', '最低收费 / 最高收费', plan.totalChargeBounds)], minimum: null, maximum: null };
  }
}

/**
 * 把读口交出的规范化文档翻成详情栏的人话结构。文档不是对象时交回一份空壳，只带一条「暂不支持展示」的项和原文。
 * 任何一处看不懂都只降级那一处，不抛。
 */
export function parsePriceCardContent(document: unknown): PriceCardContentView {
  if (!isRecord(document)) {
    return {
      header: emptyHeader,
      feeItems: [unsupportedItem('document', '价卡内容', document)],
      weightRule: null,
      amountRounding: null,
      bounds: { minimum: null, maximum: null },
      refusals: [],
      references: [],
      technicalRefs: [],
      unsupportedCount: 1,
    };
  }

  const header = headerOf(document);
  const aggregationCode = str(document.aggregation);
  const dependencies = new Map<string, Record<string, unknown>>();
  if (Array.isArray(document.chargeDependencies)) {
    for (const dependency of document.chargeDependencies) {
      if (isRecord(dependency) && typeof dependency.id === 'string') dependencies.set(dependency.id, dependency);
    }
  }
  const context: Context = { aggregation: aggregationCode, dependencies };

  const feeItems: FeeItem[] = [baseItem(document, context)];

  const fixedRules = (Array.isArray(document.rules) ? document.rules : []).filter(isRecord);
  [...fixedRules]
    .sort((left, right) => Number(left.order ?? 0) - Number(right.order ?? 0))
    .forEach((rule, index) => feeItems.push(fixedItem(rule, index, context)));

  const surcharges = (Array.isArray(document.surchargeRules) ? document.surchargeRules : []).filter(isRecord);
  surcharges.forEach((rule, index) => feeItems.push(surchargeItem(rule, index, surcharges, context)));

  const bounds = boundItems(document);
  feeItems.push(...bounds.items);

  // 文档比这份代码新：没见过的顶层键各占一行，原文带上。
  for (const [key, value] of Object.entries(document)) {
    if (!knownPlanKeys.has(key)) feeItems.push(unsupportedItem(`unknown:${key}`, '未识别的内容', { [key]: value }));
  }

  let weightRule: string | null = null;
  if (document.weightPolicy !== undefined) {
    try {
      weightRule = weightRuleText(document.weightPolicy);
    } catch (error) {
      if (!(error instanceof Unsupported)) throw error;
      weightRule = '重量规则暂不支持展示';
      feeItems.push(unsupportedItem('weight-policy', '重量规则', document.weightPolicy));
    }
  }

  let amountRounding: string | null = null;
  if (document.amountRounding !== undefined) {
    try {
      amountRounding = amountRoundingText(document.amountRounding);
    } catch (error) {
      if (!(error instanceof Unsupported)) throw error;
      amountRounding = '金额取整暂不支持展示';
      feeItems.push(unsupportedItem('amount-rounding', '金额取整', document.amountRounding));
    }
  }

  const refusals: Refusal[] = [];
  if (Array.isArray(document.exclusions)) {
    document.exclusions.forEach((exclusion, index) => {
      try {
        const row = record(exclusion, `拒收条款 ${index + 1}`);
        refusals.push({ clause: text(row.clause, '拒收条款出处'), when: triggerText(row.condition) });
      } catch (error) {
        if (!(error instanceof Unsupported)) throw error;
        feeItems.push(unsupportedItem(`exclusion:${index}`, '拒收条款', exclusion));
      }
    });
  }

  const technicalRefs: { label: string; value: string }[] = [];
  const baseTable = referenceId(isRecord(document.rateTable) ? document.rateTable.reference : undefined);
  if (baseTable !== '') technicalRefs.push({ label: '价表', value: baseTable });
  const weightPolicyRef = referenceId(isRecord(document.weightPolicy) ? document.weightPolicy.reference : undefined);
  if (weightPolicyRef !== '') technicalRefs.push({ label: '重量策略', value: weightPolicyRef });
  for (const rule of surcharges) {
    const calculation = isRecord(rule.calculation) ? rule.calculation : {};
    const tableRef = referenceId(isRecord(calculation.table) ? calculation.table.reference : undefined);
    if (tableRef !== '') technicalRefs.push({ label: '附加费价表', value: tableRef });
  }

  return {
    header,
    feeItems,
    weightRule,
    amountRounding,
    bounds: { minimum: bounds.minimum, maximum: bounds.maximum },
    refusals,
    references: referenceRows(document, feeItems, surcharges),
    technicalRefs,
    unsupportedCount: feeItems.filter((item) => item.kind === 'unsupported').length,
  };
}

// ---- 价格表列表的状态 ----

export interface PriceCardStatusInput {
  effectiveFrom: string;
  effectiveTo?: string;
  selectionEndsAt?: string;
  retiredAt?: string;
}

/**
 * 一行价卡此刻的状态：退役 > 截止 > 适用期。退役与截止是发布责任方在册上另记的收尾（它们不改发布时登记的适用期），
 * 所以排在适用期前面；截止点在将来的不算截止，仍按适用期判。
 */
export function priceCardStatus(row: PriceCardStatusInput, now: Date): PriceCardStatusCode {
  const at = now.getTime();
  const time = (value: string | undefined) => (value === undefined || value === '' ? Number.NaN : Date.parse(value));
  if (!Number.isNaN(time(row.retiredAt)) && time(row.retiredAt) <= at) return 'RETIRED';
  if (!Number.isNaN(time(row.selectionEndsAt)) && time(row.selectionEndsAt) <= at) return 'CUT_OFF';
  if (time(row.effectiveFrom) > at) return 'NOT_YET';
  if (!Number.isNaN(time(row.effectiveTo)) && time(row.effectiveTo) <= at) return 'EXPIRED';
  return 'IN_FORCE';
}

export function priceCardStatusLabel(code: PriceCardStatusCode): { label: string; tone: 'good' | 'info' | 'warn' | 'bad' | 'muted' } {
  return priceCardStatusLabels[code];
}
