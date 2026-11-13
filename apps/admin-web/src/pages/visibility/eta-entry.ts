import type { RegistrationResponseBody } from '../../components/registration';
import { postMasterData, type ApiResult } from '../catalogue-api';

/**
 * 预计到达（ETA）的手工录入（dox/decisions.md「产品形态」）：没有预测来源对接时，把承运商告知的、或运营自己估的预计
 * 到达录进来。先用登记面板接上——照提示填一份 JSON——不是引导式表单。
 */

export const etaEntryEndpoint = '/eta-prediction-manual-entries';

export const etaEntryTitle = '录入预计到达';

export const etaEntryHint =
  '请填 parcel（包裹号）、milestone（到哪个里程碑的预计时间，用标准里程碑的词）、source（CARRIER_PROVIDED 承运商告知的 / OPERATOR_DERIVED 我们自己估的，' +
  '两种各算各的）、rangeFrom 与 rangeTo（预计到达的时间范围，RFC 3339，北京时间请写 +08:00；rangeTo 必须晚于 rangeFrom，只知道一个时间点时请照材料给出前后的余量）、confidence（可信度）、' +
  'original（所凭原件：承运商邮件、通话记录或内部估算单的编号或存档引用，必填）。同一份内容重录不会多出一版；改了时间范围或换了原件就是新的一版，旧的一版保留。' +
  '录进来的预计到达要经过 ETA 展示规则判断之后才会给客户看，不是录了就对客展示。所属公司与录入人由系统按登录身份记下；需要持有「录入预计到达」的授予。';

export const etaEntryOutcomeLabels: Record<string, string> = {
  ETA_FORMED: '已录入第一版预计到达',
  ETA_REFRESHED: '已录入新的一版，前一版保留',
  ETA_EXISTING_RESULT: '这份内容已经录过，它就是现在算数的那一版',
  ETA_EXISTING_SUPERSEDED: '这份内容早先录过，但已被后来的一版接替；现在算数的是下面这一版。要改回先前的值，请凭新的原件再录一次',
  NOT_ACCEPTED: '内容不完整或不成立，没有受理',
};

export function etaEntryProblemNote(code: string): string {
  if (code === 'MALFORMED_REQUEST') {
    return '内容格式不对：请核对是否多填了字段、source 是否两个词之一、两个时间是否 RFC 3339 且 rangeTo 晚于 rangeFrom、original 与 confidence 是否填了。';
  }
  if (code === 'OPERATOR_NOT_GRANTED') return '你没有「录入预计到达」这一项授予，请联系管理员。';
  return `这次提交没有成功（错误码 ${code}）；稍后原样再录一次，仍不行请记下关联标识联系系统管理员。`;
}

export function submitETAEntry(snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(etaEntryEndpoint, snapshot);
}
