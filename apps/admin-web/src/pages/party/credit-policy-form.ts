// 信用政策版本逐字段表单的纯逻辑（票 admin-write-faces/16；ADR-0101 决定八本册选形：逐字段表单 + 额度
// 「金额 / 比例」二选一控件）：草稿形状、本地编不进类型的格、草稿 → 载荷。全部是纯函数，node:test 钉着；
// 组件 CreditPolicyPublicationForm.tsx 只负责摆，五步由 PublicationDraftFlow 走。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。额度两格**恰一在场**由服务端领域构造门
// 判：两格都填或都空照发，答回来的是 `creditPolicy.limit` 那一格的拒绝，表单只呈现。零金额是登记方说出的
// 「授予零信用」，不折成缺席（读面 policy-rows.ts 的 creditLimitCell 同一判据）。本地只拦一类：整数格填了
// 编不进 JSON 整数的文本——那不是替服务端判断，是根本组不出那份载荷。空字段、区间先后、引用是否在册一律
// 送上去让服务端逐格点名。

import type { CreditPolicyRecord } from './api';
import type { CommercialPublicationPayload, CreditPolicyBodyPayload, VocabularySetRecord } from './publication-draft-api';
import { currentDisplayTimeZone } from '../moment';
import { integerOf, integerProblem, wallMomentOf, wallMomentProblem } from './publication-form-shared';

export interface CreditPolicyDraft {
  objectId: string;
  version: string;
  scope: string;
  /** 版本壳的有效区间；上界留空即持续有效。 */
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  legalEntity: string;
  authorityLevel: string;
  chargeType: string;
  /** 额度两格的文本；空即该格缺席。恰一由服务端裁，本地不挑。 */
  limitMinor: string;
  limitRatioBasisPoints: string;
  /**
   * 比例额度声明的基数（ADR-0129），词表读口 `ratioBase` 一集的码；空即缺席（「未选」是空值状态，不是默认值）。
   * 比例在场而它缺席、金额在场而它在场都照发，由服务端点名 `creditPolicy.ratioBase`；表单不替登记方挑分母。
   */
  ratioBase: string;
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
    ratioBase: '',
    bodyEffectiveStartsAt: '',
    bodyEffectiveEndsAt: '',
  };
}

/**
 * 「基于此版本发新版本」的预填（票 party-commercial-uiux/14）：同一个政策对象，责任法人、授权层级、费用类型、额度与基数照抄；
 * 版本号、版本的适用范围（读面没有）、两对生效区间一律留空。额度只抄记录上在场的那一格，两格都缺（响应不合契约）就都空着，
 * 不替人挑。
 */
export function creditPolicyDraftFromRecord(record: CreditPolicyRecord): CreditPolicyDraft {
  return {
    ...emptyCreditPolicyDraft(),
    objectId: record.objectId,
    legalEntity: record.legalEntity,
    authorityLevel: record.authorityLevel,
    chargeType: record.chargeType,
    limitMinor: record.limitMinor === undefined ? '' : String(record.limitMinor),
    limitRatioBasisPoints: record.limitRatioBasisPoints === undefined ? '' : String(record.limitRatioBasisPoints),
    ratioBase: record.ratioBase ?? '',
  };
}

/** 词表读口答复里 `ratioBase` 一集的码；答复里没有这一集即 null（缺席与空数组分开：空数组是有这一集但今天没给码）。 */
export function ratioBaseCodesOf(sets: readonly VocabularySetRecord[]): string[] | null {
  const set = sets.find((candidate) => candidate.name === 'ratioBase');
  return set ? [...set.codes] : null;
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
  'creditPolicy.ratioBase',
  'creditPolicy.effectiveStartsAt',
  'creditPolicy.effectiveEndsAt',
] as const;

/**
 * 组件里显 Problems 的路径表（票 22 判据 3），按 CreditPolicyPublicationForm 的 JSX 逐处抄：`kind` 一处 Problems 挂在版本壳
 * 标题下，`creditPolicy.limit` 一处 Problems 挂在额度两格下，`creditPolicy.ratioBase` 是基数下拉那一格（VocabularySelect
 * 自带 Field），其余各一 Field。与上面的认领表由 publication-form-rendered-paths.test.ts 比对——改 JSX 里的 path 要同步改这里。
 */
export const creditPolicyRenderedPaths: readonly string[] = [
  'kind',
  'objectId',
  'version',
  'scope',
  'effectiveStartsAt',
  'effectiveEndsAt',
  'creditPolicy.legalEntity',
  'creditPolicy.authorityLevel',
  'creditPolicy.chargeType',
  'creditPolicy.limitMinor',
  'creditPolicy.limitRatioBasisPoints',
  'creditPolicy.ratioBase',
  'creditPolicy.limit',
  'creditPolicy.effectiveStartsAt',
  'creditPolicy.effectiveEndsAt',
];

/**
 * 本地编不进 JSON 类型的格，按 JSON 路径归组；空对象即可送预览。**只此两格**——这不是校验，是组不出载荷。整数格的
 * 文本 → 数值与它的本地问题同出共享层一条判据（integerOf / integerProblem），两处不会一处放一处拦。
 */
export function creditPolicyLocalProblems(
  draft: CreditPolicyDraft,
  timeZone: string = currentDisplayTimeZone(),
): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  // 四个时刻格是日期时间控件的墙钟，换不成时刻同样是「组不出载荷」（票 party-commercial-uiux/10）。
  for (const [path, raw] of [
    ['effectiveStartsAt', draft.effectiveStartsAt],
    ['effectiveEndsAt', draft.effectiveEndsAt],
    ['creditPolicy.effectiveStartsAt', draft.bodyEffectiveStartsAt],
    ['creditPolicy.effectiveEndsAt', draft.bodyEffectiveEndsAt],
  ] as const) {
    const moment = wallMomentProblem(raw, timeZone);
    if (moment !== null) problems[path] = [moment];
  }
  const minor = integerProblem(draft.limitMinor);
  if (minor !== null) problems['creditPolicy.limitMinor'] = [minor];
  const ratio = integerProblem(draft.limitRatioBasisPoints);
  if (ratio !== null) problems['creditPolicy.limitRatioBasisPoints'] = [ratio];
  return problems;
}

/**
 * 草稿 → 产品定义的载荷。可缺的键（两个区间上界、两个额度格、基数）缺席而不是空串：服务端按键在场与否分辨
 * 「没有」，空串会被当成填了空的值送进构造门；额度格是数值，`0` 照发。基数选了就发，不看比例格填没填——金额带
 * 基数是服务端要点名的一格问题，表单静默丢掉它反而把那格问题吞了。必填的键空着也送——预览会逐格点名，
 * 表单据此挂到格旁。**载荷里没有身份、没有摘要**：租户与录入者由接入渠道的操作者信封给。
 */
export function creditPolicyPayloadOf(
  draft: CreditPolicyDraft,
  timeZone: string = currentDisplayTimeZone(),
): CommercialPublicationPayload {
  const body: CreditPolicyBodyPayload = {
    legalEntity: draft.legalEntity.trim(),
    authorityLevel: draft.authorityLevel.trim(),
    chargeType: draft.chargeType.trim(),
    effectiveStartsAt: wallMomentOf(draft.bodyEffectiveStartsAt, timeZone),
  };
  const limitMinor = integerOf(draft.limitMinor);
  if (limitMinor !== undefined) body.limitMinor = limitMinor;
  const limitRatioBasisPoints = integerOf(draft.limitRatioBasisPoints);
  if (limitRatioBasisPoints !== undefined) body.limitRatioBasisPoints = limitRatioBasisPoints;
  if (draft.ratioBase.trim() !== '') body.ratioBase = draft.ratioBase.trim();
  if (draft.bodyEffectiveEndsAt.trim() !== '') body.effectiveEndsAt = wallMomentOf(draft.bodyEffectiveEndsAt, timeZone);

  const payload: CommercialPublicationPayload = {
    kind: 'CREDIT_POLICY',
    objectId: draft.objectId.trim(),
    version: draft.version.trim(),
    scope: draft.scope.trim(),
    effectiveStartsAt: wallMomentOf(draft.effectiveStartsAt, timeZone),
    creditPolicy: body,
  };
  if (draft.effectiveEndsAt.trim() !== '') payload.effectiveEndsAt = wallMomentOf(draft.effectiveEndsAt, timeZone);
  return payload;
}
