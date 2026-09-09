// 信用政策版本逐字段表单的纯逻辑（票 admin-write-faces/16；ADR-0101 决定八本册选形：逐字段表单 + 额度
// 「金额 / 比例」二选一控件）：草稿形状、本地编不进类型的格、草稿 → 载荷。全部是纯函数，node:test 钉着；
// 组件 CreditPolicyPublicationForm.tsx 只负责摆，五步由 PublicationDraftFlow 走。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。额度两格**恰一在场**由服务端领域构造门
// 判：两格都填或都空照发，答回来的是 `creditPolicy.limit` 那一格的拒绝，表单只呈现。零金额是登记方说出的
// 「授予零信用」，不折成缺席（读面 policy-rows.ts 的 creditLimitCell 同一判据）。本地只拦一类：整数格填了
// 编不进 JSON 整数的文本——那不是替服务端判断，是根本组不出那份载荷。空字段、区间先后、引用是否在册一律
// 送上去让服务端逐格点名。

import type { CommercialPublicationPayload, CreditPolicyBodyPayload } from './publication-draft-api';
import { integerOf, integerProblem, normalizeMoment } from './publication-form-shared';

// 时点归一曾定义在本文件、被兄弟表单借用（票 22 抬到共享层）；这里保留导出只为既有调用点与测试不改一字，定义只在共享层。
export { normalizeMoment };

export interface CreditPolicyDraft {
  objectId: string;
  version: string;
  scope: string;
  /** 版本壳的有效区间；上界留空即无上界。 */
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  legalEntity: string;
  authorityLevel: string;
  chargeType: string;
  /** 额度两格的文本；空即该格缺席。恰一由服务端裁，本地不挑。 */
  limitMinor: string;
  limitRatioBasisPoints: string;
  /** 正文自己的有效区间（0020 正文列），与壳区间是两回事，各自送。 */
  bodyEffectiveStartsAt: string;
  bodyEffectiveEndsAt: string;
}

export function emptyCreditPolicyDraft(): CreditPolicyDraft {
  return {
    objectId: '',
    version: '',
    scope: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    legalEntity: '',
    authorityLevel: '',
    chargeType: '',
    limitMinor: '',
    limitRatioBasisPoints: '',
    bodyEffectiveStartsAt: '',
    bodyEffectiveEndsAt: '',
  };
}

/**
 * 表单认领的 JSON 路径：载荷能发出的每一条键，加服务端对「恰一」点名用的 `creditPolicy.limit`。流程组件把
 * 这些路径上的问题交给表单挂到格旁，其余路径单列。
 */
export const creditPolicyFieldPaths = [
  'kind',
  'objectId',
  'version',
  'scope',
  'effectiveStartsAt',
  'effectiveEndsAt',
  'creditPolicy.legalEntity',
  'creditPolicy.authorityLevel',
  'creditPolicy.chargeType',
  'creditPolicy.limit',
  'creditPolicy.limitMinor',
  'creditPolicy.limitRatioBasisPoints',
  'creditPolicy.effectiveStartsAt',
  'creditPolicy.effectiveEndsAt',
] as const;

/**
 * 本地编不进 JSON 类型的格，按 JSON 路径归组；空对象即可送预览。**只此两格**——这不是校验，是组不出载荷。整数格的
 * 文本 → 数值与它的本地问题同出共享层一条判据（integerOf / integerProblem），两处不会一处放一处拦。
 */
export function creditPolicyLocalProblems(draft: CreditPolicyDraft): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  const minor = integerProblem(draft.limitMinor);
  if (minor !== null) problems['creditPolicy.limitMinor'] = [minor];
  const ratio = integerProblem(draft.limitRatioBasisPoints);
  if (ratio !== null) problems['creditPolicy.limitRatioBasisPoints'] = [ratio];
  return problems;
}

/**
 * 草稿 → 产品定义的载荷。可缺的键（两个区间上界、两个额度格）缺席而不是空串：服务端按键在场与否分辨
 * 「没有」，空串会被当成填了空的值送进构造门；额度格是数值，`0` 照发。必填的键空着也送——预览会逐格点名，
 * 表单据此挂到格旁。**载荷里没有身份、没有摘要**：租户与录入者由接入渠道的操作者信封给。
 */
export function creditPolicyPayloadOf(draft: CreditPolicyDraft): CommercialPublicationPayload {
  const body: CreditPolicyBodyPayload = {
    legalEntity: draft.legalEntity.trim(),
    authorityLevel: draft.authorityLevel.trim(),
    chargeType: draft.chargeType.trim(),
    effectiveStartsAt: normalizeMoment(draft.bodyEffectiveStartsAt),
  };
  const limitMinor = integerOf(draft.limitMinor);
  if (limitMinor !== undefined) body.limitMinor = limitMinor;
  const limitRatioBasisPoints = integerOf(draft.limitRatioBasisPoints);
  if (limitRatioBasisPoints !== undefined) body.limitRatioBasisPoints = limitRatioBasisPoints;
  if (draft.bodyEffectiveEndsAt.trim() !== '') body.effectiveEndsAt = normalizeMoment(draft.bodyEffectiveEndsAt);

  const payload: CommercialPublicationPayload = {
    kind: 'CREDIT_POLICY',
    objectId: draft.objectId.trim(),
    version: draft.version.trim(),
    scope: draft.scope.trim(),
    effectiveStartsAt: normalizeMoment(draft.effectiveStartsAt),
    creditPolicy: body,
  };
  if (draft.effectiveEndsAt.trim() !== '') payload.effectiveEndsAt = normalizeMoment(draft.effectiveEndsAt);
  return payload;
}
