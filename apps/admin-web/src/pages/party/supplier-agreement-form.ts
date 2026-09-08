// 供应商协议逐字段表单的纯逻辑（票 admin-write-faces/11；ADR-0101 决定八逐册裁形：逐字段表单，采购方案从
// 价卡目录选）。草稿形状、草稿 → 载荷、表单认领的 JSON 路径。全部是纯函数，node:test 钉着；组件只负责摆，
// 五步由公共半边 PublicationDraftFlow 走。写法照 pricing/series-form.ts。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。空字段、区间先后、引用是否在册一律送上去
// 让服务端答——预览口逐格 problems 回来挂到对应格旁；这里连「必填」都不拦，免得两处口径。

import type { PriceCardRecord } from '../pricing/api';
import { normalizeMoment } from '../pricing/series-form';
import type { CommercialPublicationPayload, SupplierAgreementBodyPayload } from './publication-draft-api';

/**
 * 草稿：版本壳五格 + 协议正文六格，全是文本。壳与正文各有自己的适用范围与区间——壳上的是**版本**的
 * 适用范围与区间（SaveVersion 逐列比对的项），正文里的是**协议**的（0021 正文，随发布同笔登记进供应商协议册），
 * 领域把两样分开收，表单也分开给；「从版本壳带入」只是抄一次，不是把两样并成一样。
 */
export interface SupplierAgreementDraft {
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  supplier: string;
  legalEntity: string;
  agreementScope: string;
  purchasePlan: string;
  agreementEffectiveStartsAt: string;
  agreementEffectiveEndsAt: string;
}

export function emptySupplierAgreementDraft(): SupplierAgreementDraft {
  return {
    objectId: '',
    version: '',
    scope: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    supplier: '',
    legalEntity: '',
    agreementScope: '',
    purchasePlan: '',
    agreementEffectiveStartsAt: '',
    agreementEffectiveEndsAt: '',
  };
}

/**
 * 表单渲染的 JSON 路径（载荷键名原词）。公共半边拿它分「本表单的格」与「未认领的路径」：服务端点名的
 * 路径不在这张表里就单列出来，不静默丢。这张表与 supplierAgreementPayloadOf 能产出的键一一对应，测试钉着。
 */
export const supplierAgreementFieldPaths: readonly string[] = [
  'objectId',
  'version',
  'scope',
  'effectiveStartsAt',
  'effectiveEndsAt',
  'supplierAgreement.supplier',
  'supplierAgreement.legalEntity',
  'supplierAgreement.scope',
  'supplierAgreement.purchasePlan',
  'supplierAgreement.effectiveStartsAt',
  'supplierAgreement.effectiveEndsAt',
];

/**
 * 草稿 → 产品定义的载荷。可缺的上界缺席而不是空串：服务端按键在场与否分辨「没有上界」。壳上不带指名引用
 * （协议正文里的供应商、法人与方案已经是它的全部引用，0021 正文没有别的），不带方向。**载荷里没有身份也
 * 没有摘要**：租户与录入者由接入渠道的操作者信封给，摘要只有服务端算。
 */
export function supplierAgreementPayloadOf(draft: SupplierAgreementDraft): CommercialPublicationPayload {
  const body: SupplierAgreementBodyPayload = {
    supplier: draft.supplier.trim(),
    legalEntity: draft.legalEntity.trim(),
    scope: draft.agreementScope.trim(),
    purchasePlan: draft.purchasePlan.trim(),
    effectiveStartsAt: normalizeMoment(draft.agreementEffectiveStartsAt),
  };
  if (draft.agreementEffectiveEndsAt.trim() !== '') {
    body.effectiveEndsAt = normalizeMoment(draft.agreementEffectiveEndsAt);
  }
  const payload: CommercialPublicationPayload = {
    kind: 'SUPPLIER_AGREEMENT',
    objectId: draft.objectId.trim(),
    version: draft.version.trim(),
    scope: draft.scope.trim(),
    effectiveStartsAt: normalizeMoment(draft.effectiveStartsAt),
    supplierAgreement: body,
  };
  if (draft.effectiveEndsAt.trim() !== '') {
    payload.effectiveEndsAt = normalizeMoment(draft.effectiveEndsAt);
  }
  return payload;
}

/**
 * 价卡目录行 → 方案版本引用串。写法与 settlement-accounting 拼方案版本引用同一条（`planId@planVersion`，
 * 见 internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go）；跨上下文只传引用，表单不读方案内容，
 * 方向与绑定换算是 PRICE_RULE 那册的事。
 */
export function planReferenceOf(card: Pick<PriceCardRecord, 'planId' | 'planVersion'>): string {
  return `${card.planId}@${card.planVersion}`;
}

/** 「从版本壳带入」：把壳上的适用范围与区间抄进协议正文那三格，其余不动。抄的是文本，仍由服务端逐格判。 */
export function withShellCopiedIntoAgreement(draft: SupplierAgreementDraft): SupplierAgreementDraft {
  return {
    ...draft,
    agreementScope: draft.scope,
    agreementEffectiveStartsAt: draft.effectiveStartsAt,
    agreementEffectiveEndsAt: draft.effectiveEndsAt,
  };
}
