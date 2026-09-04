// 逐字段登记表单的纯逻辑（票 pricing-reference-series-operations/08）：草稿形状、从目录行预填更正
// 草稿、本地能判的结构问题、草稿 → 载荷。全部是纯函数，node:test 钉着；组件只负责摆。
//
// **本文件不算摘要、不裁证据等级、不铸引用令牌、不判领域规则**（票 04 红线；MCP-3 裁决四条之四）。
// `draftProblems` 只拦「送上去必然被 400 拒」的结构缺格——空字段、不像十进制的取值、解不出的
// 时刻、汇率没选口径、更正没写依据——判据同复核面板那句：本地拦住不是替服务端判断，是不把一个
// 必然被拒的请求送上去，那个 400 会与治理答案挤在同一格里。期次重叠、无上界不在末期、更正不得
// 换序列身份之类的领域规则**不在这里重写**：那是领域构造门的活，重写一遍就是两处口径。

import type {
  ReferenceSeriesRecord,
  SeriesPeriodPayload,
  SeriesRegistrationPayload,
} from './api';

export type SeriesKindDraft = '' | 'FUEL_RATE' | 'EXCHANGE_RATE' | 'PUBLISHED_AMOUNT';

const seriesKinds: ReadonlyArray<Exclude<SeriesKindDraft, ''>> = ['FUEL_RATE', 'EXCHANGE_RATE', 'PUBLISHED_AMOUNT'];

function seriesKindOf(raw: string): SeriesKindDraft {
  return (seriesKinds as ReadonlyArray<string>).includes(raw) ? (raw as SeriesKindDraft) : '';
}

export interface PeriodDraft {
  startsAt: string;
  endsAt: string;
  value: string;
  evidenceRef: string;
}

export interface QuoteBasisDraft {
  policyId: string;
  policyVersion: string;
}

export interface CorrectionDraft {
  priorVersion: string;
  /** 前版的内容摘要（目录行透出的 contentDigest），作回指的指纹带回（ADR-0108 Decision 五）。 */
  priorFingerprint: string;
  basis: string;
}

export interface SeriesDraft {
  seriesId: string;
  seriesVersion: string;
  kind: SeriesKindDraft;
  sourceIdentifier: string;
  quoteBasis: QuoteBasisDraft | null;
  /** 只对 PUBLISHED_AMOUNT 有意义：每期金额的币种（三位大写代码），由领域构造门判与方案一致。 */
  currency: string;
  periods: PeriodDraft[];
  correction: CorrectionDraft | null;
  /** 预览时指名的对照版本；空即不指名（更正版本由服务端默认对它回指的那一版）。 */
  compareWithVersion: string;
}

export function emptyPeriodDraft(): PeriodDraft {
  return { startsAt: '', endsAt: '', value: '', evidenceRef: '' };
}

export function emptySeriesDraft(): SeriesDraft {
  return {
    seriesId: '',
    seriesVersion: '',
    kind: '',
    sourceIdentifier: '',
    quoteBasis: null,
    currency: '',
    periods: [emptyPeriodDraft()],
    correction: null,
    compareWithVersion: '',
  };
}

/**
 * 「更正此版本」的预填：同一条序列、同一种类、同一来源与口径，**该版全部期次**照抄进草稿，更正
 * 回指自动带上（版本号 + 前版内容摘要作指纹），更正依据留空强制人填，新版本号留空强制人
 * 起——更正是新版本，不是改旧版本，页面上因此没有「编辑」。对照版本默认就是被更正的那一版。
 */
export function correctionDraftOf(record: ReferenceSeriesRecord): SeriesDraft {
  return {
    seriesId: record.seriesId,
    seriesVersion: '',
    kind: seriesKindOf(record.kind),
    sourceIdentifier: record.sourceIdentifier,
    quoteBasis:
      record.quoteBasisId && record.quoteBasisVersion
        ? { policyId: record.quoteBasisId, policyVersion: record.quoteBasisVersion }
        : null,
    // 目录行今天不透出币种，金额序列的更正草稿这一格留空强制人填——留空会被 draftProblems 拦住，不会静默送空。
    currency: '',
    periods: record.periods.map((period) => ({
      startsAt: period.startsAt,
      endsAt: period.endsAt ?? '',
      value: period.value,
      evidenceRef: period.evidenceRef ?? '',
    })),
    correction: {
      priorVersion: record.seriesVersion,
      priorFingerprint: record.contentDigest,
      basis: '',
    },
    compareWithVersion: record.seriesVersion,
  };
}

// 取值只认十进制文本，不认指数记法——与服务端 ParseDecimal 的入口一致（那边会拒 1e3）。
const decimalText = /^[+-]?(\d+(\.\d+)?|\.\d+)$/;
const dateOnly = /^\d{4}-\d{2}-\d{2}$/;
// 与服务端 NewCurrency 的形状一致：三位大写字母；只拦形状，币种是否与方案一致由领域判。
const currencyCode = /^[A-Z]{3}$/;

/** 日期只填到天时补成当天零点 UTC 的 RFC 3339；其余原样交给服务端解。 */
export function normalizeMoment(raw: string): string {
  const text = raw.trim();
  return dateOnly.test(text) ? `${text}T00:00:00Z` : text;
}

function parsableMoment(raw: string): boolean {
  const text = normalizeMoment(raw);
  return text !== '' && !Number.isNaN(Date.parse(text));
}

/**
 * 本地能判的结构问题，一条一句；空数组即可送。**只拦必然 400 的缺格**，不重写领域规则
 * （见文件头）。
 */
export function draftProblems(draft: SeriesDraft): string[] {
  const problems: string[] = [];
  if (draft.seriesId.trim() === '') problems.push('序列标识未填');
  if (draft.seriesVersion.trim() === '') problems.push('版本号未填');
  if (draft.kind === '') problems.push('种类未选');
  if (draft.sourceIdentifier.trim() === '') problems.push('来源标识未填');
  if (draft.kind === 'EXCHANGE_RATE' && draft.quoteBasis === null) {
    problems.push('汇率必须选一版声明了口径的商业价格政策（不接受未声明口径的裸汇率）');
  }
  if (draft.kind === 'PUBLISHED_AMOUNT' && !currencyCode.test(draft.currency.trim())) {
    problems.push('按期公布金额的序列必须填币种（三位大写代码，与引用它的定价方案一致）');
  }
  if (draft.periods.length === 0) problems.push('至少要一期取值');
  draft.periods.forEach((period, index) => {
    const label = `第 ${index + 1} 期`;
    if (!parsableMoment(period.startsAt)) problems.push(`${label}起点不是可解的时刻（RFC 3339 或 YYYY-MM-DD）`);
    if (period.endsAt.trim() !== '' && !parsableMoment(period.endsAt)) {
      problems.push(`${label}止点不是可解的时刻（留空即无上界，只许末期）`);
    }
    if (!decimalText.test(period.value.trim())) problems.push(`${label}取值不是十进制文本（不接受指数记法）`);
  });
  if (draft.correction !== null) {
    if (draft.correction.priorVersion.trim() === '') problems.push('更正必须回指被更正的版本');
    if (draft.correction.basis.trim() === '') problems.push('更正依据未填（更正是新版本，凭什么更正要写清）');
    if (
      draft.correction.priorVersion.trim() !== '' &&
      draft.correction.priorVersion.trim() === draft.seriesVersion.trim()
    ) {
      problems.push('新版本号不能与被更正的版本相同');
    }
  }
  return problems;
}

function periodPayloadOf(period: PeriodDraft): SeriesPeriodPayload {
  const payload: SeriesPeriodPayload = {
    startsAt: normalizeMoment(period.startsAt),
    value: period.value.trim(),
  };
  if (period.endsAt.trim() !== '') payload.endsAt = normalizeMoment(period.endsAt);
  if (period.evidenceRef.trim() !== '') payload.evidenceRef = period.evidenceRef.trim();
  return payload;
}

/**
 * 草稿 → 产品定义的载荷。可缺的键缺席而不是空串：服务端按键在场与否分辨「没有」，空串会被当成
 * 一个填了空的值送进构造门。**载荷里没有身份**：租户与登记责任方由接入渠道的操作者信封给。
 */
export function payloadOf(draft: SeriesDraft): SeriesRegistrationPayload {
  const payload: SeriesRegistrationPayload = {
    seriesId: draft.seriesId.trim(),
    seriesVersion: draft.seriesVersion.trim(),
    kind: draft.kind,
    sourceIdentifier: draft.sourceIdentifier.trim(),
    periods: draft.periods.map(periodPayloadOf),
  };
  if (draft.quoteBasis !== null) {
    payload.quoteBasis = {
      policyId: draft.quoteBasis.policyId,
      policyVersion: draft.quoteBasis.policyVersion,
    };
  }
  if (draft.kind === 'PUBLISHED_AMOUNT' && draft.currency.trim() !== '') {
    payload.currency = draft.currency.trim();
  }
  if (draft.correction !== null) {
    payload.correction = {
      priorVersion: draft.correction.priorVersion.trim(),
      basis: draft.correction.basis.trim(),
    };
    if (draft.correction.priorFingerprint !== '') {
      payload.correction.priorFingerprint = draft.correction.priorFingerprint;
    }
  }
  if (draft.compareWithVersion.trim() !== '') {
    payload.compareWithVersion = draft.compareWithVersion.trim();
  }
  return payload;
}

/**
 * 载荷的稳定文本，用来判「预览过的是不是眼前这一份」：预览之后再改一格，预览就作废，登记按钮
 * 收回。它**不是摘要**，只是页面自己比对两份载荷用的键；内容摘要只有服务端算。
 */
export function payloadKey(payload: SeriesRegistrationPayload): string {
  return JSON.stringify(payload, Object.keys(flatten(payload)).sort());
}

function flatten(value: unknown, into: Record<string, true> = {}): Record<string, true> {
  if (Array.isArray(value)) {
    value.forEach((item) => flatten(item, into));
  } else if (value !== null && typeof value === 'object') {
    Object.entries(value as Record<string, unknown>).forEach(([key, nested]) => {
      into[key] = true;
      flatten(nested, into);
    });
  }
  return into;
}
