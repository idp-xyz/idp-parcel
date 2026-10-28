import type { TechnicalRow } from '../../components/TechnicalDetails';
import { operatorRefusalMessage } from '../../auth/access-refusal';
import { labelFor } from '../../vocabulary/status';
import type { ApiResult } from './api';

// 本区各写口页面给运营看的话。presentation.ts 的词表是用例原词(委托、未决、续办引用),委托查阅两页还在引它;
// 这里是屏上另一层:错误说哪里错、怎么改,不说「服务端」「不可覆盖」;状态码与问题码不进正文,由页面收进「技术详情」。
// 词典(src/vocabulary)落 main 后,这一层改为引它。

// 调用方问题(4xx)的人话。码本身进技术详情,这里只说下一步做什么。
const problemMessages: Record<string, string> = {
  MALFORMED_REQUEST: '填写的内容系统认不出来,请检查后再提交;原样重发不会有不同结果。',
  METHOD_NOT_ALLOWED: '这个入口不接受这种提交方式,请联系管理员。',
  INTAKE_FAILED: '这次没有提交成功,可以稍后再试。',
  NO_ANSWER_FORMED: '这次没有处理成功,可以稍后再试;重试不会重复建单。',
  UNNAMED_OUTCOME: '系统交回了认不出的结果,已按没有处理成功对待,可以稍后再试。',
  SHIPMENT_REQUEST_NOT_VISIBLE: '按填写的运单号、客户单号与包裹号找不到这票运单。请核对后再提交。',
  SHIPMENT_REQUEST_NOT_YET_ACCEPTED: '这票运单还没受理,改不了资料;受理前要改请走补充。',
  ACCESS_CHANNEL_NOT_CONFIGURED: '这台管理台还没接上操作者登录渠道,请部署的人配置后再试。',
};

export function problemMessage(code: string): string {
  return problemMessages[code] ?? '请检查填写内容后再试;问题码见技术详情。';
}

/** 一次答复的技术行:状态码与问题码。传输错误没有状态码,只记错误文本。 */
export function answerTechnicalRows<Body>(answer: ApiResult<Body>, outcomeCode?: string): TechnicalRow[] {
  switch (answer.kind) {
    case 'outcome':
      return [
        { label: 'HTTP 状态', value: String(answer.status) },
        { label: '结果码', value: outcomeCode ?? '' },
      ];
    case 'callerProblem':
    case 'noAnswer':
      return [
        { label: 'HTTP 状态', value: String(answer.status) },
        { label: '问题码', value: answer.code },
      ];
    case 'transport':
      return [{ label: '错误', value: answer.message }];
    case 'unconfigured':
      return [
        { label: 'HTTP 状态', value: '403' },
        { label: '问题码', value: 'ACCESS_CHANNEL_NOT_CONFIGURED' },
      ];
    case 'loginRequired':
      return [
        { label: 'HTTP 状态', value: '401' },
        { label: '问题码', value: 'OPERATOR_CREDENTIAL_REJECTED' },
      ];
    case 'notGranted':
      return [
        { label: 'HTTP 状态', value: '403' },
        { label: '问题码', value: 'OPERATOR_NOT_GRANTED' },
      ];
  }
}

// ---- 写动作之后给操作员看的一句话 ----

/**
 * 正文只说业务上怎么回事、下一步做什么;状态码与结果码收进 technical,由页面放进「技术详情」折叠区——
 * 报障或对日志时展开就有原名。`refused` 多带一格 kind,页面据它在「需要登录」时接一颗重新登录。
 */
export type OperatorNote =
  | { tone: 'unconfigured'; text: string; technical: TechnicalRow[] }
  | { tone: 'refused'; kind: 'loginRequired' | 'notGranted'; text: string; technical: TechnicalRow[] }
  | { tone: 'outcome'; text: string; technical: TechnicalRow[] }
  | { tone: 'problem'; text: string; technical: TechnicalRow[] };

/**
 * 命令口的共同答复格:非业务答案的几格各命令同形,业务答案由 outcomeText 按各自词表出;
 * callerProblemText 让各页对自己认得的 4xx(如「册上已没有这一笔」)另说一句,答 undefined 时落通用句。
 */
export function commandNoteOf<Body>(
  label: string,
  answer: ApiResult<Body>,
  outcomeText: (body: Body, status: number) => string,
  outcomeCode: (body: Body) => string,
  callerProblemText?: (problem: { status: number; code: string }) => string | undefined,
): OperatorNote {
  switch (answer.kind) {
    case 'outcome':
      return { tone: 'outcome', text: outcomeText(answer.body, answer.status), technical: answerTechnicalRows(answer, outcomeCode(answer.body)) };
    case 'unconfigured':
      return {
        tone: 'unconfigured',
        text: `「${label}」没有执行:${problemMessage('ACCESS_CHANNEL_NOT_CONFIGURED')}`,
        technical: answerTechnicalRows(answer),
      };
    case 'loginRequired':
    case 'notGranted':
      return {
        tone: 'refused',
        kind: answer.kind,
        text: operatorRefusalMessage(answer.kind, `「${label}」没有执行`),
        technical: answerTechnicalRows(answer),
      };
    case 'callerProblem': {
      const special = callerProblemText?.(answer);
      return { tone: 'problem', text: special ?? `「${label}」没有被受理:${problemMessage(answer.code)}`, technical: answerTechnicalRows(answer) };
    }
    case 'noAnswer':
      return { tone: 'problem', text: `「${label}」这次没有处理成功,可以稍后再试;重试不会重复登记。`, technical: answerTechnicalRows(answer) };
    default:
      return { tone: 'problem', text: `「${label}」没有送到服务器,请检查网络后再试。`, technical: answerTechnicalRows(answer) };
  }
}

// ---- 接受前复核页的两个命令 ----

/** 复核完成命令(ManualReviewCompletionOutcome)的人话。每格都是形成了的业务答案,不画成故障。 */
export function manualReviewOutcomeText(body: { outcome: string; decisionKind?: string; currentVersion?: string }): string {
  switch (body.outcome) {
    case 'RECORDED':
      return '复核已记录,系统会接着判断这票运单能不能受理。';
    case 'ALREADY_COMPLETED':
      return '这一版已经有人记录过复核了,这次没有再记。';
    case 'TASK_ALREADY_CLOSED':
      return `这票运单已经有结果了${body.decisionKind ? `(${labelFor(body.decisionKind)})` : ''},不再需要复核。`;
    case 'VERSION_SUPERSEDED':
      return '客户已提交了新版本,这一版的复核不再作数;请刷新后按新版本复核。';
    case 'REVISION_CONFLICT':
      return '刚才有别的操作同时在改这票运单,这次没有写入;请刷新后再试。';
    case 'NOT_AUTHORIZED':
      return '你没有复核这票运单的权限;需要时请管理员授予。';
    case 'AUTHORITY_RULES_NOT_CONFIGURED':
      return '复核的授权规则还没配置,系统不默认任何人可以复核;请管理员配置后再试,重试不会改变结果。';
    default:
      return '复核已处理,结果见技术详情。';
  }
}

/** 主动拒绝命令(ActiveRejectionOutcome)的人话。 */
export function activeRejectionOutcomeText(body: { outcome: string; decisionKind?: string }): string {
  switch (body.outcome) {
    case 'FORMED':
      return '已拒绝这票运单。';
    case 'NOT_AUTHORIZED':
      return '你没有拒绝这票运单的权限;需要时请管理员授予。';
    case 'UNDECIDED':
      return '这次没有形成决定,也不算拒绝;可以稍后再试。';
    case 'TASK_ALREADY_CLOSED':
      return `这票运单已经有结果了${body.decisionKind ? `(${labelFor(body.decisionKind)})` : ''},不能再拒绝。`;
    default:
      return '拒绝已处理,结果见技术详情。';
  }
}
