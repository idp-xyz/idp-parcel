import type { RegistrationResponseBody } from '../../components/registration';
import { postMasterData, type ApiResult } from '../catalogue-api';

/**
 * 承运方订舱答复的手工录入（dox/decisions.md「产品形态」）：没有对接承运商系统时，承运方用电话或邮件答复了订舱，照着
 * 录进来。先用登记面板接上——照提示填一份 JSON——不是引导式表单。
 */

export const bookingAnswerEntryEndpoint = '/transport-fulfillment-booking-answer-manual-entries';

export const bookingAnswerEntryTitle = '录入承运方的订舱答复';

export const bookingAnswerEntryHint =
  '请填 booking（订舱标识，在「订舱」区的技术详情里）、answer（给这份答复起的标识，同一份答复重录要用同一个）、' +
  'outcome（ACCEPTED 接受 / REFUSED 拒绝 / EXPIRED 失效 / WITHDRAWN 撤回 / WAITLISTED 候补）、quantity（只在接受时填，是接受量）、' +
  'basis（其余四种时填，是原因或候补的依据）、decidedAt（承运方答复的时间，RFC 3339，不能晚于现在）、' +
  'original（所凭原件：邮件、通话记录的编号或存档引用，必填）。答复方不用填：系统取这份订舱所属委托的承运方。' +
  '一次订舱只有一份定局，候补之后可以再录一份定局；已有定局之后再录另一份会答冲突。' +
  '所属公司与录入人由系统按登录身份记下；需要持有「录入承运方的订舱答复」的授予。';

export const bookingAnswerEntryOutcomeLabels: Record<string, string> = {
  BOOKING_ANSWERED: '已记下承运方的答复',
  EXISTING_ANSWER: '这份答复已经录过（以第一次录入的为准）',
  ANSWER_CONFLICT: '这次订舱已经有定局，或同一个标识下录的是另一份内容；没有覆盖',
  BOOKING_NOT_FOUND: '找不到这份订舱，请核对订舱标识',
  INPUT_NOT_ACCEPTED: '内容不完整或不成立（例如没填答复时间，或答复时间早于订舱提出的时间），没有受理',
  COMMISSION_UNDECIDED: '暂时定不了，稍后原样再录一次即可',
};

/** 这一口自己的错误说明：页面上别的口的说明讲的是别的字段。 */
export function bookingAnswerEntryProblemNote(code: string): string {
  if (code === 'MALFORMED_REQUEST') {
    return '内容格式不对：请核对是否多填了字段、outcome 是否五个词之一、original 是否填了、decidedAt 是否 RFC 3339 且不晚于现在（北京时间请写 +08:00，不要写 Z）。';
  }
  if (code === 'OPERATOR_NOT_GRANTED') return '你没有「录入承运方的订舱答复」这一项授予，请联系管理员。';
  return `这次提交没有成功（错误码 ${code}）；稍后原样再录一次，仍不行请记下关联标识联系系统管理员。`;
}

export function submitBookingAnswerEntry(snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(bookingAnswerEntryEndpoint, snapshot);
}
