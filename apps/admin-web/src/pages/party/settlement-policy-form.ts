// 结算政策逐字段表单的纯逻辑（票 admin-write-faces/15；ADR-0101 决定八逐册裁形：逐字段表单，合同引用对象 + 版本
// 一起选，方式下拉由服务端词表读口供）。草稿形状、草稿 → 载荷、表单认领的 JSON 路径、合同选项键、词表取集。
// 全部是纯函数，node:test 钉着；组件只负责摆，五步由公共半边 PublicationDraftFlow 走。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。空字段、区间先后、方式在不在集内、引用在不在册、
// 币种存不存在一律送上去让服务端答——预览口逐格 problems 回来挂到对应格旁；这里连「必填」都不拦，免得两处口径。
// 本册特有的一句：结算政策答「怎么结」不答「要不要接受前控制」（那是 0007 / 0024 两层的事），草稿里没有控制字段。

import { normalizeMoment } from './publication-form-shared';
import type {
  CommercialPublicationPayload,
  SettlementPolicyBodyPayload,
  VocabularySetRecord,
} from './publication-draft-api';

/**
 * 草稿：版本壳五格 + 政策正文九格，全是文本。壳上的区间是**版本**的（SaveVersion 逐列比对的项），正文里的是
 * **政策适用**的（0011 六维之一），领域分开收、表单也分开给；「区间从版本壳带入」只是抄一次。壳上的适用范围与正文
 * 的费用范围（chargeScope）是两种引用，不抄。`method` 存的是服务端词表给的码，空串即未选——不预选。
 */
export interface SettlementPolicyDraft {
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  method: string;
  legalEntity: string;
  counterparty: string;
  contractObjectId: string;
  contractVersion: string;
  chargeScope: string;
  currency: string;
  policyEffectiveStartsAt: string;
  policyEffectiveEndsAt: string;
}

export function emptySettlementPolicyDraft(): SettlementPolicyDraft {
  return {
    objectId: '',
    version: '',
    scope: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    method: '',
    legalEntity: '',
    counterparty: '',
    contractObjectId: '',
    contractVersion: '',
    chargeScope: '',
    currency: '',
    policyEffectiveStartsAt: '',
    policyEffectiveEndsAt: '',
  };
}

/**
 * 表单渲染的 JSON 路径（载荷键名原词）。公共半边拿它分「本表单的格」与「未认领的路径」：服务端点名的路径不在
 * 这张表里就单列出来，不静默丢。合同维展开到两格——服务端逐格问题落在 settlementPolicy.contract.objectId /
 * .version 上，不落在 contract 本身。这张表与 settlementPolicyPayloadOf 能产出的键一一对应，测试钉着。
 */
export const settlementPolicyFieldPaths: readonly string[] = [
  'objectId',
  'version',
  'scope',
  'effectiveStartsAt',
  'effectiveEndsAt',
  'settlementPolicy.method',
  'settlementPolicy.legalEntity',
  'settlementPolicy.counterparty',
  'settlementPolicy.contract.objectId',
  'settlementPolicy.contract.version',
  'settlementPolicy.chargeScope',
  'settlementPolicy.currency',
  'settlementPolicy.effectiveStartsAt',
  'settlementPolicy.effectiveEndsAt',
];

/**
 * 草稿 → 产品定义的载荷。可缺的上界缺席而不是空串：服务端按键在场与否分辨「没有上界」。方式与币种原样送（只去
 * 首尾空白）：集内不集内、存在不存在由服务端答，改大小写或查表就是本地在裁。合同维是对象 + 版本两格——两段式
 * 指称串只许领域 NewQualifiedVersionLabel 一处拼，表单不拼版本号。壳上不带指名引用。**载荷里没有身份也没有摘要**：
 * 租户与录入者由接入渠道的操作者信封给，摘要只有服务端算。
 */
export function settlementPolicyPayloadOf(draft: SettlementPolicyDraft): CommercialPublicationPayload {
  const body: SettlementPolicyBodyPayload = {
    method: draft.method.trim(),
    legalEntity: draft.legalEntity.trim(),
    counterparty: draft.counterparty.trim(),
    contract: { objectId: draft.contractObjectId.trim(), version: draft.contractVersion.trim() },
    chargeScope: draft.chargeScope.trim(),
    currency: draft.currency.trim(),
    effectiveStartsAt: normalizeMoment(draft.policyEffectiveStartsAt),
  };
  if (draft.policyEffectiveEndsAt.trim() !== '') {
    body.effectiveEndsAt = normalizeMoment(draft.policyEffectiveEndsAt);
  }
  const payload: CommercialPublicationPayload = {
    kind: 'SETTLEMENT_POLICY',
    objectId: draft.objectId.trim(),
    version: draft.version.trim(),
    scope: draft.scope.trim(),
    effectiveStartsAt: normalizeMoment(draft.effectiveStartsAt),
    settlementPolicy: body,
  };
  if (draft.effectiveEndsAt.trim() !== '') {
    payload.effectiveEndsAt = normalizeMoment(draft.effectiveEndsAt);
  }
  return payload;
}

/** 客户合同目录里的一版：合同对象 + 版本号，表单让操作者一起选。 */
export interface ContractVersionChoice {
  objectId: string;
  version: string;
}

/**
 * 目录行 → 选单项的键。两格编成一个不歧义的串（JSON 数组），**不用「/」拼**：拼「对象/版本」是领域
 * NewQualifiedVersionLabel 一处的事，表单在这里拼一份就是第二处知道分隔符的代码，而且标识里若含「/」还拆不回来。
 */
export function contractChoiceKey(choice: ContractVersionChoice): string {
  return JSON.stringify([choice.objectId, choice.version]);
}

/** 选单项的键 → 目录行的两格；键对不上任何一行交回 null（目录刷新把当前值刷掉时，选单据此保留手填项）。 */
export function contractChoiceOf(
  key: string,
  contracts: readonly ContractVersionChoice[],
): ContractVersionChoice | null {
  const found = contracts.find((contract) => contractChoiceKey(contract) === key);
  return found ? { objectId: found.objectId, version: found.version } : null;
}

/**
 * 从词表答复里取名为 `method` 的那一集的码（票 20：集合名就是载荷里那格的键名）。没有那一集交回 null——本文件
 * 不内置 PREPAID / TERMS 作回退，表单据 null 显占位；空数组是「有这一集但服务端今天没给码」，与缺席分开。
 */
export function methodCodesOf(sets: readonly VocabularySetRecord[]): string[] | null {
  const set = sets.find((candidate) => candidate.name === 'method');
  return set ? [...set.codes] : null;
}

/**
 * 「区间从版本壳带入」：把壳上的区间抄进政策适用区间那两格，其余不动。只抄区间不抄范围——壳范围是版本的适用范围，
 * 正文的 chargeScope 是费用范围引用，两样不是一回事。抄的是文本，仍由服务端逐格判。
 */
export function withShellIntervalCopiedIntoPolicy(draft: SettlementPolicyDraft): SettlementPolicyDraft {
  return {
    ...draft,
    policyEffectiveStartsAt: draft.effectiveStartsAt,
    policyEffectiveEndsAt: draft.effectiveEndsAt,
  };
}
