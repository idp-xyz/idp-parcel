import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parsePriceCardContent, priceCardStatus, type FeeItem, type PriceCardContentView } from './price-card-content';

// 价卡详情的解析。五张演示种子卡取自仓里的真实文档（scripts/demo-seeds/data/pricing，种子的 plan 块就是登记的规范化
// 文档，后端读口原样交出的就是它）；路径相对 apps/admin-web，run-tests.mjs 固定以它为 cwd 起 node --test。
// 其余用例在种子上改一处或手造一个小文档，钉各计算方法、各价表族与「看不懂就降级」。

const seedRoot = '../../scripts/demo-seeds/data/pricing';

function seedPlan(name: string): Record<string, any> {
  return JSON.parse(readFileSync(`${seedRoot}/${name}`, 'utf8')).plan;
}

const v1 = () => seedPlan('price-card-cn-sg.json');
const cost = () => seedPlan('price-card-cn-sg-cost.json');
const full1 = () => seedPlan('price-card-cn-sg-full.json');
const full2 = () => seedPlan('price-card-cn-sg-full-02.json');
const full3 = () => seedPlan('price-card-cn-sg-full-03.json');

function itemNamed(view: PriceCardContentView, name: string): FeeItem {
  const found = view.feeItems.find((item) => item.name === name);
  ok(found, `没有叫「${name}」的费用项：${view.feeItems.map((item) => item.name).join(' / ')}`);
  return found;
}

const names = (view: PriceCardContentView) => view.feeItems.map((item) => item.name);

const money = (amount: string, scale = 0, currency = 'CNY') => ({ amount: { coefficient: amount, scale }, currency });
const decimal = (coefficient: string, scale = 0) => ({ coefficient, scale });
const weight = (coefficient: string, scale = 0, unit = 'KG') => ({ value: decimal(coefficient, scale), unit });

// ---- 五张种子卡 ----

// Covers: v1 售价卡（SYN-PLAN-CN-SG-01）——基础运费按首重续重，一条无条件固定规则；它绑了燃油序列却没有费用项在读它，
// 这一点照实写出，不美化。
test('v1 售价卡：两项费用，固定规则无条件写「每件都收」', () => {
  const view = parsePriceCardContent(v1());
  deepEqual(names(view), ['基础运费', 'SYN 偏远区域附加（合成演示值）']);

  deepEqual(view.feeItems[0].how, [
    '按首重续重计价，续重不足一档按一档计',
    '分区 Z1：首重 0.5 千克收 55 元，之后每 0.5 千克加收 18 元',
    '分区 Z2：首重 0.5 千克收 48 元，之后每 0.5 千克加收 15 元',
  ]);
  deepEqual(view.feeItems[0].when, ['每件都收']);

  deepEqual(view.feeItems[1].how, ['每件固定 5 元']);
  deepEqual(view.feeItems[1].when, ['每件都收']);
  deepEqual(view.feeItems[1].notes, []);

  deepEqual(view.header, {
    planId: 'SYN-PLAN-CN-SG-01',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
    direction: '售价',
    purpose: '客户收费',
    currency: 'CNY',
    weightUnit: '千克',
    aggregation: '每件计价',
    period: { startsAt: '2026-01-01T00:00:00Z', endsAt: null },
  });
  equal(
    view.weightRule,
    '取实重与体积重较大者；体积重 = 长 × 宽 × 高 ÷ 5000（长宽高按厘米），体积重向上进位到 0.1 千克；取较大者后，计价重量向上进位到 0.5 千克。',
  );
  equal(view.amountRounding, '金额四舍五入到 0.01 元，只对合计取整');
  deepEqual(view.bounds, { minimum: null, maximum: null });
  deepEqual(view.references, [
    { kindLabel: '燃油费率序列', name: 'SYN-SERIES-FUEL-01', usedFor: ['这张卡绑定了它，但没有费用项在读它'], target: 'reference-series' },
    { kindLabel: '分区目录', name: 'SYN-CAT-ZONE-CN-SG', usedFor: ['用于：按收件邮编查分区'], target: null },
  ]);
  equal(view.unsupportedCount, 0);
});

// Covers: 成本卡（SYN-PLAN-CN-SG-COST-01）——重量段查价表族，只按实重；方向与用途译成「成本 / 供应商成本」。
test('成本卡：一项基础运费，按分区和重量段查价，只按实重', () => {
  const view = parsePriceCardContent(cost());
  deepEqual(names(view), ['基础运费']);
  deepEqual(view.feeItems[0].how, [
    '按分区和重量段查价',
    '分区 Z1：不足 1 千克收 30 元；1 千克起、不足 5 千克收 85 元；5 千克起、不足 30 千克收 260 元',
  ]);
  equal(view.header.direction, '成本');
  equal(view.header.purpose, '供应商成本');
  equal(view.weightRule, '只按实重计价，不计体积重；计价重量向上进位到 0.5 千克。');
  deepEqual(view.references, [{ kindLabel: '分区目录', name: 'SYN-CAT-ZONE-CN-SG', usedFor: ['用于：按收件邮编查分区'], target: null }]);
});

// Covers: FULL-01——燃油按序列当期费率、超尺寸与超重同组、偏远一档二档各一条规则（同组、优先级不同）。
test('完整价卡一：六项费用，条件与互斥组念成人话', () => {
  const view = parsePriceCardContent(full1());
  deepEqual(names(view), [
    '基础运费',
    '燃油附加费（合成演示值）',
    '超尺寸费（合成演示值）',
    '超重费（合成演示值）',
    '偏远地区费·一档（合成演示值）',
    '偏远地区费·二档（合成演示值）',
  ]);

  const fuel = itemNamed(view, '燃油附加费（合成演示值）');
  deepEqual(fuel.how, ['按基础运费的百分比收取，百分比为燃油费率序列的当期费率']);
  // 恒真条件照实念：引擎没有无条件附加费，燃油对每票收是靠「实重大于 0」（ADR-0348），不改写成「每件都收」。
  deepEqual(fuel.when, ['实重大于 0 千克']);

  const oversize = itemNamed(view, '超尺寸费（合成演示值）');
  deepEqual(oversize.how, ['每件固定 35 元']);
  deepEqual(oversize.when, ['最长边大于 100 厘米，或长加周长大于 300 厘米']);
  deepEqual(oversize.notes, ['与「超重费（合成演示值）」同组，同时命中只收较高的一项']);

  const overweight = itemNamed(view, '超重费（合成演示值）');
  deepEqual(overweight.how, ['每件固定 25 元']);
  deepEqual(overweight.when, ['实重大于 20 千克']);
  deepEqual(overweight.notes, ['与「超尺寸费（合成演示值）」同组，同时命中只收较高的一项']);

  const remote1 = itemNamed(view, '偏远地区费·一档（合成演示值）');
  deepEqual(remote1.how, ['每件固定 12 元']);
  deepEqual(remote1.when, ['偏远档位等于 T1']);
  // 同组但优先级不同：不能说「只收较高的一项」，先按优先级（1 最高）。
  deepEqual(remote1.notes, ['与「偏远地区费·二档（合成演示值）」同组，同时命中只收一项：先按优先级（1 最高），同级收较高的一项']);
  deepEqual(itemNamed(view, '偏远地区费·二档（合成演示值）').how, ['每件固定 28 元']);

  equal(view.amountRounding, '金额四舍五入到 0.01 元，每一项费用和合计各取整一次');
  deepEqual(
    view.references.map((reference) => [reference.kindLabel, reference.name]),
    [
      ['燃油费率序列', 'SYN-SERIES-FUEL-FULL-01'],
      ['偏远档位目录', 'SYN-CAT-REMOTE-TIER-CN-SG'],
      ['分区目录', 'SYN-CAT-ZONE-CN-SG-FULL'],
    ],
  );
  deepEqual(view.references[0].usedFor, ['用于：燃油附加费（合成演示值）']);
  equal(view.references[0].target, 'reference-series');
  equal(view.references[1].target, null, '目录没有对应的管理台页面，只显示不跳');
});

// Covers: FULL-02——超重读自身计价重量（特征 CHARGEABLE_WEIGHT，界面叫「计价重量（不含最低计价重量抬高）」）、住宅派送读地址性质。
test('完整价卡二：超重读自身计价重量，住宅派送读地址性质，共七项', () => {
  const view = parsePriceCardContent(full2());
  equal(view.feeItems.length, 7);
  deepEqual(itemNamed(view, '超重费（合成演示值）').when, ['计价重量（不含最低计价重量抬高）大于 20 千克']);
  const residential = itemNamed(view, '住宅派送费（合成演示值）');
  deepEqual(residential.how, ['每件固定 6 元']);
  deepEqual(residential.when, ['地址性质等于住宅']);
  deepEqual(residential.notes, []);
});

// Covers: FULL-03——偏远一条规则按档位查价表（触发条件列档位、价表给各档金额）、最低收费 120.99 元（PPC-6）。
test('完整价卡三：偏远按档位查表，最低收费补足到下限', () => {
  const view = parsePriceCardContent(full3());
  deepEqual(names(view), [
    '基础运费',
    '燃油附加费（合成演示值）',
    '超尺寸费（合成演示值）',
    '超重费（合成演示值）',
    '偏远地区费（合成演示值）',
    '最低收费',
  ]);

  const remote = itemNamed(view, '偏远地区费（合成演示值）');
  deepEqual(remote.how, ['按偏远档位查价表：T1 档 12 元；T2 档 28 元']);
  deepEqual(remote.when, ['偏远档位属于 T1、T2']);
  deepEqual(remote.notes, [], '这条规则单独成项，没有互斥组');

  const minimum = itemNamed(view, '最低收费');
  equal(minimum.kind, 'bound');
  deepEqual(minimum.how, ['合计低于 120.99 元时，补足到 120.99 元']);
  deepEqual(minimum.when, ['各项费用合计低于 120.99 元时']);
  deepEqual(minimum.notes, ['只补差额，单独列一行；其他费用不改']);
  deepEqual(view.bounds, { minimum: '120.99 元', maximum: null });

  deepEqual(view.technicalRefs, [
    { label: '价表', value: 'SYN-TABLE-CN-SG-FULL-03@v1' },
    { label: '重量策略', value: 'SYN-WEIGHT-CN-SG-FULL-03@v1' },
    { label: '附加费价表', value: 'SYN-TABLE-CN-SG-FULL-03-REMOTE@v1' },
  ]);
  equal(view.unsupportedCount, 0);
});

// Covers: 界面文案不泄漏英文枚举——五张种子卡翻出来的每一句，剔除 SYN- 标识与分区 / 档位代码之后，不剩两个以上连续的英文字母。
test('种子卡翻出的文案没有英文枚举', () => {
  const allowed = /SYN-[A-Z0-9-]+|\bSYN\b|\b[ZT]\d+\b/g;
  for (const plan of [v1(), cost(), full1(), full2(), full3()]) {
    const view = parsePriceCardContent(plan);
    const sentences = [
      view.header.direction,
      view.header.purpose,
      view.header.weightUnit,
      view.header.aggregation,
      view.weightRule ?? '',
      view.amountRounding ?? '',
      ...view.feeItems.flatMap((item) => [item.name, ...item.how, ...item.when, ...item.notes]),
      ...view.references.flatMap((reference) => [reference.kindLabel, ...reference.usedFor]),
    ];
    for (const sentence of sentences) {
      equal(/[A-Za-z]{2,}/.test(sentence.replace(allowed, '')), false, `「${sentence}」含英文枚举`);
    }
  }
});

// ---- 触发条件 ----

function surchargeDoc(condition: unknown, extra: Record<string, unknown> = {}): Record<string, any> {
  const plan = full1();
  plan.surchargeRules = [
    {
      id: 'R1',
      code: 'OVERSIZE_SURCHARGE',
      description: '',
      effect: 'ADD',
      condition,
      calculation: { method: 'FIXED_AMOUNT', amount: money('35') },
      exclusivity: 'STANDALONE',
      ...extra,
    },
  ];
  return plan;
}

const predicate = (source: string, operator: string, threshold: Record<string, unknown>) => ({
  kind: 'PREDICATE',
  predicate: { source, operator, ...threshold },
});

// Covers: 比较词与数值特征的单位——大于、不小于、小于、不大于；长度带厘米、体积带立方厘米、重量带千克。
test('条件：四个次序比较与三种量纲', () => {
  const when = (condition: unknown) => parsePriceCardContent(surchargeDoc(condition)).feeItems[1].when;
  deepEqual(when(predicate('SECOND_LONGEST_SIDE', 'GE', { lengthThreshold: { value: decimal('60'), unit: 'CM' } })), ['次长边不小于 60 厘米']);
  deepEqual(when(predicate('VOLUME', 'LT', { volumeThreshold: { value: decimal('5000'), unit: 'CM' } })), ['体积小于 5000 立方厘米']);
  deepEqual(when(predicate('VOLUMETRIC_WEIGHT', 'LE', { weightThreshold: weight('25', 1) })), ['体积重不大于 2.5 千克']);
  deepEqual(when(predicate('LENGTH_AND_GIRTH', 'GT', { lengthThreshold: { value: decimal('108'), unit: 'IN' } })), ['长加周长大于 108 英寸']);
});

// Covers: 「全部成立 / 任一成立」组合与嵌套；嵌套的组合加括号；同一类别特征的几个「等于」并成「属于」。
test('条件：全部成立用「且」、任一成立用「或」，嵌套加括号，同特征的等于并成属于', () => {
  const longest = predicate('LONGEST_SIDE', 'GT', { lengthThreshold: { value: decimal('100'), unit: 'CM' } });
  const heavy = predicate('ACTUAL_WEIGHT', 'GT', { weightThreshold: weight('20') });
  const tier = (value: string) => predicate('ADDRESS_TYPE', 'EQ', { category: value });
  const when = (condition: unknown) => parsePriceCardContent(surchargeDoc(condition)).feeItems[1].when[0];

  equal(when({ kind: 'ALL_OF', operands: [longest, heavy] }), '最长边大于 100 厘米，且实重大于 20 千克');
  equal(when({ kind: 'ANY_OF', operands: [longest, heavy] }), '最长边大于 100 厘米，或实重大于 20 千克');
  equal(
    when({ kind: 'ALL_OF', operands: [longest, { kind: 'ANY_OF', operands: [heavy, tier('T1')] }] }),
    '最长边大于 100 厘米，且（实重大于 20 千克，或偏远档位等于 T1）',
  );
  equal(when({ kind: 'ANY_OF', operands: [tier('T1'), tier('T2'), tier('T3')] }), '偏远档位属于 T1、T2、T3');
  // 不同特征的「等于」不能并。
  equal(
    when({ kind: 'ANY_OF', operands: [tier('T1'), predicate('ZONE', 'EQ', { category: 'Z2' })] }),
    '偏远档位等于 T1，或分区等于 Z2',
  );
  equal(when(predicate('ADDRESS_NATURE', 'EQ', { category: 'COMMERCIAL' })), '地址性质等于商业');
  equal(when(predicate('SERVICE_OPTION', 'EQ', { category: 'SIGNATURE' })), '服务选项等于 SIGNATURE');
});

// ---- 计算方法 ----

function oneRuleItem(calculation: unknown, rule: Record<string, unknown> = {}, plan: Record<string, any> = full1()): FeeItem {
  plan.surchargeRules = [
    {
      id: 'R1',
      code: 'OVERWEIGHT_SURCHARGE',
      description: '',
      effect: 'ADD',
      condition: predicate('ACTUAL_WEIGHT', 'GT', { weightThreshold: weight('0') }),
      calculation,
      exclusivity: 'STANDALONE',
      ...rule,
    },
  ];
  return parsePriceCardContent(plan).feeItems[1];
}

test('计算方法：百分比（固定百分比、序列再乘系数、全部费用为基数）', () => {
  deepEqual(oneRuleItem({ method: 'PERCENT_OF_BASIS', percentage: decimal('85', 1), basis: 'SYN-DEP-FULL-FUEL' }).how, [
    '按基础运费的 8.5% 收取',
  ]);
  deepEqual(
    oneRuleItem({ method: 'PERCENT_OF_BASIS', seriesKind: 'FUEL_RATE', seriesFactor: decimal('8', 1), basis: 'SYN-DEP-FULL-FUEL' }).how,
    ['按基础运费的百分比收取，百分比为燃油费率序列的当期费率，再乘 80%'],
  );
  const plan = full1();
  plan.chargeDependencies = [{ id: 'ALL', code: 'X', composition: 'ALL_CHARGES', excludes: ['FUEL_SURCHARGE'] }];
  deepEqual(oneRuleItem({ method: 'PERCENT_OF_BASIS', percentage: decimal('3'), basis: 'ALL' }, {}, plan).how, [
    '按其他已计收费用之和（不含燃油附加费）的 3% 收取',
  ]);
});

test('计算方法：取较大者、按期公布金额（窗外不收 / 待判断）', () => {
  deepEqual(
    oneRuleItem({
      method: 'GREATER_OF',
      operands: [
        { method: 'FIXED_AMOUNT', amount: money('1250', 2) },
        { method: 'PERCENT_OF_BASIS', percentage: decimal('10'), basis: 'SYN-DEP-FULL-FUEL' },
      ],
    }).how,
    ['下面几项取较大的一项：', '· 每件固定 12.50 元', '· 按基础运费的 10% 收取'],
  );
  deepEqual(oneRuleItem({ method: 'SERIES_AMOUNT', seriesId: 'SYN-SERIES-PSS-01', outOfWindow: 'NOT_CHARGED' }).how, [
    '按「SYN-SERIES-PSS-01」当期公布的金额收取',
    '当期没有公布金额时不收',
  ]);
  equal(oneRuleItem({ method: 'SERIES_AMOUNT', seriesId: 'S', outOfWindow: 'PENDING' }).how[1], '当期没有公布金额时先不出价，等公布后再判断');
});

test('计算方法：按分区查表、多重量段时分行写', () => {
  const table = {
    family: 'WEIGHT_ZONE',
    unit: 'KG',
    entries: [
      { id: 'a', zone: 'Z1', minimum: weight('0'), maximum: weight('2'), amount: money('5') },
      { id: 'b', zone: 'Z1', minimum: weight('2'), amount: money('9') },
      { id: 'c', zone: 'Z2', minimum: weight('0'), maximum: weight('2'), amount: money('7') },
      { id: 'd', zone: 'Z2', minimum: weight('2'), amount: money('11') },
    ],
  };
  deepEqual(oneRuleItem({ method: 'TABLE_LOOKUP', table }).how, [
    '按分区和重量段查价表',
    '分区 Z1：不足 2 千克收 5 元；2 千克及以上收 9 元',
    '分区 Z2：不足 2 千克收 7 元；2 千克及以上收 11 元',
  ]);
});

// Covers: 固定金额的计收单位与方向——按票聚合写「每票」、按件计收写「按件数累计」、减项写「减」并在备注标明。
test('固定金额：聚合口径、按件计收、减项', () => {
  const plan = full1();
  plan.aggregation = 'PER_SHIPMENT';
  const shipment = oneRuleItem({ method: 'FIXED_AMOUNT', amount: money('5') }, {}, plan);
  deepEqual(shipment.how, ['每票固定 5 元']);
  const perPiece = oneRuleItem({ method: 'FIXED_AMOUNT', amount: money('5') }, { unit: 'PER_PIECE' }, plan);
  deepEqual(perPiece.how, ['每件 5 元，按件数累计']);
  const deduct = oneRuleItem({ method: 'FIXED_AMOUNT', amount: money('2') }, { effect: 'DEDUCT' });
  deepEqual(deduct.how, ['每件减 2 元']);
  deepEqual(deduct.notes, ['减项：金额从合计里扣除']);
  equal(parsePriceCardContent(plan).header.aggregation, '按票聚合计价');
  deepEqual(parsePriceCardContent(plan).feeItems[0].when, ['每票都收']);
});

test('金额写法：整数不带小数、一位小数补到两位、其他币种写中文名或币种码', () => {
  const how = (amount: ReturnType<typeof money>) => oneRuleItem({ method: 'FIXED_AMOUNT', amount }).how[0];
  equal(how(money('100', 2)), '每件固定 1 元');
  equal(how(money('125', 1)), '每件固定 12.50 元');
  equal(how(money('1', 3)), '每件固定 0.001 元');
  equal(how(money('7', 0, 'USD')), '每件固定 7 美元');
  equal(how(money('7', 0, 'ZAR')), '每件固定 7 ZAR');
});

test('条件最低计价重量与拒收条款', () => {
  const item = oneRuleItem(
    { method: 'FIXED_AMOUNT', amount: money('5') },
    {
      conditionalMinimumWeight: {
        id: 'M1',
        condition: predicate('LONGEST_SIDE', 'GT', { lengthThreshold: { value: decimal('60'), unit: 'CM' } }),
        minimum: weight('2'),
      },
    },
  );
  deepEqual(item.notes, ['另有最低计价重量：最长边大于 60 厘米时，计价重量不低于 2 千克']);

  const plan = full1();
  plan.exclusions = [
    { id: 'X1', clause: '超长不收', condition: predicate('LONGEST_SIDE', 'GT', { lengthThreshold: { value: decimal('150'), unit: 'CM' } }) },
  ];
  deepEqual(parsePriceCardContent(plan).refusals, [{ clause: '超长不收', when: '最长边大于 150 厘米' }]);
});

test('总额上限与上下限两端', () => {
  const plan = full3();
  plan.totalChargeBounds = { minimum: money('12099', 2), maximum: money('500') };
  const view = parsePriceCardContent(plan);
  deepEqual(view.bounds, { minimum: '120.99 元', maximum: '500 元' });
  const maximum = itemNamed(view, '最高收费');
  deepEqual(maximum.how, ['合计高于 500 元时，超出的部分扣减，合计封顶 500 元']);
  deepEqual(maximum.when, ['各项费用合计高于 500 元时']);
  deepEqual(maximum.notes, ['减项：扣减的差额单独列一行']);
  deepEqual(view.feeItems.slice(-2).map((item) => item.name), ['最低收费', '最高收费']);
});

test('价表族：单价族与分段重量取整', () => {
  const plan = full1();
  plan.rateTable = {
    family: 'UNIT_PRICE',
    unit: 'KG',
    unitPrice: [{ id: 'u1', zone: 'Z1', amountPerUnit: money('32') }],
  };
  plan.weightPolicy = {
    method: 'ACTUAL_ONLY',
    rounding: [
      { mode: 'CEILING', increment: weight('1', 1), maximum: weight('5') },
      { mode: 'HALF_UP', increment: weight('5', 1), maximum: weight('20') },
      { mode: 'CEILING', increment: weight('1') },
    ],
  };
  const view = parsePriceCardContent(plan);
  deepEqual(view.feeItems[0].how, ['按计价重量乘单价计价', '分区 Z1：每千克 32 元']);
  equal(
    view.weightRule,
    '只按实重计价，不计体积重；计价重量不足 5 千克向上进位到 0.1 千克，5 千克起、不足 20 千克四舍五入到 0.5 千克，20 千克及以上向上进位到 1 千克。',
  );
});

// ---- 看不懂就降级 ----

// Covers: 读到的文档里有看不懂的结构（将来新增的计算方法）——只那一项降级成「暂不支持展示」并带原文，其余项照常，不丢项、不抛。
test('未知计算方法：只降级那一项，带原文，其余照常', () => {
  const plan = full1();
  const unknownRule = JSON.parse(JSON.stringify(plan.surchargeRules[0]));
  unknownRule.calculation = { method: 'TIERED_PERCENT', tiers: [1, 2] };
  plan.surchargeRules[0] = unknownRule;
  const view = parsePriceCardContent(plan);

  equal(view.feeItems.length, 6, '不丢项');
  const degraded = itemNamed(view, '燃油附加费（合成演示值）');
  equal(degraded.kind, 'unsupported');
  deepEqual(degraded.how, ['这一项暂不支持展示']);
  deepEqual(degraded.raw, unknownRule);
  equal(view.unsupportedCount, 1);
  equal(itemNamed(view, '超尺寸费（合成演示值）').kind, 'surcharge');
});

test('未知特征、未知比较、未知互斥立场、坏数值、未知价表族都只降级各自那一项', () => {
  const unknownFeature = parsePriceCardContent(surchargeDoc(predicate('DESTINATION_COUNTRY', 'EQ', { category: 'SG' })));
  equal(unknownFeature.feeItems[1].kind, 'unsupported');
  equal(unknownFeature.feeItems[0].kind, 'base');

  const unknownOperator = parsePriceCardContent(surchargeDoc(predicate('ACTUAL_WEIGHT', 'IN', { weightThreshold: weight('1') })));
  equal(unknownOperator.feeItems[1].kind, 'unsupported');

  const unknownStance = parsePriceCardContent(surchargeDoc(predicate('ACTUAL_WEIGHT', 'GT', { weightThreshold: weight('1') }), { exclusivity: 'SOMETIMES' }));
  equal(unknownStance.feeItems[1].kind, 'unsupported');

  // 数值特征比相等、类别特征比大小：领域构造期都拒，读到了说明文档来路不对，不硬翻。
  equal(parsePriceCardContent(surchargeDoc(predicate('ACTUAL_WEIGHT', 'EQ', { weightThreshold: weight('1') }))).feeItems[1].kind, 'unsupported');
  equal(parsePriceCardContent(surchargeDoc(predicate('ZONE', 'GT', { category: 'Z1' }))).feeItems[1].kind, 'unsupported');

  const badNumber = oneRuleItem({ method: 'FIXED_AMOUNT', amount: { amount: { coefficient: '1.5', scale: 0 }, currency: 'CNY' } });
  equal(badNumber.kind, 'unsupported');

  const plan = full1();
  plan.rateTable = { family: 'FLAT_FEE', unit: 'KG' };
  const unknownFamily = parsePriceCardContent(plan);
  equal(unknownFamily.feeItems[0].kind, 'unsupported');
  equal(unknownFamily.feeItems[0].name, '基础运费');
  equal(unknownFamily.feeItems.length, 6);
});

// Covers: 文档比这份代码新——出现本模块不认的顶层键（将来新增的规则种类）各占一行，带原文。
test('未知顶层键各占一行降级，带原文', () => {
  const plan = full3();
  plan.discountRules = [{ id: 'D1', percent: 5 }];
  const view = parsePriceCardContent(plan);
  const extra = view.feeItems[view.feeItems.length - 1];
  equal(extra.kind, 'unsupported');
  equal(extra.name, '未识别的内容');
  deepEqual(extra.raw, { discountRules: [{ id: 'D1', percent: 5 }] });
  equal(view.unsupportedCount, 1);
  equal(view.feeItems.length, 7);
});

test('重量策略、金额取整、总额上下限看不懂时各降级一格', () => {
  const plan = full3();
  plan.weightPolicy = { method: 'BLENDED', rounding: [] };
  plan.amountRounding = { mode: 'BANKERS', increment: money('1', 2), points: ['TOTAL'] };
  plan.totalChargeBounds = { minimum: { amount: { coefficient: 'x', scale: 0 }, currency: 'CNY' } };
  const view = parsePriceCardContent(plan);
  equal(view.weightRule, '重量规则暂不支持展示');
  equal(view.amountRounding, '金额取整暂不支持展示');
  deepEqual(view.bounds, { minimum: null, maximum: null });
  equal(view.unsupportedCount, 3);
  equal(view.feeItems.filter((item) => item.name === '最低收费 / 最高收费').length, 1);
});

test('文档不是对象时交回一份只带一条降级项和原文的空壳', () => {
  for (const document of [null, 'text', 42, [1, 2]]) {
    const view = parsePriceCardContent(document);
    equal(view.feeItems.length, 1);
    equal(view.feeItems[0].kind, 'unsupported');
    deepEqual(view.feeItems[0].raw, document);
    equal(view.unsupportedCount, 1);
    equal(view.weightRule, null);
  }
});

test('缺了键的残缺文档不抛：没有基础价表、没有规则', () => {
  const view = parsePriceCardContent({ reference: { id: 'P', version: 'v1' } });
  equal(view.header.planId, 'P');
  equal(view.feeItems[0].kind, 'unsupported');
  equal(view.references.length, 0);
});

// ---- 价格表列表的状态 ----

test('状态：退役 > 截止 > 未生效 > 已过期 > 生效中；截止点在将来的不算截止', () => {
  const now = new Date('2026-06-01T00:00:00Z');
  const base = { effectiveFrom: '2026-01-01T00:00:00Z' };
  equal(priceCardStatus(base, now), 'IN_FORCE');
  equal(priceCardStatus({ effectiveFrom: '2026-07-01T00:00:00Z' }, now), 'NOT_YET');
  equal(priceCardStatus({ ...base, effectiveTo: '2026-05-01T00:00:00Z' }, now), 'EXPIRED');
  equal(priceCardStatus({ ...base, effectiveTo: '2026-09-01T00:00:00Z' }, now), 'IN_FORCE');
  equal(priceCardStatus({ ...base, selectionEndsAt: '2026-05-15T00:00:00Z' }, now), 'CUT_OFF');
  equal(priceCardStatus({ ...base, selectionEndsAt: '2026-08-15T00:00:00Z' }, now), 'IN_FORCE');
  equal(priceCardStatus({ ...base, selectionEndsAt: '2026-05-15T00:00:00Z', retiredAt: '2026-05-20T00:00:00Z' }, now), 'RETIRED');
});
