// 有效时间判断页的判读与行转写（票 label-channel/21）。
//
// 单独成模块而不写在页面里：这一页的全部风险都在「每一格该显示什么」上——空册与未配置分不分得开、
// 「已按同值判过」是答案还是失败、待判断行会不会被填上一个不存在的有效时间——而页面组件要跑起来
// 得有 React 与 DOM。判读是纯函数就能拿测试钉住（同 handover-scope-summary 的先例）。

import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
import type {
  EffectiveTimeJudgmentResponseBody,
  ExternalTrackingFactListResponseBody,
  ExternalTrackingFactView,
} from './effective-time-judgment-api';
import { labelOf } from './presentation';

/** 有效时间依据封闭三格（transportfulfillment domain EffectiveTimeBasis），中文取 CONTEXT 原词。 */
export const effectiveBasisLabels: Record<string, string> = {
  PENDING: '待判断',
  JUDGED_EXPLICITLY: '所有者显式判断',
  JUDGED_BY_RULE: '按已登记规则判断',
};

/** 版本来源封闭两格（domain VersionOrigin）：素材到达形成的版本 / 一次判断形成的版本。 */
export const versionOriginLabels: Record<string, string> = {
  MATERIAL: '素材到达',
  JUDGMENT: '判断形成',
};

/** 判断口结果代数四格（application EffectiveTimeJudgmentOutcome）。 */
export const judgmentOutcomeLabels: Record<string, string> = {
  EFFECTIVE_TIME_JUDGED: '已判断——新版本回指前版，已交 visibility-exception',
  ALREADY_JUDGED_AS_GIVEN: '已按同值判过——交回既有判断版本，不再落新版',
  INPUT_NOT_ACCEPTED: '未受理——事实不存在于本租户，或事实引用 / 有效时间不成形',
  JUDGMENT_UNDECIDED: '未决——依赖故障，判断与否未知',
};

/** 未决成因（application TrackingAdoptionUndecidedReason 里判断会用到的两格）。 */
export const judgmentUndecidedReasonLabels: Record<string, string> = {
  TRACKING_FACT_REGISTRY_UNAVAILABLE: '事实登记册读不回来',
  TRACKING_IDENTITY_UNAVAILABLE: '版本签发不可用',
};

export interface FactRow {
  key: string;
  /** 行动作「判断」要指名的事实引用，与呈现列分开持有。 */
  fact: string;
  version: string;
  values: Readonly<Record<string, string>>;
}

/**
 * 当前版转写成一行。有效时间与依据明细在待判断行上是 `—` 与空串，**不是**发生时间或接收时间
 * ——「默认等于发生时间」在页面上也不许出现（ADR-0102 Alternatives 第二条）。
 */
export function factRowsOf(body: ExternalTrackingFactListResponseBody): FactRow[] {
  return body.facts.map((record) => ({
    key: `fact:${record.fact}@${record.version}`,
    fact: record.fact,
    version: record.version,
    values: {
      fact: record.fact,
      version: record.version,
      object: record.object,
      credential: record.credential,
      sourceEvent: record.sourceEvent ?? '—',
      status: record.status,
      occurredAt: formatInstant(record.occurredAt),
      receivedAt: formatInstant(record.receivedAt),
      effectiveBasis: labelOf(effectiveBasisLabels, record.effectiveBasis),
      effectiveAt: record.effectiveAt ? formatInstant(record.effectiveAt) : '—',
      basisDetail:
        record.effectiveRule && record.effectiveRuleVersion
          ? `${record.effectiveRule}@${record.effectiveRuleVersion}`
          : '',
      supersedes: record.supersedes ?? '',
      origin: labelOf(versionOriginLabels, record.origin),
    },
  }));
}

interface FactListViewOptions {
  module: ModuleInfo;
  endpoint: string;
}

/**
 * 列表区的呈现态。HTTP 那半（未配置 / 调用方问题 / 未形成答案 / 传输失败）转交 catalogueViewState，
 * 不在这里抄第二份；本函数只管：源未填不发请求、两种视图各自的空册措辞。
 *
 * 空册是真答案不是缺陷：待判断视图空说的是「这家源没有等着判的」（要么尚无被认领的轨迹，要么
 * 全部已判断），全部当前版视图空说的是「这家源还没有被认领的轨迹」——两句不同，与 403 的
 * 「接入渠道未配置」更是两件事。
 */
export function factListViewState(
  answer: ApiResult<ExternalTrackingFactListResponseBody> | null,
  source: string,
  view: ExternalTrackingFactView,
  rowCount: number,
  retry: () => void,
  options: FactListViewOptions,
): TemplateViewState {
  const trimmed = source.trim();
  if (trimmed === '') {
    return {
      kind: 'empty',
      title: '尚未指定轨迹源',
      description:
        '外部承运轨迹事实按（租户，轨迹源）上列——规则按源登记、回填按源重判，判断人也按源看；未指定源时页面不发请求。',
    };
  }
  const empty =
    view === 'pending'
      ? {
          title: `轨迹源 ${trimmed} 此刻没有待判断的事实`,
          description:
            '读取入口已配置，这一格是真答案：要么该源尚无被认领的轨迹，要么全部已判断（显式或按已登记规则）。切到「全部当前版」可看已判断的版本及其依据。',
        }
      : {
          title: `轨迹源 ${trimmed} 此刻没有任何当前版`,
          description:
            '读取入口已配置，这家源还没有被认领的外部承运轨迹事实——认领由收编执行器按源的拉取节拍进行，页面不含合成数据。',
        };
  return catalogueViewState(answer, rowCount, retry, {
    module: options.module,
    endpoint: options.endpoint,
    emptyTitle: empty.title,
    emptyDescription: empty.description,
  });
}

/**
 * 判断口答复的呈现。`tone` 分三色：`answered`（形成了的业务答案，含已判断与已按同值判过）、
 * `refused`（未受理，重试无用）、`undecided`（未决，可重试）；`landed` 只在新落了一版时为真。
 *
 * `已按同值判过`不是失败：它带回既有判断版本及其依据，正是票 21 红线要摆在面上的那一句——
 * 「这一条已经判过、按哪个依据判的」。details 逐行给出，页面原样列出不再加工。
 */
export interface JudgmentNote {
  tone: 'answered' | 'refused' | 'undecided';
  landed: boolean;
  canRetry: boolean;
  headline: string;
  details: string[];
}

export function describeJudgmentAnswer(body: EffectiveTimeJudgmentResponseBody): JudgmentNote {
  const headline = `判断口答复：${body.outcome} —— ${labelOf(judgmentOutcomeLabels, body.outcome)}`;
  switch (body.outcome) {
    case 'EFFECTIVE_TIME_JUDGED':
      return {
        tone: 'answered',
        landed: true,
        canRetry: false,
        headline,
        details: [
          `新版本 ${body.version ?? '—'} 回指 ${body.supersedes ?? '—'}`,
          basisLine(body),
          handoffLine(body),
        ],
      };
    case 'ALREADY_JUDGED_AS_GIVEN':
      return {
        tone: 'answered',
        landed: false,
        canRetry: false,
        headline,
        details: [
          `这一条已经判过：版本 ${body.version ?? '—'}，${basisLine(body)}`,
          ...(body.supersedes ? [`该版本回指 ${body.supersedes}`] : []),
          handoffLine(body),
        ],
      };
    case 'INPUT_NOT_ACCEPTED':
      return {
        tone: 'refused',
        landed: false,
        canRetry: false,
        headline,
        details: ['同样的输入重试仍是同一个答案：请核对事实引用是否属本租户、有效时间是否为 RFC 3339 时刻。'],
      };
    case 'JUDGMENT_UNDECIDED':
      return {
        tone: 'undecided',
        landed: false,
        canRetry: true,
        headline,
        details: [
          `${labelOf(judgmentUndecidedReasonLabels, body.undecidedReason ?? '')}；续办引用 ${body.continuationReference ?? '—'}。这不是「判断被拒」，重试可能得到答案。`,
        ],
      };
    default:
      // 未收录的 outcome 原样示出：服务端新增一格时，页面宁可显示英文原名，也不把它归进某个既有
      // 中文说法——那会让一种新答案冒充另一种。
      return { tone: 'answered', landed: false, canRetry: false, headline, details: [] };
  }
}

/** problem+json 错误码的中文说明。写口的 MALFORMED_REQUEST 说的是载荷形状，与查阅那句「查询参数」分开措辞。 */
export function judgmentProblemNote(code: string): string {
  switch (code) {
    case 'METHOD_NOT_ALLOWED':
      return '端点不接受当前 HTTP 方法；请检查前端与服务端版本是否一致。';
    case 'MALFORMED_REQUEST':
      return '载荷不是服务端接受的形状（只有事实引用与 RFC 3339 时刻两键，且不得带身份键）；请检查前端与服务端版本是否一致。';
    case 'INTAKE_FAILED':
      return '接入面未能形成操作者身份；服务端未读取判断内容。';
    case 'NO_ANSWER_FORMED':
      return '判断编排未能形成答案，可稍后重试。';
    default:
      return `服务端问题码：${code}`;
  }
}

function basisLine(body: EffectiveTimeJudgmentResponseBody): string {
  const basis = labelOf(effectiveBasisLabels, body.effectiveBasis ?? '');
  const at = body.effectiveAt ? `，有效时间 ${formatInstant(body.effectiveAt)}` : '';
  const rule =
    body.effectiveRule && body.effectiveRuleVersion
      ? `（规则 ${body.effectiveRule}@${body.effectiveRuleVersion}）`
      : '';
  return `依据：${basis}${rule}${at}`;
}

function handoffLine(body: EffectiveTimeJudgmentResponseBody): string {
  return body.handoffReference
    ? `意图尚未交出 visibility-exception，续办引用 ${body.handoffReference}；重放同一判断会重发。`
    : '意图已交 visibility-exception，客户可见面随派发进程更新。';
}
