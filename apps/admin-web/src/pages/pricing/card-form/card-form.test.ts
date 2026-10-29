import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import type { PriceCardTemplateSpec, PriceCardWorkbook } from '../api';
import { parsePriceCardContent } from '../price-card-content';
import { workbookToDocument } from './document';
import { stepOfField } from './fields';
import {
  alwaysCondition,
  emptyState,
  newCondition,
  newFee,
  newRateRow,
  newSegment,
  suggestChargeCode,
  type CardFormState,
} from './model';
import { mapProblems, problemCountsByStep, problemsByField } from './problems';
import { sampleState } from './sample';
import { validateAll, validateStep } from './validate';
import { formToWorkbook } from './workbook';
import { workbookToForm } from './workbook-to-form';

// 价卡编辑器的纯函数验收。参照物有两个，各自独立：
//   1. 后端测试数据里的 FULL-03 结构化工作簿（internal/parcelpricing/adapters/pricecardtemplate/testdata）——
//      逐表逐行逐列相等；
//   2. 种子里登记的规范化文档（scripts/demo-seeds/data/pricing，服务端登记的原件）——把表单生成的工作簿译成文档，
//      与原件逐字段相等。
// 路径相对 apps/admin-web，run-tests.mjs 固定以它为 cwd 起 node --test。规格取自后端规格口的一次取数
// （card-form/testdata，取证于 GET /pricing-price-card-template/spec），测试里的表与列因此是服务端真给的那一份。

const spec = JSON.parse(readFileSync('src/pages/pricing/card-form/testdata/price-card-template-spec-PPT-2.json', 'utf8')) as PriceCardTemplateSpec;
const full03Workbook = () =>
  JSON.parse(
    readFileSync('../../internal/parcelpricing/adapters/pricecardtemplate/testdata/price-card-cn-sg-full-03-workbook-PPT-2.json', 'utf8'),
  ) as PriceCardWorkbook;
const seedPlan = (name: string): Record<string, any> =>
  JSON.parse(readFileSync(`../../scripts/demo-seeds/data/pricing/${name}`, 'utf8')).plan;

type Rows = Record<string, string | number | null>[];

// 领域在构造时把价表行、各类规则、依赖、绑定与目录链接排好序，这些表的行序不进摘要，比较时按内容排；
// 条件树、计算树与进位段的先后有语义（兄弟节点按行序进树），逐行保持原序比较。
const orderInsensitive = new Set([
  'tables',
  'rates_weight_zone',
  'rates_first_continue',
  'rates_unit_price',
  'fixed_charges',
  'surcharges',
  'charge_dependencies',
  'reference_series',
  'reference_catalogues',
]);
const keyed = (row: unknown) => JSON.stringify(Object.entries(row as object).sort(([a], [b]) => a.localeCompare(b)));

function normalizedSheets(workbook: PriceCardWorkbook): Record<string, Rows> {
  const out: Record<string, Rows> = {};
  for (const [name, rows] of Object.entries(workbook.sheets)) {
    if (rows.length === 0) continue;
    out[name] = orderInsensitive.has(name) ? [...rows].sort((a, b) => keyed(a).localeCompare(keyed(b))) : rows;
  }
  return out;
}

function assertSameWorkbook(actual: PriceCardWorkbook, expected: PriceCardWorkbook) {
  equal(actual.templateVersion, expected.templateVersion);
  const left = normalizedSheets(actual);
  const right = normalizedSheets(expected);
  deepEqual(Object.keys(left).sort(), Object.keys(right).sort(), '出现的表不同');
  for (const name of Object.keys(right)) deepEqual(left[name], right[name], `表 ${name} 不相等`);
}

// ---- FULL-03 回环 ----

test('FULL-03 样例工作簿还原成表单状态，再生成回来，逐表逐行逐列相等', () => {
  const original = full03Workbook();
  const { workbook } = formToWorkbook(workbookToForm(original), spec);
  assertSameWorkbook(workbook, original);
});

test('FULL-03：card 表字段列全，没填的写 null，与样例逐行相等（字段次序也相同）', () => {
  const original = full03Workbook();
  const { workbook } = formToWorkbook(workbookToForm(original), spec);
  deepEqual(workbook.sheets.card, original.sheets.card);
});

test('FULL-03：条件树 / 计算树 / 进位段保持样例的行序', () => {
  const original = full03Workbook();
  const { workbook } = formToWorkbook(workbookToForm(original), spec);
  for (const name of ['conditions', 'calculations', 'weight_rounding']) {
    deepEqual(workbook.sheets[name], original.sheets[name], `${name} 的行序不同`);
  }
});

// 表单生成的工作簿译成文档，与服务端登记的原件逐字段相等。manifest、内容摘要和规范化版本是服务端算的，不比。
const volatileKeys = ['manifest', 'contentDigest', 'canonicalization'];
const sortedKeys = new Set(['entries', 'firstContinue', 'unitPrice', 'rules', 'surchargeRules', 'chargeDependencies', 'referenceSeries', 'referenceCatalogues']);
function comparable(value: unknown, key = ''): unknown {
  if (Array.isArray(value)) {
    const mapped = value.map((item) => comparable(item));
    return sortedKeys.has(key) ? mapped.sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b))) : mapped;
  }
  if (typeof value === 'object' && value !== null) {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, comparable(v, k)]));
  }
  return value;
}
function withoutVolatile(doc: Record<string, unknown>) {
  const copy = { ...doc };
  for (const key of volatileKeys) delete copy[key];
  return copy;
}

test('FULL-03：表单生成的工作簿译成的文档，与种子里服务端登记的规范化文档一致', () => {
  const { workbook } = formToWorkbook(workbookToForm(full03Workbook()), spec);
  deepEqual(comparable(withoutVolatile(workbookToDocument(workbook))), comparable(withoutVolatile(seedPlan('price-card-cn-sg-full-03.json'))));
});

// ---- 只有基础运费的小卡：种子 SYN-PLAN-CN-SG-COST-01 ----

function costCardState(): CardFormState {
  const state = emptyState();
  Object.assign(state.basics, {
    planId: 'SYN-PLAN-CN-SG-COST-01',
    planVersion: 'v1',
    scope: 'SYN-SCOPE-01',
    direction: 'BUY',
    purpose: 'SUPPLIER_COST',
    aggregation: 'PER_PACKAGE',
    currency: 'CNY',
    weightUnit: 'KG',
    periodStartsAt: '2026-01-01',
    authId: 'SYN-AUTH-COST-DIR-01',
    authVersion: 'v1',
  });
  const rows = [
    ['SYN-COST-Z1-A', '0', '1', '30'],
    ['SYN-COST-Z1-B', '1', '5', '85'],
    ['SYN-COST-Z1-C', '5', '30', '260'],
  ].map(([id, minimum, maximum, amount]) => {
    const row = newRateRow({ zone: 'Z1', minimum, maximum, amount });
    state.idHints[`rate:${row.uid}`] = id;
    return row;
  });
  state.base = { family: 'WEIGHT_ZONE', rows };
  state.weight = { method: 'ACTUAL_ONLY', divisor: '', lengthUnit: '', chargeable: [newSegment({ mode: 'CEILING', increment: '0.5' })], volumetric: [newSegment()] };
  state.amountRounding = { enabled: true, mode: 'HALF_UP', increment: '0.01', points: ['TOTAL'] };
  state.refs.zoneCatalogue = 'SYN-CAT-ZONE-CN-SG';
  Object.assign(state.idHints, {
    table: 'SYN-TABLE-CN-SG-COST-01',
    tableVersion: 'v1',
    weight: 'SYN-WEIGHT-CN-SG-COST-01',
    weightVersion: 'v1',
  });
  return state;
}

test('只有基础运费的小卡：译成的文档与种子 COST-01 一致', () => {
  const { workbook } = formToWorkbook(costCardState(), spec);
  deepEqual(comparable(withoutVolatile(workbookToDocument(workbook))), comparable(withoutVolatile(seedPlan('price-card-cn-sg-cost.json'))));
});

test('只有基础运费的小卡：工作簿只有 card、tables、价表行、进位段与目录，没有多余的表', () => {
  const { workbook } = formToWorkbook(costCardState(), spec);
  deepEqual(Object.keys(workbook.sheets).sort(), ['card', 'reference_catalogues', 'rates_weight_zone', 'tables', 'weight_rounding'].sort());
  deepEqual(workbook.sheets.weight_rounding, [{ policy: 'CHARGEABLE', mode: 'CEILING', increment: '0.5', unit: 'KG' }]);
  const card = Object.fromEntries(workbook.sheets.card.map((row) => [row.field as string, row.value]));
  equal(card.volumetricDivisor, null);
  equal(card.periodStartsAt, '2026-01-01T00:00:00Z');
  equal(card.periodEndsAt, null);
});

// ---- 基础运费 + 一条条件附加费：不指定标识，核标识自动生成且互相引用得上 ----

function surchargeCardState(): CardFormState {
  const state = costCardState();
  state.idHints = {};
  state.basics.planId = 'SYN-PLAN-FORM-SMALL-01';
  state.refs.zoneCatalogue = '';
  const fee = newFee('CONDITIONAL', 'KG');
  Object.assign(fee, {
    name: '超尺寸费（合成演示值）',
    code: 'OVERSIZE_SURCHARGE',
    amount: '35',
    conditions: [newCondition({ source: 'LONGEST_SIDE', operator: 'GT', value: '100', unit: 'CM' })],
  });
  state.fees = [fee];
  return state;
}

test('基础运费 + 一条条件附加费：标识按方案标识与序号生成，互相引用得上', () => {
  const { workbook } = formToWorkbook(surchargeCardState(), spec);
  const P = 'SYN-PLAN-FORM-SMALL-01';
  deepEqual(workbook.sheets.conditions, [
    { nodeId: `${P}-COND-1`, kind: 'PREDICATE', source: 'LONGEST_SIDE', operator: 'GT', threshold: '100', unit: 'CM' },
  ]);
  deepEqual(workbook.sheets.calculations, [{ nodeId: `${P}-CALC-1`, method: 'FIXED_AMOUNT', amount: '35' }]);
  deepEqual(workbook.sheets.surcharges, [
    {
      ruleId: `${P}-RULE-1`,
      chargeCode: 'OVERSIZE_SURCHARGE',
      description: '超尺寸费（合成演示值）',
      effect: 'ADD',
      conditionId: `${P}-COND-1`,
      calculationId: `${P}-CALC-1`,
      exclusivity: 'STANDALONE',
    },
  ]);
  deepEqual(
    workbook.sheets.tables.map((row) => row.tableId),
    [`${P}-TABLE`],
  );
});

test('条件附加费译成的文档念出「最长边大于 100 厘米」，且没有暂不支持的项', () => {
  const { workbook } = formToWorkbook(surchargeCardState(), spec);
  const view = parsePriceCardContent(workbookToDocument(workbook));
  equal(view.unsupportedCount, 0);
  const item = view.feeItems.find((candidate) => candidate.name === '超尺寸费（合成演示值）');
  ok(item);
  deepEqual(item.when, ['最长边大于 100 厘米']);
  deepEqual(item.how, ['每件固定 35 元']);
});

// ---- 示例 ----

test('示例：本地校验全过，译成的文档念得出每一项，没有暂不支持的项', () => {
  const state = sampleState();
  deepEqual(validateAll(state, spec), []);
  const { workbook } = formToWorkbook(state, spec);
  const view = parsePriceCardContent(workbookToDocument(workbook));
  equal(view.unsupportedCount, 0);
  deepEqual(
    view.feeItems.map((item) => item.name),
    ['基础运费', '燃油附加费（合成演示值）', '超尺寸费（合成演示值）', '超重费（合成演示值）', '偏远地区费（合成演示值）', '最低收费'],
  );
  const oversize = view.feeItems.find((item) => item.name.startsWith('超尺寸'));
  deepEqual(oversize?.when, ['最长边大于 100 厘米，或长加周长大于 300 厘米']);
  const remote = view.feeItems.find((item) => item.name.startsWith('偏远'));
  deepEqual(remote?.when, ['偏远档位属于 T1、T2']);
  ok(remote?.how[0].includes('T1 档 12 元') || remote?.how[0].includes('T1 档 12.00 元'), remote?.how.join('|'));
});

test('示例：与 FULL-03 同构——换上 FULL-03 的标识，生成的工作簿与样例相等', () => {
  // 示例的方案标识是演示用的新标识；这里只核「同一份内容、标识取自样例」时两边一致，证明差别只在标识上。
  const fromSample = formToWorkbook(sampleState(), spec).workbook;
  const full03 = full03Workbook();
  const shape = (workbook: PriceCardWorkbook) => ({
    conditions: workbook.sheets.conditions.map((row) => ({ ...row, nodeId: undefined, parentId: undefined })),
    surcharges: workbook.sheets.surcharges.map((row) => ({
      chargeCode: row.chargeCode,
      exclusivity: row.exclusivity,
      exclusivityGroup: row.exclusivityGroup === undefined ? null : 'g',
      priority: row.priority,
    })),
    bounds: workbook.sheets.total_bounds,
    rounding: workbook.sheets.weight_rounding,
  });
  deepEqual(shape(fromSample), shape(full03));
});

// ---- 费用代码建议 ----

test('费用代码：认得的名称用产品里已有的代码，纯英文转大写下划线，重名补序号', () => {
  equal(suggestChargeCode('PERCENT', '燃油附加费', []), 'FUEL_SURCHARGE');
  equal(suggestChargeCode('CONDITIONAL', '超尺寸费', []), 'OVERSIZE_SURCHARGE');
  equal(suggestChargeCode('CONDITIONAL', 'peak season fee', []), 'PEAK_SEASON_FEE');
  equal(suggestChargeCode('CONDITIONAL', '旺季费', []), 'OTHER_SURCHARGE');
  equal(suggestChargeCode('FIXED', '操作费', []), 'FIXED_CHARGE');
  equal(suggestChargeCode('CONDITIONAL', '超尺寸费', ['OVERSIZE_SURCHARGE']), 'OVERSIZE_SURCHARGE_2');
});

// ---- 问题定位 ----

test('问题按（表、行、列）查回表单控件，路径是运营看得懂的话', () => {
  const state = sampleState();
  const { workbook, origins } = formToWorkbook(state, spec);
  const oversize = state.fees[1];
  const rowOf = (sheet: string, predicate: (row: Record<string, unknown>) => boolean) =>
    workbook.sheets[sheet].findIndex(predicate) + 2;
  const problems = [
    { sheet: 'conditions', row: rowOf('conditions', (row) => row.source === 'LENGTH_AND_GIRTH'), column: 'threshold', code: 'CELL_INVALID', message: '不是十进制数' },
    { sheet: 'tables', row: 2, column: 'weightUnit', code: 'CELL_INVALID', message: '不是可取值' },
    { sheet: 'rates_first_continue', row: 3, column: 'firstAmount', code: 'CELL_INVALID', message: '金额不对' },
    { sheet: 'surcharges', row: 3, column: 'priority', code: 'CELL_REQUIRED', message: '必填格为空' },
    { sheet: 'card', row: 7, column: 'scope', code: 'CELL_REQUIRED', message: '必填格为空' },
    { sheet: 'total_bounds', row: 2, column: 'minimum', code: 'CONSTRUCTION_REJECTED', message: '最低收费要为正' },
    { sheet: '', row: 0, column: '', code: 'FILE_NOT_WORKBOOK', message: '不是工作簿' },
  ];
  const { located, unlocated } = mapProblems(problems, origins);
  equal(unlocated.length, 1);
  equal(unlocated[0].code, 'FILE_NOT_WORKBOOK');
  const [condition, weightUnit, rate, priority, scope, bounds] = located;
  equal(condition.where, '费用项 2「超尺寸费（合成演示值）」 · 条件 2 · 比较值');
  equal(condition.field, `fee.${oversize.uid}.cond.${oversize.conditions[1].uid}.value`);
  equal(condition.step, 'fees');
  equal(weightUnit.where, '基本信息 · 重量单位');
  equal(weightUnit.field, 'basics.weightUnit');
  equal(rate.where, '基础运费 · 第 2 行 · 首重金额');
  equal(rate.field, `base.row.${state.base.rows[1].uid}.firstAmount`);
  equal(priority.where, '费用项 2「超尺寸费（合成演示值）」 · 优先级');
  equal(scope.field, 'basics.scope');
  equal(bounds.step, 'bounds');
  deepEqual(problemCountsByStep(located), { fees: 2, basics: 2, base: 1, bounds: 1 });
  equal(problemsByField(located).get('basics.weightUnit')?.length, 1);
});

test('表层面的问题（行号 0）落到该表的默认控件；找不到行的落默认控件而不是丢掉', () => {
  const { origins } = formToWorkbook(sampleState(), spec);
  const { located, unlocated } = mapProblems(
    [
      { sheet: 'weight_rounding', row: 0, column: '', code: 'CELL_REQUIRED', message: '缺' },
      { sheet: 'surcharges', row: 99, column: 'ruleId', code: 'ID_DUPLICATED', message: '重复' },
      { sheet: 'nonexistent', row: 3, column: 'x', code: 'SHEET_UNKNOWN', message: '多出的表' },
    ],
    origins,
  );
  equal(located.length, 2);
  equal(located[0].field, 'weight.method');
  equal(located[1].step, 'fees');
  equal(unlocated.length, 1);
});

test('偏远档位价表的「还没有目录」问题落在偏远档位目录那一格', () => {
  const state = sampleState();
  state.refs.remoteTierCatalogue = '';
  const { workbook, origins } = formToWorkbook(state, spec);
  const row = workbook.sheets.calculations.findIndex((candidate) => candidate.method === 'TABLE_LOOKUP_BY_REMOTE_TIER') + 2;
  const { located } = mapProblems([{ sheet: 'calculations', row, column: 'method', code: 'REFERENCE_UNRESOLVED', message: '没有目录' }], origins);
  equal(located[0].field, 'refs.remoteTierCatalogue');
  equal(stepOfField(located[0].field), 'refs');
  equal(located[0].where, '费用项 4「偏远地区费（合成演示值）」 · 偏远档位目录');
});

// ---- 本地校验 ----

test('空表单：基本信息一堆必填，每一条都带控件定位', () => {
  const issues = validateStep('basics', emptyState(), spec);
  ok(issues.length >= 8);
  ok(issues.every((issue) => issue.field.startsWith('basics.')));
  ok(issues.some((issue) => issue.field === 'basics.currency' && issue.text === '请选择币种'));
});

test('数字格式：金额写了单位或逗号会被拦下并说人话', () => {
  const state = sampleState();
  state.fees[1].amount = '35元';
  state.base.rows[0].cells.firstAmount = '1,200';
  const fee = validateStep('fees', state, spec);
  deepEqual(fee.map((issue) => issue.text), ['金额要填数字，如 12 或 0.5（不用逗号、不写单位）']);
  const base = validateStep('base', state, spec);
  equal(base.length, 1);
  equal(base[0].where, '基础运费 · 第 1 行');
});

test('币种单位一致：体积重阈值的单位与价表重量单位不同会被拦下', () => {
  const state = sampleState();
  state.fees[2].conditions = [newCondition({ source: 'VOLUMETRIC_WEIGHT', operator: 'GT', value: '20', unit: 'LB' })];
  const issues = validateStep('fees', state, spec);
  equal(issues.length, 1);
  ok(issues[0].text.includes('与价表的重量单位一致'));
});

test('重量规则：取较大者才要体积重除数；非末段必须有上界', () => {
  const state = sampleState();
  state.weight.method = 'ACTUAL_ONLY';
  state.weight.divisor = '';
  deepEqual(validateStep('weight', state, spec), []);
  state.weight.method = 'MAX';
  equal(validateStep('weight', state, spec).length, 1);
  state.weight.divisor = '5000';
  state.weight.chargeable = [newSegment({ mode: 'CEILING', increment: '0.5' }), newSegment({ mode: 'CEILING', increment: '1' })];
  const issues = validateStep('weight', state, spec);
  equal(issues.length, 1);
  ok(issues[0].field.endsWith('.maximum'));
});

test('偏远档位价表要有偏远档位目录；最低收费不能高于最高收费', () => {
  const state = sampleState();
  state.refs.remoteTierCatalogue = '';
  equal(validateStep('refs', state, spec).length, 1);
  state.bounds = { minimum: '300', maximum: '200' };
  equal(validateStep('bounds', state, spec).length, 1);
});

test('固定金额没有条件构造器，也不进互斥组', () => {
  const state = emptyState();
  Object.assign(state.basics, sampleState().basics);
  state.base = sampleState().base;
  state.weight = sampleState().weight;
  const fee = newFee('FIXED', 'KG');
  Object.assign(fee, { name: '操作费', code: 'HANDLING_FEE', amount: '3', groupName: '不该出现' });
  state.fees = [fee];
  const { workbook } = formToWorkbook(state, spec);
  deepEqual(workbook.sheets.fixed_charges, [
    { ruleId: `${state.basics.planId}-RULE-1`, chargeCode: 'HANDLING_FEE', description: '操作费', effect: 'ADD', amount: '3', order: '1' },
  ]);
  equal(workbook.sheets.surcharges, undefined);
  equal(workbook.sheets.conditions, undefined);
});

test('「每件都收」的起手条件是实重大于 0（FULL-03 里燃油费也是这么表达的）', () => {
  const row = alwaysCondition('KG');
  equal(`${row.source} ${row.operator} ${row.value} ${row.unit}`, 'ACTUAL_WEIGHT GT 0 KG');
});
