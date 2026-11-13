import type { RegistrationResponseBody } from '../../components/registration';
import { postMasterData, type ApiResult } from '../catalogue-api';

/**
 * 承运商轨迹的手工录入与批量导入（dox/decisions.md「产品形态」）：没有对接承运商或轨迹聚合平台时，照承运商给的材料把
 * 轨迹录进来。先用登记面板接上——照提示填一份 JSON，一次一条或一批——不是引导式表单，也还没有 Excel 上传。
 */

export const trackingEntryEndpoint = '/transport-fulfillment-external-tracking-manual-entries';

export const trackingEntryTitle = '录入承运商轨迹';

export const trackingEntryHint =
  '请填 entries（数组，一次 1 到 200 条）。每条填：source（轨迹来源，与时间确认规则、里程碑映射里登记的同一个词）、credential（承运商运单号等外部承运凭证，' +
  '要先在「外部承运凭证」里登记到包裹上）、status（承运商的原始状态词，原样填，不用翻译）、occurredAt（承运商给的发生时间，RFC 3339；承运商没给就不填，' +
  '不要拿录入时间顶替）、original（所凭原件：邮件、截图或对账表的编号或存档引用，必填）。可选：eventId（承运商的事件号；不填时系统按内容派生一个，' +
  '同一条重复导入会被认出来）、carrier、correctionOf（要更正的那一条的 eventId，取答复里交回的那个）。' +
  '系统会在事件号前加 TRKMANUAL/，与接口来的轨迹分开。所属公司与录入人由系统按登录身份记下；需要持有「录入承运商轨迹」的授予。' +
  '录进来的轨迹和接口来的一样，要经过时间确认才进物流动态。';

/** 整批的答复只有一格；各条各自的答复在下面逐条列出。 */
export const trackingEntryOutcomeLabels: Record<string, string> = {
  ENTRIES_PROCESSED: '这一批已处理，各条的结果见下',
};

/** 各条答复的中文。 */
export const trackingEntryRowLabels: Record<string, string> = {
  TRACKING_FACT_ADOPTED: '已录入，时间已按规则确认',
  TRACKING_FACT_ADOPTED_PENDING_JUDGMENT: '已录入，等待时间确认',
  DUPLICATE_DELIVERY: '这一条已经录过',
  MATERIAL_LEFT_UNADOPTED: '没有录成事实，原因已记下',
  INPUT_NOT_ACCEPTED: '内容不完整或不成立，没有受理',
  ADOPTION_UNDECIDED: '暂时定不了，稍后重投整批即可',
  ANSWER_NOT_FORMED: '这一条没有形成答复，稍后重投整批即可',
};

export const trackingEntryReasonLabels: Record<string, string> = {
  CREDENTIAL_UNKNOWN: '这个承运凭证没有登记到任何包裹上',
  OCCURRED_AT_NOT_GIVEN_BY_SOURCE: '没有填发生时间',
  CREDENTIAL_REGISTRY_UNCONFIGURED: '还没有登记任何外部承运凭证',
};

interface TrackingEntryAnswer {
  index: number;
  outcome: string;
  reason?: string;
  eventId?: string;
  object?: string;
}

export interface TrackingEntriesResponseBody extends RegistrationResponseBody {
  counts?: Record<string, number>;
  entries?: TrackingEntryAnswer[];
}

/** 把逐条答复折成几行人读的话，按原顺序。 */
export function trackingEntryLines(body: TrackingEntriesResponseBody): string[] {
  return (body.entries ?? []).map((entry) => {
    const outcome = trackingEntryRowLabels[entry.outcome] ?? entry.outcome;
    const reason = entry.reason ? `（${trackingEntryReasonLabels[entry.reason] ?? entry.reason}）` : '';
    const object = entry.object ? `，包裹 ${entry.object}` : '';
    const event = entry.eventId ? `，事件号 ${entry.eventId}` : '';
    return `第 ${entry.index + 1} 条：${outcome}${reason}${object}${event}`;
  });
}

export function submitTrackingEntries(snapshot: unknown): Promise<ApiResult<TrackingEntriesResponseBody>> {
  return postMasterData<TrackingEntriesResponseBody>(trackingEntryEndpoint, snapshot);
}
