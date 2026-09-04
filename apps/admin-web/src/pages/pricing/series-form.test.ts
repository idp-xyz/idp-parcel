import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import type { ReferenceSeriesRecord, SeriesRegistrationPayload } from './api';
import {
  correctionDraftOf,
  draftProblems,
  emptySeriesDraft,
  normalizeMoment,
  payloadKey,
  payloadOf,
  type SeriesDraft,
} from './series-form';

// 本文件钉的是表单纯逻辑的三条边界（票 pricing-reference-series-operations/08）：
//   1. 「更正此版本」预填**该版全部期次**并自动回指，新版本号与更正依据留空强制人填；
//   2. 本地校验只拦「送上去必然被 400 拒」的结构缺格，**不重写领域规则**——期次重叠之类的
//      在这里是零命中，那是领域构造门的活，重写一遍就是两处口径；
//   3. 草稿 → 载荷时可缺的键**缺席而不是空串**：服务端按键在场与否分辨「没有」。

function record(over: Partial<ReferenceSeriesRecord> = {}): ReferenceSeriesRecord {
  return {
    seriesId: 'SYN-FX-USD',
    seriesVersion: 'v3',
    kind: 'EXCHANGE_RATE',
    sourceIdentifier: 'cfets-daily',
    registrant: 'ops-a',
    quoteBasisId: 'PP-FX',
    quoteBasisVersion: '2',
    effectiveFrom: '2026-08-01T00:00:00Z',
    effectiveTo: '2026-08-03T00:00:00Z',
    evidenceGrade: 'VERIFIABLE',
    canonicalization: 'c14n-1',
    contentDigest: 'sha256:content',
    referenceDigest: '',
    registeredAt: '2026-08-01T00:00:00Z',
    reviewCount: 1,
    approvedReviewCount: 1,
    periods: [
      { startsAt: '2026-08-01T00:00:00Z', endsAt: '2026-08-02T00:00:00Z', value: '7.10', evidenceRef: 'ev-1' },
      { startsAt: '2026-08-02T00:00:00Z', endsAt: '2026-08-03T00:00:00Z', value: '7.11' },
    ],
    ...over,
  };
}

function completeDraft(over: Partial<SeriesDraft> = {}): SeriesDraft {
  return {
    seriesId: 'SYN-PRC-FUEL',
    seriesVersion: 'v1',
    kind: 'FUEL_RATE',
    sourceIdentifier: 'src-a',
    quoteBasis: null,
    currency: '',
    periods: [{ startsAt: '2026-08-01', endsAt: '', value: '0.12', evidenceRef: '' }],
    correction: null,
    compareWithVersion: '',
    ...over,
  };
}

// Covers: ADR-0110 Decision 一——按期公布金额的序列必须带币种；币种只在这一种上进载荷，费率序列不带。
test('published amount series carries a currency and only then', () => {
  ok(draftProblems(completeDraft({ kind: 'PUBLISHED_AMOUNT' })).some((line) => line.includes('币种')));
  ok(draftProblems(completeDraft({ kind: 'PUBLISHED_AMOUNT', currency: 'usd' })).some((line) => line.includes('币种')));
  deepEqual(draftProblems(completeDraft({ kind: 'PUBLISHED_AMOUNT', currency: 'USD' })), []);

  const payload = payloadOf(completeDraft({ kind: 'PUBLISHED_AMOUNT', currency: ' USD ' }));
  equal(payload.currency, 'USD');
  equal(payloadOf(completeDraft({ kind: 'FUEL_RATE', currency: 'USD' })).currency, undefined);
  equal(correctionDraftOf(record({ kind: 'PUBLISHED_AMOUNT' })).kind, 'PUBLISHED_AMOUNT');
});

// Covers: 预填抄全部期次（缺席的 endsAt / evidenceRef 变成空串好让输入框绑定），更正回指带
// 版本号与前版内容摘要作指纹（ADR-0108），对照版本默认就是被更正的那一版；新版本号与依据留空。
test('更正预填抄该版全部期次并自动回指，新版本号与依据留空强制人填', () => {
  const draft = correctionDraftOf(record());
  equal(draft.seriesId, 'SYN-FX-USD');
  equal(draft.seriesVersion, '');
  equal(draft.kind, 'EXCHANGE_RATE');
  equal(draft.sourceIdentifier, 'cfets-daily');
  deepEqual(draft.quoteBasis, { policyId: 'PP-FX', policyVersion: '2' });
  deepEqual(draft.periods, [
    { startsAt: '2026-08-01T00:00:00Z', endsAt: '2026-08-02T00:00:00Z', value: '7.10', evidenceRef: 'ev-1' },
    { startsAt: '2026-08-02T00:00:00Z', endsAt: '2026-08-03T00:00:00Z', value: '7.11', evidenceRef: '' },
  ]);
  deepEqual(draft.correction, {
    priorVersion: 'v3',
    priorFingerprint: 'sha256:content',
    basis: '',
  });
  equal(draft.compareWithVersion, 'v3');
});

// Covers: 目录行的种类不在封闭两格时不硬塞——留空让人重选，而不是把一个未知种类原样送回去。
test('更正预填遇到不认识的种类时留空而不硬塞', () => {
  const draft = correctionDraftOf(record({ kind: 'SOMETHING_NEW', quoteBasisId: undefined, quoteBasisVersion: undefined }));
  equal(draft.kind, '');
  equal(draft.quoteBasis, null);
});

// Covers: 空草稿的结构缺格逐条点名；完整草稿零问题。
test('空草稿逐条点名结构缺格，完整草稿零问题', () => {
  const problems = draftProblems(emptySeriesDraft());
  ok(problems.includes('序列标识未填'));
  ok(problems.includes('版本号未填'));
  ok(problems.includes('种类未选'));
  ok(problems.includes('来源标识未填'));
  ok(problems.some((line) => line.startsWith('第 1 期起点')));
  ok(problems.some((line) => line.startsWith('第 1 期取值')));
  deepEqual(draftProblems(completeDraft()), []);
});

// Covers: 汇率必须选口径（领域硬句「不接受未声明口径的裸汇率」的必然 400 那半），燃油不要口径。
test('汇率缺口径是结构缺格，燃油不要口径', () => {
  ok(draftProblems(completeDraft({ kind: 'EXCHANGE_RATE' })).some((line) => line.startsWith('汇率必须选')));
  deepEqual(
    draftProblems(completeDraft({ kind: 'EXCHANGE_RATE', quoteBasis: { policyId: 'PP-FX', policyVersion: '2' } })),
    [],
  );
  deepEqual(draftProblems(completeDraft({ kind: 'FUEL_RATE', quoteBasis: null })), []);
});

// Covers: 取值只认十进制文本；指数记法在服务端 ParseDecimal 入口会被拒，所以本地就拦。
test('取值拒指数记法与非数字，接受带符号与小数', () => {
  const bad = completeDraft({ periods: [{ startsAt: '2026-08-01', endsAt: '', value: '1e3', evidenceRef: '' }] });
  ok(draftProblems(bad).some((line) => line.includes('取值不是十进制文本')));
  const good = completeDraft({
    periods: [
      { startsAt: '2026-08-01', endsAt: '2026-08-02', value: '-0.5', evidenceRef: '' },
      { startsAt: '2026-08-02', endsAt: '', value: '.25', evidenceRef: '' },
    ],
  });
  deepEqual(draftProblems(good), []);
});

// Covers: 更正的两条结构缺格——回指为空、依据为空、新版本号与被更正版本相同。
test('更正必须回指、写依据、且新版本号不同于被更正版本', () => {
  const draft = completeDraft({
    seriesVersion: 'v3',
    correction: { priorVersion: 'v3', priorFingerprint: '', basis: '' },
  });
  const problems = draftProblems(draft);
  ok(problems.some((line) => line.startsWith('更正依据未填')));
  ok(problems.includes('新版本号不能与被更正的版本相同'));
  const noPrior = completeDraft({ correction: { priorVersion: '', priorFingerprint: '', basis: 'x' } });
  ok(draftProblems(noPrior).includes('更正必须回指被更正的版本'));
});

// Covers: 领域规则**不在本地重写**——两期重叠、无上界不在末期，本地都是零命中；那是领域构造门
// 要答的，前端答了就是两处口径（票 04 红线）。
test('期次重叠与无上界不在末期不是本地问题——那是领域构造门的活', () => {
  const overlapping = completeDraft({
    periods: [
      { startsAt: '2026-08-01', endsAt: '', value: '0.12', evidenceRef: '' },
      { startsAt: '2026-08-01', endsAt: '2026-08-05', value: '0.13', evidenceRef: '' },
    ],
  });
  deepEqual(draftProblems(overlapping), []);
});

// Covers: 只填到天补成当天零点 UTC；完整时刻原样；两端空白去掉。
test('日期只填到天时补成当天零点 UTC，其余原样交服务端', () => {
  equal(normalizeMoment(' 2026-08-01 '), '2026-08-01T00:00:00Z');
  equal(normalizeMoment('2026-08-01T08:00:00+08:00'), '2026-08-01T08:00:00+08:00');
});

// Covers: 可缺的键缺席而不是空串——服务端按键在场与否分辨「没有」，空串会被当成填了空的值。
test('草稿到载荷时可缺的键缺席而不是空串', () => {
  const payload = payloadOf(completeDraft());
  deepEqual(payload, {
    seriesId: 'SYN-PRC-FUEL',
    seriesVersion: 'v1',
    kind: 'FUEL_RATE',
    sourceIdentifier: 'src-a',
    periods: [{ startsAt: '2026-08-01T00:00:00Z', value: '0.12' }],
  });
  ok(!('quoteBasis' in payload));
  ok(!('correction' in payload));
  ok(!('compareWithVersion' in payload));
  ok(!('endsAt' in payload.periods[0]));
  ok(!('evidenceRef' in payload.periods[0]));
});

// Covers: 更正载荷带回指与前版内容摘要作指纹（有则原样带回，服务端不铸任何令牌顶替）；
// 口径与对照版本按有无在场；各格两端空白去掉。
test('更正载荷带回指与前版摘要作指纹，口径与对照版本按有无在场', () => {
  const draft = correctionDraftOf(record());
  draft.seriesVersion = ' v4 ';
  draft.correction!.basis = ' 8 月 2 日那期抄错了 ';
  const payload = payloadOf(draft);
  equal(payload.seriesVersion, 'v4');
  deepEqual(payload.quoteBasis, { policyId: 'PP-FX', policyVersion: '2' });
  deepEqual(payload.correction, {
    priorVersion: 'v3',
    priorFingerprint: 'sha256:content',
    basis: '8 月 2 日那期抄错了',
  });
  equal(payload.compareWithVersion, 'v3');

  const noDigest = payloadOf(
    completeDraft({ correction: { priorVersion: 'v1', priorFingerprint: '', basis: 'b' } }),
  );
  ok(!('priorFingerprint' in noDigest.correction!));
});

// Covers: 载荷键与键的书写顺序无关、与内容有关——预览之后改一格，键就变，登记按钮才收得回。
test('载荷键与书写顺序无关、与任何一格内容有关', () => {
  const a: SeriesRegistrationPayload = {
    seriesId: 'S',
    seriesVersion: 'v1',
    kind: 'FUEL_RATE',
    sourceIdentifier: 'src',
    periods: [{ startsAt: '2026-08-01T00:00:00Z', value: '0.1' }],
  };
  const reordered: SeriesRegistrationPayload = {
    periods: [{ value: '0.1', startsAt: '2026-08-01T00:00:00Z' }],
    sourceIdentifier: 'src',
    kind: 'FUEL_RATE',
    seriesVersion: 'v1',
    seriesId: 'S',
  };
  equal(payloadKey(a), payloadKey(reordered));
  const changed: SeriesRegistrationPayload = { ...a, periods: [{ startsAt: '2026-08-01T00:00:00Z', value: '0.2' }] };
  notEqual(payloadKey(a), payloadKey(changed));
});
