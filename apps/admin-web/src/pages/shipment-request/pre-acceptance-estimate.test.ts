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

// 本文件钉运单详情「受理前财务控制」一块里估价那一半（ADR-0379 决定二、四，票 declared-goods-lines/19 的形状）：逐提交版本
// 一段、新的在前、补记之前的旧行没有逐件明细、占用按种类各一套状态、两种金额写法分开、数组格是 null 也不炸，以及没有留痕时
// 只说数据说得出的话（不适用、停因、还没有）。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const current = { currentVersion: 'SUBV-2' };

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

const credit = (overrides: Partial<PreAcceptanceOccupationRecord> = {}): PreAcceptanceOccupationRecord => ({
  kind: 'CREDIT_EXPOSURE',
  id: 'EXP-0001',
  status: 'RECORDED',
  amount: '153600',
  currency: 'CNY',
  formedAt: '2026-11-11T02:00:00Z',
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

const listed = (estimates: unknown) => ({ kind: 'outcome' as const, status: 200, body: { estimates } as never });

test('一版估价摊成一段：合计按最小币单位写，费用行照原样带小数与币种，价卡写成方案@版本', () => {
  const section = estimateSectionOf(estimate(), current);
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

test('两版估价按试算时点新的在前，只有当前版本标成当前；时点读不出时退回读口次序的倒序', () => {
  const older = estimate({ submissionVersion: 'SUBV-1', trialAt: '2026-11-10T01:00:00Z' });
  const newer = estimate({ submissionVersion: 'SUBV-2', trialAt: '2026-11-11T01:59:00Z' });
  for (const order of [
    [older, newer],
    [newer, older],
  ]) {
    deepEqual(
      estimateSectionsOf(order, current).map((section) => [section.submissionVersion, section.versionLabel]),
      [
        ['SUBV-2', '当前版本'],
        ['SUBV-1', '此前版本'],
      ],
    );
  }
  deepEqual(
    estimateSectionsOf(
      [estimate({ submissionVersion: 'SUBV-1', trialAt: 'x' }), estimate({ submissionVersion: 'SUBV-2', trialAt: 'y' })],
      current,
    ).map((section) => section.submissionVersion),
    ['SUBV-2', 'SUBV-1'],
  );
});

test('补记之前的旧行：写明没有逐件明细，逐件只剩小计；没申报服务产品不出这一格', () => {
  const section = estimateSectionOf(
    estimate({
      detailRecorded: false,
      serviceProduct: undefined,
      parcels: [{ parcel: 'DPCL-1', evaluation: 'EVAL-1', totalAmount: '38400', currency: 'CNY' }],
    }),
    current,
  );
  equal(section.detailRecorded, false);
  equal(section.detailNote, NOT_RECORDED_DETAIL_NOTE);
  equal(NOT_RECORDED_DETAIL_NOTE, '这份估价形成于补记之前，没有逐件明细');
  equal(section.serviceProduct, undefined);
  deepEqual(section.parcels, [{ parcel: 'DPCL-1', subtotal: '38,400 CNY', weight: '未记', charges: null }]);
});

test('没有占用：只在当前版本且结论受限时说控制受限，其余只说没有占用', () => {
  const none = estimate({ occupations: [] });
  equal(estimateSectionOf(none, { currentVersion: 'SUBV-2', controlOutcome: 'RESTRICTED' }).occupationNote, '没有占用：控制受限');
  equal(estimateSectionOf(none, { currentVersion: 'SUBV-2', controlOutcome: 'HELD' }).occupationNote, '没有占用');
  equal(estimateSectionOf(none, { currentVersion: 'SUBV-2' }).occupationNote, '没有占用');
  equal(estimateSectionOf(none, { currentVersion: 'SUBV-3', controlOutcome: 'RESTRICTED' }).occupationNote, '没有占用');
  deepEqual(estimateSectionOf(none, current).occupations, []);
});

test('占用：冻结中、信用占用中、已释放、已扣费各按种类的说法与时刻，带上占用标识', () => {
  deepEqual(occupationViewOf(freeze()), {
    id: 'FRZ-0001',
    kind: '资金冻结',
    status: '资金冻结中',
    amount: '153,600 CNY',
    times: [{ label: '冻结时刻', at: '2026-11-11T02:00:00Z' }],
    summary: '资金冻结 · 资金冻结中 · 153,600 CNY',
  });
  equal(occupationViewOf(credit()).summary, '信用占用 · 信用占用中 · 153,600 CNY');
  const released = occupationViewOf(freeze({ status: 'RELEASED', releasedAt: '2026-11-11T03:00:00Z' }));
  equal(released.status, '已释放');
  deepEqual(released.times, [
    { label: '冻结时刻', at: '2026-11-11T02:00:00Z' },
    { label: '释放时刻', at: '2026-11-11T03:00:00Z' },
  ]);
  const converted = occupationViewOf(freeze({ status: 'CONVERTED', convertedAmount: '153600', convertedAt: '2026-11-12T00:00:00Z' }));
  equal(converted.status, '已扣费');
  equal(converted.summary, '资金冻结 · 已扣费 · 153,600 CNY · 已扣 153,600 CNY');
  deepEqual(converted.times.map((time) => time.label), ['冻结时刻', '扣费时刻']);
});

test('部分转扣费：状态仍是占用中，加一句部分已扣费并写出已扣多少', () => {
  const partial = occupationViewOf(freeze({ status: 'HELD', convertedAmount: '50000', convertedAt: '2026-11-12T00:00:00Z' }));
  equal(partial.status, '资金冻结中，部分已扣费');
  equal(partial.converted, '50,000 CNY');
  equal(occupationViewOf(credit({ convertedAmount: '100' })).status, '信用占用中，部分已扣费');
});

test('扣过一部分再放掉余下的：状态是已释放，不加部分已扣费，已扣多少照列', () => {
  const view = occupationViewOf(
    freeze({ status: 'RELEASED', convertedAmount: '50000', convertedAt: '2026-11-12T00:00:00Z', releasedAt: '2026-11-13T00:00:00Z' }),
  );
  equal(view.status, '已释放');
  equal(view.converted, '50,000 CNY');
  equal(view.summary, '资金冻结 · 已释放 · 153,600 CNY · 已扣 50,000 CNY');
  deepEqual(view.times.map((time) => time.label), ['冻结时刻', '释放时刻', '扣费时刻']);
});

test('组合策略：冻结与信用占用同时在场，各出一行、标识各自的', () => {
  const section = estimateSectionOf(estimate({ occupations: [freeze(), credit({ id: 'EXP-0002', amount: '20000' })] }), current);
  deepEqual(
    section.occupations.map((occupation) => [occupation.id, occupation.kind]),
    [
      ['FRZ-0001', '资金冻结'],
      ['EXP-0002', '信用占用'],
    ],
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
    current,
  );
  equal(section.form, 'NEW_FORM');
  equal(section.parcels[0]?.charges?.[0]?.effect, 'NEW_EFFECT');
  equal(section.occupations[0]?.summary, 'NEW_KIND · NEW_STATUS · 1 CNY');
});

test('Go 的 nil 切片编成 null：估价、逐件、费用行、占用几格是 null 也照常摊开，不抛错', () => {
  const nulls = {
    ...estimate(),
    parcels: [{ parcel: 'DPCL-1', evaluation: 'EVAL-1', totalAmount: '38400', currency: 'CNY', pricingWeight: null, chargeLines: null }],
    occupations: null,
  };
  const state = estimateBlockStateOf(listed([nulls]), { currentVersion: 'SUBV-2', controlOutcome: 'RESTRICTED' }, undefined);
  equal(state.kind, 'formed');
  if (state.kind === 'formed') {
    deepEqual(state.sections[0]?.parcels, [{ parcel: 'DPCL-1', subtotal: '38,400 CNY', weight: '未记', charges: null }]);
    deepEqual(state.sections[0]?.occupations, []);
    equal(state.sections[0]?.occupationNote, '没有占用：控制受限');
  }
  const noParcels = estimateBlockStateOf(listed([{ ...estimate(), parcels: null }]), current, undefined);
  equal(noParcels.kind === 'formed' ? noParcels.sections[0]?.parcels.length : -1, 0);
  deepEqual(estimateBlockStateOf(listed(null), current, undefined), { kind: 'none', note: '这张运单还没有受理前估价。' });
});

test('一块的状态：读回之前在读；读到了照版本列', () => {
  deepEqual(estimateBlockStateOf(null, current, undefined), { kind: 'loading' });
  const formed = estimateBlockStateOf(listed([estimate()]), current, undefined);
  equal(formed.kind, 'formed');
  if (formed.kind === 'formed') {
    equal(formed.sections.length, 1);
    equal(formed.gapNote, undefined);
  }
});

test('没有留痕：不适用说合同约定不做；停在财务控制时说停因，两族都认；停在别处或没停过只说还没有', () => {
  const empty = listed([]);
  deepEqual(estimateBlockStateOf(empty, { currentVersion: 'SUBV-2', controlOutcome: 'NOT_APPLICABLE' }, undefined), {
    kind: 'none',
    note: '合同约定无财务控制，不做估价。',
  });
  // 结论已判为不适用，任务上留着的旧停因不再作数。
  deepEqual(
    estimateBlockStateOf(empty, { currentVersion: 'SUBV-2', controlOutcome: 'NOT_APPLICABLE' }, 'FINANCIAL_CONTROL_NOT_FORMED'),
    { kind: 'none', note: '合同约定无财务控制，不做估价。' },
  );
  deepEqual(estimateBlockStateOf(empty, current, 'SA-ESTIMATE_PRICE_CARD_NOT_CONFIGURED'), {
    kind: 'none',
    note: '估价没形成，停在：没有在用价卡。',
  });
  deepEqual(estimateBlockStateOf(empty, current, 'FINANCIAL_CONTROL_NOT_CONFIGURED'), {
    kind: 'none',
    note: '估价没形成，停在：财务控制缺一项登记，等运营登记。',
  });
  deepEqual(estimateBlockStateOf(empty, current, 'SA-NEW_REASON'), { kind: 'none', note: '估价没形成，停在：SA-NEW_REASON。' });
  deepEqual(estimateBlockStateOf(empty, current, 'MANUAL_REVIEW_PENDING'), { kind: 'none', note: '这张运单还没有受理前估价。' });
  deepEqual(estimateBlockStateOf(empty, current, undefined), { kind: 'none', note: '这张运单还没有受理前估价。' });
});

test('有旧版本的估价而当前版本没有：说一句，别把旧版本读成现在的', () => {
  const answer = listed([estimate({ submissionVersion: 'SUBV-1' })]);
  const gap = (context: { currentVersion: string; controlOutcome?: string }, reason?: string) => {
    const state = estimateBlockStateOf(answer, context, reason);
    return state.kind === 'formed' ? state.gapNote : '';
  };
  equal(gap(current, 'SA-ESTIMATE_PRICE_CARD_CONFLICT'), '当前版本的估价没形成，停在：价卡适用冲突。下面是此前版本的估价。');
  equal(gap(current), '当前版本还没有估价，下面是此前版本的估价。');
  equal(
    gap({ currentVersion: 'SUBV-2', controlOutcome: 'NOT_APPLICABLE' }),
    '当前版本合同约定无财务控制，不做估价；下面是此前版本的估价。',
  );
});

test('结算那一口读不到：未配置不给重试，拒绝交给登录提示，调用方问题只说不重试，没答与连不上给重试', () => {
  equal(estimateBlockStateOf({ kind: 'unconfigured' }, current, undefined).kind, 'unconfigured');
  deepEqual(estimateBlockStateOf({ kind: 'loginRequired' }, current, undefined), {
    kind: 'refused',
    refusal: 'loginRequired',
    what: '受理前估价',
  });
  deepEqual(estimateBlockStateOf({ kind: 'callerProblem', status: 404, code: 'NOT_FOUND' }, current, undefined), {
    kind: 'problem',
    text: '受理前估价这次没有被受理，问题码见技术详情；运单其余部分不受影响。',
    technical: [
      { label: 'HTTP 状态', value: '404' },
      { label: '问题码', value: 'NOT_FOUND' },
    ],
  });
  equal(estimateBlockStateOf({ kind: 'noAnswer', status: 503, code: 'NO_ANSWER_FORMED' }, current, undefined).kind, 'unreachable');
  const transport = estimateBlockStateOf({ kind: 'transport', message: 'fetch failed' }, current, undefined);
  deepEqual(transport.kind === 'unreachable' ? transport.technical : [], [{ label: '错误', value: 'fetch failed' }]);
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
