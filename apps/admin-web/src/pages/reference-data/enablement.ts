// 启用范围的页面纯逻辑（ADR-0345）。node:test 钉着；页面只负责摆。
//
// 启用范围是租户取值：没登记就是“未配置”，不套默认、不拦截。本轮只做登记、读回与展示——这一列说的是“本公司有没有登记过”，
// 不是“能不能用”：未登记的国家和币种照常可用。

import { labelFor } from '../../vocabulary/status';
import type {
  ApiResult,
  EnablementAnswerBody,
  EnablementKind,
  EnablementRecord,
  EnablementRegistrationPayload,
  EnablementStanding,
} from './api';

/** 页头那一句：说清“没登记不等于被限制”。 */
export const enablementScopeNote = '未登记的国家和币种不受限制，登记后才显示启用状态。';

/** 此刻状态的显示词；集外取值原样显示。 */
export function enablementLabel(standing: string): string {
  return labelFor(standing);
}

/** 读回的行按码索引。 */
export function enablementByCode(records: readonly EnablementRecord[]): Map<string, EnablementRecord> {
  return new Map(records.map((record) => [record.code, record]));
}

/** 某个码此刻的状态：册上没有行（或没有生效的行）就是未配置。 */
export function standingOf(byCode: ReadonlyMap<string, EnablementRecord>, code: string): EnablementStanding {
  return byCode.get(code)?.standing ?? 'NOT_CONFIGURED';
}

/** 这一行能做的动作：已启用的只能停用，其余（已停用、未配置）都是启用。 */
export type EnablementAction = 'ENABLE' | 'DISABLE';

export function actionFor(standing: EnablementStanding): EnablementAction {
  return standing === 'ENABLED' ? 'DISABLE' : 'ENABLE';
}

export function actionLabel(action: EnablementAction): string {
  return action === 'ENABLE' ? '启用' : '停用';
}

/**
 * 下一笔登记的载荷。修订号接册上最新的；生效时点取“此刻”与“册上最晚的生效时点”中较晚的一个——登记册不收早于前一笔的生效时点，
 * 本机的钟若比上一位登记者的慢，直接取本机此刻会被拒。来源单据去首尾空白，空就不带。
 */
export function nextRegistration(
  kind: EnablementKind,
  code: string,
  record: EnablementRecord | undefined,
  action: EnablementAction,
  source: string,
  now: Date,
): EnablementRegistrationPayload {
  const latestEffective = (record?.history ?? []).reduce<number>(
    (latest, row) => Math.max(latest, Date.parse(row.effectiveFrom)),
    Number.NEGATIVE_INFINITY,
  );
  const effective = new Date(Math.max(now.getTime(), Number.isNaN(latestEffective) ? now.getTime() : latestEffective));
  const trimmed = source.trim();
  return {
    kind,
    code,
    revision: (record?.latestRevision ?? 0) + 1,
    status: action === 'ENABLE' ? 'ENABLED' : 'DISABLED',
    effectiveFrom: effective.toISOString(),
    ...(trimmed === '' ? {} : { source: trimmed }),
  };
}

/** 来源单据的字数上限，与登记册的上限同一个数。 */
export const ENABLEMENT_SOURCE_LIMIT = 200;

/** 本地只拦来源单据过长；别的一律送上去让登记册答。 */
export function sourceProblem(source: string): string | null {
  return [...source.trim()].length > ENABLEMENT_SOURCE_LIMIT ? `来源单据最多 ${ENABLEMENT_SOURCE_LIMIT} 个字。` : null;
}

export type EnablementOutcomeNote = { landed: boolean; text: string };

const refusalText: Record<string, string> = {
  CODE_NOT_IN_CATALOGUE: '目录里没有这个码，请刷新页面后再试。',
  REVISION_OUT_OF_SEQUENCE: '这一行刚被别人改过，请刷新后再试。',
  EFFECTIVE_BEFORE_PRIOR: '生效时间早于上一次登记，请刷新后再试。',
  INPUT_NOT_WELL_FORMED: '填写的内容有误，请检查后再试。',
};

/**
 * 登记答复的人话。landed 为真表示“册上现在就是这一笔”（新登记，或同样的内容早已登过），页面据此关窗并重取；
 * 其余都不关窗，把原因摆在窗里。接入那几格（未配置、凭证被拒、未授予）和传输失败不在业务答复里，也走这里说人话。
 */
export function enablementOutcomeNote(answer: ApiResult<EnablementAnswerBody>, action: EnablementAction): EnablementOutcomeNote {
  const verb = actionLabel(action);
  switch (answer.kind) {
    case 'outcome': {
      const body = answer.body;
      if (body.outcome === 'REGISTERED') return { landed: true, text: `已${verb}。` };
      if (body.outcome === 'ALREADY_REGISTERED') return { landed: true, text: `这一次的${verb}早已登记过。` };
      if (body.outcome === 'CONTENT_CONFLICT') return { landed: false, text: '这一行刚被别人用不同的内容登记过，请刷新后再试。' };
      return { landed: false, text: refusalText[body.refusalReason ?? ''] ?? '没有登记成功，请刷新后再试。' };
    }
    case 'unconfigured':
      return { landed: false, text: '登记功能还没有开通，请联系管理员。' };
    case 'loginRequired':
      return { landed: false, text: '登录已过期，请重新登录后再试。' };
    case 'notGranted':
      return { landed: false, text: '你没有登记启用范围的权限，请联系管理员。' };
    case 'callerProblem':
      return { landed: false, text: answer.detail ?? '提交的内容有误，请检查后再试。' };
    case 'noAnswer':
      return { landed: false, text: '系统暂时没有给出答复，这一次是否登记上了还不清楚，请刷新后确认。' };
    case 'transport':
      return { landed: false, text: '没有连上系统，请稍后再试。' };
  }
}

/** 动作的确认语：说清楚这一步只登记、不拦别处。 */
export function confirmMessage(action: EnablementAction, name: string): string {
  return action === 'ENABLE'
    ? `把${name}登记为本公司启用。这只是登记，不影响其他国家和币种的使用。`
    : `把${name}登记为本公司停用。这只是登记，目前不会阻止任何业务使用它。`;
}

/** 列表上的状态格怎么显示：没读到时不假装“未配置”。 */
export type StandingCell = { kind: 'standing'; standing: EnablementStanding } | { kind: 'loading' } | { kind: 'unreadable' };

export function standingCell(
  answer: ApiResult<{ enablements: EnablementRecord[] }> | null,
  byCode: ReadonlyMap<string, EnablementRecord>,
  code: string,
): StandingCell {
  if (answer === null) return { kind: 'loading' };
  if (answer.kind !== 'outcome') return { kind: 'unreadable' };
  return { kind: 'standing', standing: standingOf(byCode, code) };
}
