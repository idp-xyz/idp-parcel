import type { ApiResult } from '../pages/catalogue-api';

/**
 * 写动作的三态文案（票 admin-web-workspace-form/05 第 2 条；蓝图 21.1「每个动作必须有反馈」那半，
 * 领域无关地搬过来）。一处算、各页消费，各写面的「进行中 / 已答 / 失败」才不会各造一套说法。
 *
 * **动词取调用点已有的按钮文字**（撤回 / 提交 / 取消 / 拒绝…），不在这里内置词表：动作叫什么归各页，
 * 这里只负责把它嵌进三句固定句式。
 *
 * **`answered` 不是「业务上成了」。** 结果代数里 `outcome` 只说服务端形成了答案（ADR-0022：状态码只答
 * 「有没有形成答案」，业务判别在响应体），而答案可以是负向的——`NOT_AUTHORIZED` 照样是一个答案。
 * 所以「已<动词>」这一句只配说「这个动作已送达并得到答复」，业务答案由页面拿自己的词表渲；消费方要是把
 * 它当成「撤回成功」显示，负向答案那一刻就说了假话。这也是本仓写面全部行内渲结果代数、不拿一句通用
 * 成功语顶替的原因（各 `AnswerNote` 的头注）。
 *
 * **失败文案不改写**：`callerProblem.detail` 是服务端随 4xx 交回的理由散文，原样交出；传输错交 `message`
 * 原文。两者都缺时退到能查得到的标识（错误码 + 状态码 / 固定一句「未到达」），不替服务端补一句它没说的话。
 */
export type ActionFeedback =
  | { state: 'pending'; text: string }
  | { state: 'answered'; text: string }
  | { state: 'failed'; text: string };

/** 进行中那一句；按钮在途时换上它，与 `feedbackFor(null, verb)` 是同一句。 */
export function pendingText(verb: string): string {
  return `${verb}中…`;
}

/**
 * `result` 为 `null` 表示请求在途（还没有结果代数可映）。
 *
 * `unconfigured` 归失败而不是单立一态：对操作者而言这一次动作同样没有形成答案；它「重试不会好」那层
 * 说明各页已各自整段写明，这里只给能与别处对得上的那一句。
 */
export function feedbackFor(result: ApiResult<unknown> | null, verb: string): ActionFeedback {
  if (result === null) return { state: 'pending', text: pendingText(verb) };
  switch (result.kind) {
    case 'outcome':
      return { state: 'answered', text: `已${verb}` };
    case 'callerProblem':
      return result.detail !== undefined && result.detail.trim() !== ''
        ? { state: 'failed', text: result.detail }
        : { state: 'failed', text: codeText(result.code, result.status) };
    case 'noAnswer':
      return { state: 'failed', text: codeText(result.code, result.status) };
    case 'transport': {
      const message = result.message.trim();
      return { state: 'failed', text: message === '' ? '请求未到达 parcel-api' : result.message };
    }
    case 'unconfigured':
      return { state: 'failed', text: '接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）' };
  }
}

function codeText(code: string, status: number): string {
  return `${code}（HTTP ${status}）`;
}
