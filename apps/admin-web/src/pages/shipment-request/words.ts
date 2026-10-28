import type { TechnicalRow } from '../../components/states/technical-details';
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
