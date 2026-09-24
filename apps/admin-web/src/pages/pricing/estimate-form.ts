// 试算表单的纯逻辑（票 operator-workspace-gaps/06；UC-PP-001「试算声明」；载荷键逐字取传输层 EstimatePayload 的 json 标签）。
// 全部是纯函数，node:test 钉着；组件 PricingEstimatePage.tsx 只负责摆。
//
// **本文件不裁领域规则。** 范围是否为空、方向认不认识、重量单位是否在领域词表里、哪张卡要分区哪张要邮编路线，一律原样送上去让
// 服务端逐格答；这里只做编码层的事：墙钟时刻换 RFC 3339、可缺的格留空即缺席、成组的格（尺寸三边与单位、邮编路线两端）不许半填。
// **载荷不带租户格**：租户从操作者信封来，传输层按未知键拒。

import { wallTimeToRfc3339 } from '../moment';
import type { EstimatePayload } from './api';

export interface EstimateDraft {
  scope: string;
  direction: string;
  /** `datetime-local` 原值（操作者本地墙钟）。 */
  basisAt: string;
  weightValue: string;
  weightUnit: string;
  length: string;
  width: string;
  height: string;
  lengthUnit: string;
  zone: string;
  origin: string;
  destination: string;
  settlementCurrency: string;
}

/** 空草稿：一格不预填业务值（UC-PP-001「本用例一格不给默认」）。 */
export function emptyEstimateDraft(): EstimateDraft {
  return {
    scope: '',
    direction: '',
    basisAt: '',
    weightValue: '',
    weightUnit: '',
    length: '',
    width: '',
    height: '',
    lengthUnit: '',
    zone: '',
    origin: '',
    destination: '',
    settlementCurrency: '',
  };
}

const filled = (value: string) => value.trim() !== '';

/** 草稿 → 载荷。时刻换不出来时该键缺席，由 estimateLocalProblems 在送之前拦；可缺的格留空即缺席。 */
export function estimatePayloadOf(draft: EstimateDraft, timeZone: string): EstimatePayload {
  const payload: EstimatePayload = {
    scope: draft.scope,
    direction: draft.direction,
    weight: { value: draft.weightValue, unit: draft.weightUnit },
  };
  const basisAt = filled(draft.basisAt) ? wallTimeToRfc3339(draft.basisAt, timeZone) : null;
  if (basisAt !== null) payload.basisAt = basisAt;
  if ([draft.length, draft.width, draft.height, draft.lengthUnit].some(filled)) {
    payload.dimensions = { length: draft.length, width: draft.width, height: draft.height, unit: draft.lengthUnit };
  }
  if (filled(draft.zone)) payload.zone = draft.zone;
  if (filled(draft.origin) || filled(draft.destination)) {
    payload.postalRoute = { origin: draft.origin, destination: draft.destination };
  }
  if (filled(draft.settlementCurrency)) payload.settlementCurrency = draft.settlementCurrency;
  return payload;
}

export type EstimateProblemKey = 'basisAt' | 'dimensions' | 'postalRoute';

/** 编码层问题，按格归组；空对象即可送。空的必需格不在这里拦——那是服务端要点名的格。 */
export function estimateLocalProblems(draft: EstimateDraft, timeZone: string): Partial<Record<EstimateProblemKey, string>> {
  const problems: Partial<Record<EstimateProblemKey, string>> = {};
  if (filled(draft.basisAt) && wallTimeToRfc3339(draft.basisAt, timeZone) === null) {
    problems.basisAt = '时刻格式应为本地日期与时间';
  }
  const sides = [draft.length, draft.width, draft.height, draft.lengthUnit];
  if (sides.some(filled) && !sides.every(filled)) {
    problems.dimensions = '尺寸要三边与单位一起填，或都不填';
  }
  if (filled(draft.origin) !== filled(draft.destination)) {
    problems.postalRoute = '邮编路线要始发与目的一起填，或都不填';
  }
  return problems;
}
