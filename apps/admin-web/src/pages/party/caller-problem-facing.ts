// 登记口 400 的呈现。同一码 MALFORMED_REQUEST 底下有两类恢复动作：改某一格，和对不上任何格子的请求形状
// （自报租户、未知键、尾随内容）。只拿得到 code 时说不清是哪一类，所以有 detail 时先看它点没点名到路径。
//
// 点名的形状来自身份族译装：`relationships[0].scope: party commercial: blank value: …`（party_identity_payload.go）。
// 明面用格子的名字加译好的一句；路径和英文原话不放在明面。对不上路径的散文仍原样给出，那半句本身就是人要改的东西。

import { fieldProblemText } from './field-problem-text';

export interface PointedFieldProblem {
  /** 服务端点名的路径。斜杠并排的两格拆开，句子相同。 */
  paths: string[];
  /** 路径后面的原话，交给 fieldProblemText 译。 */
  raw: string;
}

// 必须带 `[下标]`，免得一句普通散文里的冒号被当成路径。斜杠只出现在区间那种「两格一条」的点名里。
const pointedField = /^([A-Za-z][\w]*(?:\[\d+\])(?:\.[A-Za-z][\w]*)+(?:\/[A-Za-z][\w]*)?): (.+)$/;

/** 理由散文点名到登记项的某一格时拆出来；对不上这种形状就不是格子上的问题。 */
export function pointedFieldProblem(detail: string): PointedFieldProblem | null {
  const match = detail.trim().match(pointedField);
  if (match === null) return null;
  const head = match[1];
  const raw = match[2];
  const slash = head.indexOf('/');
  if (slash < 0) return { paths: [head], raw };
  const left = head.slice(0, slash);
  const right = head.slice(slash + 1);
  const dot = left.lastIndexOf('.');
  return { paths: [left, `${left.slice(0, dot + 1)}${right}`], raw };
}

export interface CallerProblemFacing {
  /** 明面上的那一句。 */
  lead: string;
  /** 对不上格子时原样放在「原因」里；点名到格子时不再重复。 */
  reason: string | null;
  /** 点名到格子时收进技术详情的整句原话；明面已经是原文时为空。 */
  technicalDetail: string;
}

/**
 * 调用方式问题在页脚怎么说。`fieldLabels` 有这一格的名字时，点名的退回写成「覆盖范围：必填，不能留空」；
 * 没有名字时只留译好的那句，路径仍在 technicalDetail 里。
 */
export function callerProblemFacing(
  code: string,
  detail: string | undefined,
  note: (code: string) => string,
  fieldLabels: Record<string, string> = {},
): CallerProblemFacing {
  const pointed = detail === undefined ? null : pointedFieldProblem(detail);
  if (pointed !== null && detail !== undefined) {
    const translated = fieldProblemText(pointed.raw).text;
    const labels = pointed.paths.flatMap((path) => {
      const label = fieldLabels[path];
      return label === undefined || label.trim() === '' ? [] : [label];
    });
    return {
      lead: labels.length > 0 ? `${labels.join('、')}：${translated}` : translated,
      reason: null,
      technicalDetail: detail.trim(),
    };
  }
  const reason = detail !== undefined && detail.trim() !== '' ? detail : null;
  return { lead: `调用方式问题：${note(code)}`, reason, technicalDetail: '' };
}
