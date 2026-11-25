// 作业台的每一笔提交都经这里（纯 TypeScript，node:test 直接测；发请求的 sendWorkFact 把 postWorkFact 当参数）：
// 装报文、铸作业事实号、判后端的答复该算「成了」「没成别再送」还是「可以再送」。
//
// 报文不另写一套：装法是旧作业台页的那一份——事实号、仓、（该作业要的话）发生时刻与设备序号按 `workFactPrefillFor` 补，
// 再和工位填的格一起过 `buildGuidedPayload`，最后叠上定义里的固定载荷。所以同一个作业在两处装出来的报文逐键相同，后端用
// 封闭载荷解码，多一个键、错一个字都是 400。各工位另有测试把装出来的报文与定义钉在一起。
//
// 不会记两笔：事实号在**开始这一笔时**铸一次，之后不管送几次都是它。没送到（transport、noAnswer）时保留同一个 attempt
// 给「再送一次」；已经送到了而答复丢了的，重送后端答「已有结果」（EXISTING_RESULT），算成功。

import { accessRefusalCode, accessRefusalStatus, accessRefusalTitle, operatorRefusalMessage, type AccessRefusalKind } from '../../../auth/access-refusal';
import { postWorkFact, type ApiResult, type WorkFactHeaders } from '../../catalogue-api';
import { buildGuidedPayload, type GuidedFormValues } from '../guided-forms';
import { workFactOutcomeLabels, workFactPrefillFor, workFactProblemNote, type WorkFactSpec } from '../work-fact';
import { advanceDeviceSequence, readDeviceSequence, type StorageLike } from './device-binding';

export interface WorkFactAttempt {
  /** 这一笔的作业事实号。 */
  factId: string;
  endpoint: string;
  /** 已装好的报文；重送原样再送。 */
  payload: Record<string, unknown>;
  headers: WorkFactHeaders;
  /** 铸这一笔占下的本机设备序号。 */
  sequence: number;
}

export interface PrepareInput {
  spec: WorkFactSpec;
  deviceId: string;
  /** 设备绑的仓：既是报文里的仓，也是请求头里的作业范围引用。 */
  warehouse: string;
  /** 这一笔用的设备序号。 */
  sequence: number;
  /** 工位自己填的格（任务号、数量、库位这类）；事实号、仓、发生时刻、设备序号由这里按定义补。 */
  values: GuidedFormValues;
  now?: Date;
}

export type PrepareResult = { ok: true; attempt: WorkFactAttempt } | { ok: false; reason: string };

/** 装一笔：与旧作业台页同一份做法，装不出来（某一格不合）时说是哪一格。 */
export function prepareWorkFact(input: PrepareInput): PrepareResult {
  const { spec, deviceId, warehouse, sequence, values, now } = input;
  const identity = workFactPrefillFor(spec, deviceId, warehouse, sequence, now);
  const built = buildGuidedPayload(spec.form, { ...identity, ...values });
  if (!built.ok) {
    return { ok: false, reason: built.formError ?? Object.values(built.errors)[0] ?? '这一笔的内容装不出来' };
  }
  return {
    ok: true,
    attempt: {
      factId: String(identity.factId),
      endpoint: spec.endpoint,
      payload: { ...(spec.fixedPayload ?? {}), ...built.payload },
      headers: { deviceId, scopeKind: 'WAREHOUSE', scopeReference: warehouse },
      sequence,
    },
  };
}

/**
 * 占下这一笔的设备序号并装好。每铸一笔都把本机序号推进一格（即使这个作业的报文里不带设备序号）：事实号是
 * `PDA-设备-时刻到秒-序号`，旧页只在带设备序号的作业提交后才推进序号，同一台设备一秒里连铸两笔不带序号的作业会得到
 * 同一个事实号；而库存流水按（原因，来源事实号）判重，第二笔拣货会被当成第一笔的重放、货没挪。后端不要求序号连续，
 * 所以多推几格、留下空号不要紧。
 *
 * `floor` 是这个页面里上一笔用过的序号：存储读不了、写不了时本机序号永远从头数，靠它保证同一次会话里不重号。
 */
export function mintWorkFact(
  store: StorageLike | null,
  input: Omit<PrepareInput, 'sequence'>,
  floor = 0,
): PrepareResult {
  const current = readDeviceSequence(store, input.deviceId);
  const sequence = advanceDeviceSequence(store, input.deviceId, Math.max(current, floor) + 1);
  return prepareWorkFact({ ...input, sequence });
}

type WorkFactBody = { outcome?: string };

export type WorkFactPoster = (path: string, payload: unknown, headers: WorkFactHeaders) => Promise<ApiResult<WorkFactBody>>;

/** 送出一笔。重送就是对同一个 attempt 再调一次。 */
export function sendWorkFact(attempt: WorkFactAttempt, post: WorkFactPoster = postWorkFact): Promise<ApiResult<WorkFactBody>> {
  return post(attempt.endpoint, attempt.payload, attempt.headers);
}

/**
 * 答复里「这笔事实已经记下了（这一次或更早以前）」的那些业务答复，名字取自 `workFactOutcomeLabels`，逐个对过 Go 端
 * `WaveOutcome.String()` 与各登记口的答复名：
 * EXISTING_RESULT 是同一个事实号同样内容的重放，说明更早的那次已经到了，算成功。
 * 复核数量不符（CHECK_MISMATCH）与指令留在待处理（PENDING）也是「记下了」：事实在册，只是业务上没往前走，由用它的工位另说。
 * 打包之后提交委托没成的四个词（OWNERSHIP_UNRESOLVED 这一类）不在这里：答到它们，工位上这一笔还没走完，要给「再送一次」；
 * 用到它们的工位自己分（`packCommissionWords`）。
 */
const recordedOutcomes: ReadonlySet<string> = new Set([
  'PICKED',
  'RECORDED',
  'CHECKED',
  'CHECK_MISMATCH',
  'PACKED',
  'SUBMITTED',
  'CONFIRMED',
  'DIVERTED_TO_UNCLAIMED',
  'REGISTERED',
  'SHORTAGE_REGISTERED',
  'POSTED',
  'PENDING',
  'COUNTED',
  'UNCLAIMED',
  'EXISTING_RESULT',
]);

export function outcomeRecorded(outcome: string): boolean {
  return recordedOutcomes.has(outcome);
}

export interface WorkFactVerdict {
  /**
   * done：成了。refused：没成，重送不会好（业务上不收、接入被拒、报文不对），别再送。retry：没送到或后端没能处理，
   * 结果不确定，可以再送——用同一个事实号。
   */
  verdict: 'done' | 'refused' | 'retry';
  /** 一句话说结果，界面上的字，不带英文码。 */
  headline: string;
  /** 业务答复的原名（EXISTING_RESULT 这类）；接入拒绝、问题码与传输失败没有。 */
  outcome?: string;
  /** 接入那一格被拒时是哪一格：需要登录的要给「重新登录」。 */
  access?: AccessRefusalKind;
  /** 只进折叠的技术详情，不放在明面上。 */
  technical?: { status?: number; code?: string; message?: string };
}

/** 接入未配置的那句：口径取自 access-refusal.ts 里 `AccessRefusal` 对 `unconfigured` 一格的说明——「这一口没接上渠道或部署没设发行方参数，要配部署的人去配」。 */
const unconfiguredSentence = `${accessRefusalTitle('unconfigured')}：这一口没接上渠道，或部署没设发行方参数，要部署方去配；重试不会改变结果。`;

/**
 * 把后端对一笔作业事实的答复判成三类。`labels` 默认是 `workFactOutcomeLabels`；某个工位对个别答复有更贴切的说法时传
 * 自己的表（没列的退回默认），哪些答复算「成了」不随它变——那是后端语义，不是措辞。
 *
 * 接入拒绝与问题码按现有的说法：未配置、需要登录、没有授予取 access-refusal.ts 的词，别的问题码取 `workFactProblemNote`。
 * 它们都是重送不会好的（授予、登录、配置要人去办，办好之后由人再发起一笔）。
 */
export function judgeWorkFactAnswer(
  answer: ApiResult<WorkFactBody>,
  labels: Readonly<Record<string, string>> = workFactOutcomeLabels,
): WorkFactVerdict {
  switch (answer.kind) {
    case 'outcome': {
      const outcome = answer.body?.outcome ?? '';
      const label: string | undefined = labels[outcome] ?? workFactOutcomeLabels[outcome];
      if (recordedOutcomes.has(outcome)) return { verdict: 'done', outcome, headline: label ?? '已记下' };
      return label === undefined
        ? { verdict: 'refused', outcome, headline: '后端答了一个这里不认识的结果，这一笔没有当成功。', technical: { code: outcome } }
        : { verdict: 'refused', outcome, headline: label };
    }
    case 'unconfigured':
      return {
        verdict: 'refused',
        access: 'unconfigured',
        headline: unconfiguredSentence,
        technical: { status: accessRefusalStatus('unconfigured'), code: accessRefusalCode('unconfigured') },
      };
    case 'loginRequired':
    case 'notGranted':
      return {
        verdict: 'refused',
        access: answer.kind,
        headline: operatorRefusalMessage(answer.kind),
        technical: { status: accessRefusalStatus(answer.kind), code: accessRefusalCode(answer.kind) },
      };
    case 'callerProblem':
      return {
        verdict: 'refused',
        headline: workFactProblemNote(answer.code),
        technical: { status: answer.status, code: answer.code },
      };
    case 'noAnswer':
      return {
        verdict: 'retry',
        headline: '系统暂时没能处理这一笔，结果还不确定。点「再送一次」：用的是同一个作业事实号，不会记两笔。',
        technical: { status: answer.status, code: answer.code },
      };
    case 'transport':
      return {
        verdict: 'retry',
        headline: '这一笔没有送到：网络没通，或系统没有答。点「再送一次」：用的是同一个作业事实号，不会记两笔。',
        technical: { message: answer.message },
      };
  }
}

/** 一次读口没问成的原因，一句人话；读商品资料、读出库单这类读口用。 */
export function describeReadFailure(answer: Exclude<ApiResult<unknown>, { kind: 'outcome' }>): string {
  switch (answer.kind) {
    case 'unconfigured':
    case 'loginRequired':
    case 'notGranted':
      return accessRefusalTitle(answer.kind);
    case 'callerProblem':
      return workFactProblemNote(answer.code);
    case 'noAnswer':
      return '系统暂时没答出来';
    case 'transport':
      return `网络没通（${answer.message}）`;
  }
}
