import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { PreAcceptanceEstimateRecord, PreAcceptanceOccupationRecord } from '../settlement/api';
import { getPreAcceptanceEstimates } from '../settlement/api';
import {
  estimateBlockStateOf,
  estimateSectionOf,
  estimateSectionsOf,
  NOT_RECORDED_DETAIL_NOTE,
  occupationViewOf,
} from './pre-acceptance-estimate';

// 本文件钉运单详情「受理前财务控制」一块里估价那一半（ADR-0379 决定二、四，票 declared-goods-lines/19 重钉的形状）：逐提交版本
// 一段、补记之前的旧行没有逐件明细、占用按种类各一套状态、两种金额写法分开、没有留痕时把财务控制那一步的停因说出来。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const freeze = (overrides: Partial<PreAcceptanceOccupationRecord> = {}): PreAcceptanceOccupationRecord => ({
  kind: 'FUNDS_FREEZE',
  id: 'FRZ-0001',
  status: 'HELD',
  amount: '153600',
  currency: 'CNY',
  formedAt: '2026-11-11T02:00:00Z',
  convertedAmount: '0',
  ...overrides,
});

const estimate = (overrides: Partial<PreAcceptanceEstimateRecord> = {}): PreAcceptanceEstimateRecord => ({
  submissionVersion: 'SUBV-2',
  form: 'PER_PARCEL_SELL_TRIAL_SUM',
  pricingScope: 'SYN-PRICING-SCOPE-R1',
  priceCard: { plan: 'SYN-SELL-FBA', version: 'v1', fingerprint: 'sha256:abc' },
  serviceProduct: 'SYN-PROD-FBA',
  detailRecorded: true,
  totalAmount: '153600',
  currency: 'CNY',
  trialAt: '2026-11-11T01:59:00Z',
  recordedAt: '2026-11-11T02:00:00Z',
  parcels: [
    {
      parcel: 'DPCL-1',
      evaluation: 'EVAL-1',
      totalAmount: '38400',
      currency: 'CNY',
      pricingWeight: { value: '12.5', unit: 'KG' },
      chargeLines: [
        { code: 'BASE_FREIGHT', effect: 'ADD', amount: '400.00', currency: 'CNY' },
        { code: 'FUEL_SURCHARGE', effect: 'DEDUCT', amount: '16.00', currency: 'CNY' },
      ],
    },
  ],
  occupations: [freeze()],
  ...overrides,
});

test('一版估价摊成一段：合计按最小币单位写，费用行照原样带小数与币种，价卡写成方案@版本', () => {
  const section = estimateSectionOf(estimate(), 'SUBV-2');
  equal(section.isCurrent, true);
  equal(section.versionLabel, '当前版本');
  equal(section.total, '153,600 CNY');
  equal(section.form, '逐件售价试算合计');
  deepEqual(section.priceCard, { plan: 'SYN-SELL-FBA', version: 'v1', text: 'SYN-SELL-FBA@v1' });
  equal(section.serviceProduct, 'SYN-PROD-FBA');
  equal(section.detailRecorded, true);
  equal(section.detailNote, undefined);
  deepEqual(section.parcels, [
    {
      parcel: 'DPCL-1',
      subtotal: '38,400 CNY',
      weight: '12.5 KG',
      charges: [
        { code: 'BASE_FREIGHT', label: '基础运费', effect: '加项', amount: '400.00 CNY' },
        { code: 'FUEL_SURCHARGE', label: '燃油附加费', effect: '减项', amount: '16.00 CNY' },
      ],
    },
  ]);
  equal(section.occupationNote, undefined);
  deepEqual(
    section.technical.map((row) => row.label),
    ['提交版本', '估价形态', '计价范围', '价卡内容指纹', '试算评价 · DPCL-1', '资金冻结标识'],
  );
});

test('两版估价按读口的次序逐版一段，只有当前版本标成当前', () => {
  const sections = estimateSectionsOf(
    [estimate({ submissionVersion: 'SUBV-1', trialAt: '2026-11-10T01:00:00Z' }), estimate({ submissionVersion: 'SUBV-2' })],
    'SUBV-2',
  );
  deepEqual(
    sections.map((section) => [section.submissionVersion, section.versionLabel]),
    [
      ['SUBV-1', '此前版本'],
      ['SUBV-2', '当前版本'],
    ],
  );
});

test('补记之前的旧行：写明没有逐件明细，逐件只剩小计；没申报服务产品不出这一格', () => {
  const section = estimateSectionOf(
    estimate({
      detailRecorded: false,
      serviceProduct: undefined,
      parcels: [{ parcel: 'DPCL-1', evaluation: 'EVAL-1', totalAmount: '38400', currency: 'CNY' }],
    }),
    'SUBV-2',
  );
  equal(section.detailRecorded, false);
  equal(section.detailNote, NOT_RECORDED_DETAIL_NOTE);
  equal(NOT_RECORDED_DETAIL_NOTE, '这份估价形成于补记之前，没有逐件明细');
  equal(section.serviceProduct, undefined);
  deepEqual(section.parcels, [{ parcel: 'DPCL-1', subtotal: '38,400 CNY', weight: '未记', charges: null }]);
});

test('受限没有占用：空数组写成一句，不写占了零', () => {
  const section = estimateSectionOf(estimate({ occupations: [] }), 'SUBV-2');
  deepEqual(section.occupations, []);
  equal(section.occupationNote, '没有占用：控制受限，没冻结资金也没占用信用');
});

test('占用：冻结中、信用占用中、已释放、已扣费各按种类的说法与时刻', () => {
  deepEqual(occupationViewOf(freeze()), {
    kind: '资金冻结',
    status: '资金冻结中',
    amount: '153,600 CNY',
    times: [{ label: '冻结时刻', at: '2026-11-11T02:00:00Z' }],
    summary: '资金冻结 · 资金冻结中 · 153,600 CNY',
  });
  deepEqual(
    occupationViewOf({
      kind: 'CREDIT_EXPOSURE',
      id: 'EXP-0001',
      status: 'RECORDED',
      amount: '153600',
      currency: 'CNY',
      formedAt: '2026-11-11T02:00:00Z',
    }).summary,
    '信用占用 · 信用占用中 · 153,600 CNY',
  );
  const released = occupationViewOf(freeze({ status: 'RELEASED', releasedAt: '2026-11-11T03:00:00Z' }));
  equal(released.status, '已释放');
  deepEqual(released.times, [
    { label: '冻结时刻', at: '2026-11-11T02:00:00Z' },
    { label: '释放时刻', at: '2026-11-11T03:00:00Z' },
  ]);
  const converted = occupationViewOf(
    freeze({ status: 'CONVERTED', convertedAmount: '153600', convertedAt: '2026-11-12T00:00:00Z' }),
  );
  equal(converted.status, '已扣费');
  equal(converted.summary, '资金冻结 · 已扣费 · 153,600 CNY · 已扣 153,600 CNY');
  deepEqual(converted.times.map((time) => time.label), ['冻结时刻', '扣费时刻']);
});

test('部分转扣费：状态仍是占用中，加一句部分已扣费并写出已扣多少', () => {
  const partial = occupationViewOf(
    freeze({ status: 'HELD', convertedAmount: '50000', convertedAt: '2026-11-12T00:00:00Z' }),
  );
  equal(partial.status, '资金冻结中，部分已扣费');
  equal(partial.converted, '50,000 CNY');
  const partialCredit = occupationViewOf({
    kind: 'CREDIT_EXPOSURE',
    id: 'EXP-0001',
    status: 'RECORDED',
    amount: '153600',
    currency: 'CNY',
    formedAt: '2026-11-11T02:00:00Z',
    convertedAmount: '100',
  });
  equal(partialCredit.status, '信用占用中，部分已扣费');
});

test('组合策略：冻结与信用占用同时在场，各出一行', () => {
  const section = estimateSectionOf(
    estimate({
      occupations: [
        freeze(),
        { kind: 'CREDIT_EXPOSURE', id: 'EXP-0002', status: 'RECORDED', amount: '20000', currency: 'CNY', formedAt: '2026-11-11T02:00:00Z' },
      ],
    }),
    'SUBV-2',
  );
  deepEqual(
    section.occupations.map((occupation) => occupation.kind),
    ['资金冻结', '信用占用'],
  );
});

test('认不得的形态、费用方向、占用种类与状态都照原码', () => {
  const section = estimateSectionOf(
    estimate({
      form: 'NEW_FORM',
      parcels: [
        {
          parcel: 'DPCL-1',
          evaluation: 'EVAL-1',
          totalAmount: '1',
          currency: 'CNY',
          pricingWeight: { value: '1', unit: 'KG' },
          chargeLines: [{ code: 'BASE_FREIGHT', effect: 'NEW_EFFECT', amount: '1.00', currency: 'CNY' }],
        },
      ],
      occupations: [{ kind: 'NEW_KIND', id: 'X', status: 'NEW_STATUS', amount: '1', currency: 'CNY', formedAt: '2026-11-11T02:00:00Z' }],
    }),
    'SUBV-2',
  );
  equal(section.form, 'NEW_FORM');
  equal(section.parcels[0]?.charges?.[0]?.effect, 'NEW_EFFECT');
  equal(section.occupations[0]?.summary, 'NEW_KIND · NEW_STATUS · 1 CNY');
});

test('一块的状态：读回之前在读；读到了照版本列', () => {
  deepEqual(estimateBlockStateOf(null, 'SUBV-2', undefined), { kind: 'loading' });
  const formed = estimateBlockStateOf({ kind: 'outcome', status: 200, body: { estimates: [estimate()] } }, 'SUBV-2', undefined);
  equal(formed.kind, 'formed');
  if (formed.kind === 'formed') {
    equal(formed.sections.length, 1);
    equal(formed.gapNote, undefined);
  }
});

test('没有留痕：停在财务控制时说出停因，`SA-` 与任务今天记的原因两种都认；停在别处或没停过只说还没有', () => {
  const empty = { kind: 'outcome' as const, status: 200, body: { estimates: [] } };
  deepEqual(estimateBlockStateOf(empty, 'SUBV-2', 'SA-ESTIMATE_PRICE_CARD_NOT_CONFIGURED'), {
    kind: 'none',
    note: '估价没形成，停在：没有在用价卡。',
  });
  deepEqual(estimateBlockStateOf(empty, 'SUBV-2', 'FINANCIAL_CONTROL_NOT_CONFIGURED'), {
    kind: 'none',
    note: '估价没形成，停在：财务控制缺一项登记，等运营登记。',
  });
  deepEqual(estimateBlockStateOf(empty, 'SUBV-2', 'SA-NEW_REASON'), { kind: 'none', note: '估价没形成，停在：SA-NEW_REASON。' });
  deepEqual(estimateBlockStateOf(empty, 'SUBV-2', 'MANUAL_REVIEW_PENDING'), { kind: 'none', note: '这张运单还没有受理前估价。' });
  deepEqual(estimateBlockStateOf(empty, 'SUBV-2', undefined), { kind: 'none', note: '这张运单还没有受理前估价。' });
});

test('有旧版本的估价而当前版本没有：说一句，别把旧版本读成现在的', () => {
  const answer = { kind: 'outcome' as const, status: 200, body: { estimates: [estimate({ submissionVersion: 'SUBV-1' })] } };
  const stalled = estimateBlockStateOf(answer, 'SUBV-2', 'SA-ESTIMATE_PRICE_CARD_CONFLICT');
  equal(stalled.kind === 'formed' ? stalled.gapNote : '', '当前版本的估价没形成，停在：价卡适用冲突。下面是此前版本的估价。');
  const pending = estimateBlockStateOf(answer, 'SUBV-2', undefined);
  equal(pending.kind === 'formed' ? pending.gapNote : '', '当前版本还没有估价，下面是此前版本的估价。');
});

test('结算那一口读不到：各格各说各的，可重试的只有没答与连不上', () => {
  const unconfigured = estimateBlockStateOf({ kind: 'unconfigured' }, 'SUBV-2', undefined);
  equal(unconfigured.kind === 'failed' && unconfigured.retryable, false);
  const refused = estimateBlockStateOf({ kind: 'notGranted' }, 'SUBV-2', undefined);
  equal(refused.kind === 'failed' && refused.text.startsWith('受理前估价没读到：'), true);
  const problem = estimateBlockStateOf({ kind: 'callerProblem', status: 404, code: 'NOT_FOUND' }, 'SUBV-2', undefined);
  equal(problem.kind === 'failed' && problem.retryable, false);
  deepEqual(problem.kind === 'failed' ? problem.technical : [], [
    { label: 'HTTP 状态', value: '404' },
    { label: '问题码', value: 'NOT_FOUND' },
  ]);
  const noAnswer = estimateBlockStateOf({ kind: 'noAnswer', status: 503, code: 'NO_ANSWER_FORMED' }, 'SUBV-2', undefined);
  equal(noAnswer.kind === 'failed' && noAnswer.retryable, true);
  const transport = estimateBlockStateOf({ kind: 'transport', message: 'fetch failed' }, 'SUBV-2', undefined);
  equal(transport.kind === 'failed' && transport.retryable, true);
});

test('读口按委托标识取，标识经地址编码', async () => {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify({ estimates: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } });
  }) as typeof fetch;
  const answer = await getPreAcceptanceEstimates('SHR-1&x=2');
  deepEqual(urls, ['/settlement-pre-acceptance-estimates?shipmentRequest=SHR-1%26x%3D2']);
  deepEqual(answer, { kind: 'outcome', status: 200, body: { estimates: [] } });
});
