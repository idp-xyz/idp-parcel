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
  PENDING: '待确认',
  JUDGED_EXPLICITLY: '人工确认',
  JUDGED_BY_RULE: '按规则自动确认',
};

/** 版本来源封闭两格（domain VersionOrigin）：素材到达形成的版本 / 一次判断形成的版本。 */
export const versionOriginLabels: Record<string, string> = {
  MATERIAL: '承运商回传',
  JUDGMENT: '确认形成',
};

/** 判断口结果代数四格（application EffectiveTimeJudgmentOutcome）。 */
export const judgmentOutcomeLabels: Record<string, string> = {
  EFFECTIVE_TIME_JUDGED: '已确认起算时间',
  ALREADY_JUDGED_AS_GIVEN: '这一条已经按同一时间确认过，没有重复记录',
  INPUT_NOT_ACCEPTED: '没有确认：找不到这条轨迹，或时间格式不对',
  JUDGMENT_UNDECIDED: '结果还不确定：系统暂时出了问题，可稍后重试',
};

/** 未决成因（application TrackingAdoptionUndecidedReason 里判断会用到的两格）。 */
export const judgmentUndecidedReasonLabels: Record<string, string> = {
  TRACKING_FACT_REGISTRY_UNAVAILABLE: '轨迹记录暂时读不出来',
  TRACKING_IDENTITY_UNAVAILABLE: '暂时无法生成新记录',
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
      title: '先选一个轨迹来源',
      description: '承运商回传的轨迹按来源分开查看：在上方输入轨迹来源（如承运商的接口编码），再点「查询」。',
    };
  }
  const empty =
    view === 'pending'
      ? {
          title: `${trimmed} 现在没有待确认的轨迹`,
          description: '要么这个来源还没有回传轨迹，要么全部已确认（人工或按规则）。切到「全部轨迹」可以看已确认的那些。',
        }
      : {
          title: `${trimmed} 还没有回传任何轨迹`,
          description: '系统会按约定的频率向这个来源拉取轨迹，拉到后会出现在这里。请核对来源编码是否写对。',
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
  // 英文结果码不上屏：查得到的显中文，查不到的原样露码（labelOf 的约定），原码进不进技术详情由页面定。
  const headline = labelOf(judgmentOutcomeLabels, body.outcome);
  switch (body.outcome) {
    case 'EFFECTIVE_TIME_JUDGED':
      return {
        tone: 'answered',
        landed: true,
        canRetry: false,
        headline,
        details: [
          `新记录 ${body.version ?? '—'} 接续 ${body.supersedes ?? '—'}`,
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
          `这一条已经确认过：记录 ${body.version ?? '—'}，${basisLine(body)}`,
          ...(body.supersedes ? [`接续 ${body.supersedes}`] : []),
          handoffLine(body),
        ],
      };
    case 'INPUT_NOT_ACCEPTED':
      return {
        tone: 'refused',
        landed: false,
        canRetry: false,
        headline,
        details: ['原样重试还是同一个结果：请核对轨迹编号是否抄对，时间是否写成 2026-09-04T06:05:00Z 这样的格式。'],
      };
    case 'JUDGMENT_UNDECIDED':
      return {
        tone: 'undecided',
        landed: false,
        canRetry: true,
        headline,
        details: [
          `${labelOf(judgmentUndecidedReasonLabels, body.undecidedReason ?? '')}；这不是被拒，稍后重试即可（续办引用 ${body.continuationReference ?? '—'}）。`,
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
      return '这个入口不接受这种提交方式，请刷新页面后重试；仍然不行请联系系统管理员。';
    case 'MALFORMED_REQUEST':
      return '提交的内容系统读不懂（只需要轨迹编号和时间两项）；请刷新页面后重试，仍然不行请联系系统管理员。';
    case 'INTAKE_FAILED':
      return '登录信息没能核对通过，这次什么都没有保存；请稍后重试。';
    case 'NO_ANSWER_FORMED':
      return '系统暂时没能处理，可稍后重试。';
    default:
      return `这次没有成功（错误码 ${code}）；请记下错误码联系系统管理员。`;
  }
}

function basisLine(body: EffectiveTimeJudgmentResponseBody): string {
  const basis = labelOf(effectiveBasisLabels, body.effectiveBasis ?? '');
  const at = body.effectiveAt ? `，有效时间 ${formatInstant(body.effectiveAt)}` : '';
  const rule =
    body.effectiveRule && body.effectiveRuleVersion
      ? `（规则 ${body.effectiveRule}@${body.effectiveRuleVersion}）`
      : '';
  return `确认方式：${basis}${rule}${at}`;
}

function handoffLine(body: EffectiveTimeJudgmentResponseBody): string {
  return body.handoffReference
    ? `还没同步到物流动态（续办引用 ${body.handoffReference}）；重新提交同一确认会再发一次。`
    : '已同步到物流动态，客户看到的轨迹会随之更新。';
}
