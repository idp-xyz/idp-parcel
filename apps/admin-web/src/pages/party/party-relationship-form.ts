// 参与方关系登记表单的纯逻辑（票 admin-web-group-legal-entities/10 第 2 条）。全部是纯函数，node:test 钉着；
// 组件 PartyRelationshipRegistrationForm.tsx 只负责摆。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：双方是否在册、角色与双方是否
// 匹配、区间是否倒置、修订是否连续，一律送上去让服务端答。这里只做编码层：修订号编成整数、各墙钟时刻换成
// RFC 3339、**可缺键缺席而不是零值**。
//
// 缺席为什么不能用零值顶：partyRelationshipDocument 用指针表达 effectiveEndsAt 与 approval 的缺席——缺终点即开
// 区间、缺批准即登为候选关系；零时刻在 NewEffectiveInterval 里恰是「无终点」，一个显式给出的坏时刻也会解成零值，
// 两者就分不开了，所以键在场解不出时刻是形状错。表单这一层把「操作者没填」编成缺键，把「填了但换不出来」拦在
// 本地，两种情形都不让一个零时刻过线。
//
// 各格不裁首尾空白、载荷不带租户格，理由同 business-party-form.ts。
//
// 适用范围一格原样带，只有授权交付方例外：它的范围服务端只认一种拼法，手写查询串容易拼成第二种写法而被拒，所以
// 这一角色改填客户账户与节点两格，由 authorized-delivery-scope.ts 拼；拼不出（如账户空）是编码失败，与修订号编不出
// 同办。
//
// 标了必填却留空的格子不在编码层里判：表单刚打开不先标红。点保存时由 partyRelationshipSubmitGaps 按格拦下、请求不发出。
// 服务端仍退回并点名到路径时，由 partyRelationshipServerProblems 把原话挂回那一格。

import type { PartyRelationshipRecord } from './api';
import { pointedFieldProblem } from './caller-problem-facing';
import { authorizedDeliveringPartyRole, spellAuthorizedDeliveryScope } from './authorized-delivery-scope';
import { wallTimeToRfc3339 } from '../moment';
import { partyRoleLabels } from './presentation';
import { revisionOf, suggestedNextRevision } from './registration-form';
import type { PickerOption } from './PublicationFormFields';

export interface PartyRelationshipDraft {
  relationshipId: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  holder: string;
  counterparty: string;
  /** partyRoleLabels 的键之一；空串是「未选」，照送让服务端点名，表单不预选。 */
  role: string;
  /** 适用范围原值，授权交付方以外的角色用；选授权交付方时不进载荷，切回别的角色时原值还在。 */
  scope: string;
  /** 授权交付方的适用范围收到的客户账户原值；只在选了这一角色时用。 */
  scopeCustomerAccount: string;
  /** 授权交付方的适用范围可再收到的节点原值；留空即不收窄到节点。 */
  scopeNode: string;
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟）。 */
  effectiveStartsAt: string;
  /** 留空即开区间——键缺席。 */
  effectiveEndsAt: string;
  /** 「已批准」勾选框：不勾即候选关系，批准引用与批准时刻不进载荷，残字也不进。 */
  approved: boolean;
  approvalReference: string;
  approvedAt: string;
}

/** 键名逐字取 relationshipApprovalDocument 的 json 标签。 */
export interface PartyRelationshipApprovalItem {
  reference: string;
  approvedAt?: string;
}

/** 键名逐字取 partyRelationshipDocument 的 json 标签。 */
export interface PartyRelationshipRegistrationItem {
  relationshipId: string;
  revision?: number;
  holder: string;
  counterparty: string;
  role: string;
  /** 授权交付方的两格拼不出时缺席，由 partyRelationshipLocalProblems 拦在送之前。 */
  scope?: string;
  basis: string;
  effectiveStartsAt?: string;
  effectiveEndsAt?: string;
  approval?: PartyRelationshipApprovalItem;
}

export interface PartyRelationshipRegistrationPayload {
  relationships: PartyRelationshipRegistrationItem[];
}

export function emptyPartyRelationshipDraft(): PartyRelationshipDraft {
  return {
    relationshipId: '',
    revision: '',
    holder: '',
    counterparty: '',
    role: '',
    scope: '',
    scopeCustomerAccount: '',
    scopeNode: '',
    basis: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    approved: false,
    approvalReference: '',
    approvedAt: '',
  };
}

/** 表单认领的 JSON 路径；一项一批，下标固定 0；批准那几格带嵌套路径。 */
export const partyRelationshipFieldPaths = [
  'relationships[0].relationshipId',
  'relationships[0].revision',
  'relationships[0].holder',
  'relationships[0].counterparty',
  'relationships[0].role',
  'relationships[0].scope',
  'relationships[0].basis',
  'relationships[0].effectiveStartsAt',
  'relationships[0].effectiveEndsAt',
  'relationships[0].approval.reference',
  'relationships[0].approval.approvedAt',
] as const;

export type PartyRelationshipFieldPath = (typeof partyRelationshipFieldPaths)[number];

/**
 * 表单开头的那段说明。单列在这里是因为它有两处可摆：表单自己的开头，或调用方的段头上（合作关系视图为了让表单矮一行，把它
 * 摆在段头，表单那一处就不再摆）——两处用的是同一句。
 */
export const partyRelationshipFormIntro =
  '记录谁对谁担任什么角色、从什么时候开始。双方都要先登记为往来单位；没有批准信息的先作为待确认关系，批准后再保存一次。';

/** 角色下拉的选项：只从 partyRoleLabels 派生，码是它的键、顺序同它——不在这里再抄一份封闭集。 */
export function partyRoleOptions(): PickerOption[] {
  return Object.entries(partyRoleLabels).map(([code, label]) => ({ value: code, label: `${label} · ${code}` }));
}

/** 授权交付方那两格的拼法结果；组件拿它显「将送的适用范围」，载荷与问题表也从它取，三处不各拼一遍。 */
export function deliveryScopeSpellingOf(draft: PartyRelationshipDraft) {
  return spellAuthorizedDeliveryScope({ customerAccount: draft.scopeCustomerAccount, node: draft.scopeNode });
}

/** 本草稿送出去的适用范围：授权交付方由两格拼，拼不出 → undefined（键缺席）；其余角色原样带 scope。 */
function scopeOf(draft: PartyRelationshipDraft): string | undefined {
  if (draft.role !== authorizedDeliveringPartyRole) return draft.scope;
  const spelled = deliveryScopeSpellingOf(draft);
  return spelled.ok ? spelled.scope : undefined;
}

// 墙钟留空 → undefined（键缺席）；换出来 → RFC 3339；换不出来也 → undefined，由问题表拦住不送。
function momentOf(wallTime: string, timeZone: string): string | undefined {
  if (wallTime.trim() === '') return undefined;
  return wallTimeToRfc3339(wallTime, timeZone) ?? undefined;
}

/**
 * 草稿 → 载荷。身份串与引用串原样带；修订号与各时刻编不出时**该键缺席**；终点留空与不勾已批准也缺席——
 * 前者是编码失败、后者是操作者的声明，落到载荷上同为缺键，由 partyRelationshipLocalProblems 把前者拦在送之前。
 */
export function partyRelationshipPayloadOf(
  draft: PartyRelationshipDraft,
  timeZone: string,
): PartyRelationshipRegistrationPayload {
  const item: PartyRelationshipRegistrationItem = {
    relationshipId: draft.relationshipId,
    holder: draft.holder,
    counterparty: draft.counterparty,
    role: draft.role,
    basis: draft.basis,
  };
  const scope = scopeOf(draft);
  if (scope !== undefined) item.scope = scope;
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  const startsAt = momentOf(draft.effectiveStartsAt, timeZone);
  if (startsAt !== undefined) item.effectiveStartsAt = startsAt;
  const endsAt = momentOf(draft.effectiveEndsAt, timeZone);
  if (endsAt !== undefined) item.effectiveEndsAt = endsAt;
  if (draft.approved) {
    const approval: PartyRelationshipApprovalItem = { reference: draft.approvalReference };
    const approvedAt = momentOf(draft.approvedAt, timeZone);
    if (approvedAt !== undefined) approval.approvedAt = approvedAt;
    item.approval = approval;
  }
  return { relationships: [item] };
}

/**
 * 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。修订号编不进正整数、各时刻填了却换不成 RFC 3339、
 * 授权交付方的两格拼不成那一种适用范围；批准那两格只在勾了已批准时才看——不勾时它们根本不进载荷。勾了就两格都要有：
 * 批准单据空着，服务端在接入口答形状错（400）；批准时刻缺着，用例答的是笼统的状态迁移不成立，都不点名是哪一格，
 * 所以在这里按格拦下。角色未选、双方为空、覆盖范围留空这些必填空格不在这里——表单刚打开不标红，点保存才由
 * partyRelationshipSubmitGaps 拦下。区间倒置仍是服务端的话。
 */
export function partyRelationshipLocalProblems(
  draft: PartyRelationshipDraft,
  timeZone: string,
): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['relationships[0].revision'] = ['修订号要填正整数'];
  }
  if (draft.role === authorizedDeliveringPartyRole) {
    const spelled = deliveryScopeSpellingOf(draft);
    if (!spelled.ok) problems['relationships[0].scope'] = spelled.problems;
  }
  const unparseable = (wallTime: string) => wallTime.trim() !== '' && wallTimeToRfc3339(wallTime, timeZone) === null;
  if (unparseable(draft.effectiveStartsAt)) {
    problems['relationships[0].effectiveStartsAt'] = ['生效起点要是可解析的日期时间'];
  }
  if (unparseable(draft.effectiveEndsAt)) {
    problems['relationships[0].effectiveEndsAt'] = ['生效终点要是可解析的日期时间'];
  }
  if (draft.approved) {
    if (draft.approvalReference.trim() === '') {
      problems['relationships[0].approval.reference'] = ['勾了「已批准」就要填批准单据'];
    }
    if (draft.approvedAt.trim() === '') {
      problems['relationships[0].approval.approvedAt'] = ['勾了「已批准」就要填批准时间'];
    } else if (unparseable(draft.approvedAt)) {
      problems['relationships[0].approval.approvedAt'] = ['批准时刻要是可解析的日期时间'];
    }
  }
  return problems;
}

/**
 * 修订号建议值（规则在 suggestedNextRevision，这里只交本册取键的字段）；按原串比不裁空白，理由同
 * suggestedBusinessPartyRevision。
 */
export function suggestedPartyRelationshipRevision(
  rows: readonly PartyRelationshipRecord[] | null,
  relationshipId: string,
): number {
  return suggestedNextRevision(rows, relationshipId, (row) => row.relationshipId);
}

/** 点保存时才说的那一句。与发布表单把 blank value 译成的话是同一句，避免两处各写各的。 */
export const partyRelationshipRequiredBlank = '必填，不能留空';

/** 页脚点名用的格子名字。句子里「哪一方」随本单位锁在哪一侧而变，由摆表单的那一层改这两格。 */
export const partyRelationshipGapLabels: Record<string, string> = {
  'relationships[0].relationshipId': '关系编号',
  'relationships[0].holder': '哪一方',
  'relationships[0].counterparty': '对哪一方',
  'relationships[0].role': '角色',
  'relationships[0].scope': '覆盖范围',
  'relationships[0].basis': '来源单据',
  'relationships[0].effectiveStartsAt': '开始时间',
  'relationships[0].effectiveEndsAt': '结束时间',
  'relationships[0].approval.reference': '批准单据',
  'relationships[0].approval.approvedAt': '批准时间',
};

/**
 * 点保存时才露出的必填空格。表单打开时不调用，所以空表不会先红一片。授权交付方的覆盖范围另有拼法，空账户已在
 * 编码层报，这里不把 scope 原值再报一遍。区间倒置、角色词不在封闭集里，仍留给服务端。
 */
export function partyRelationshipSubmitGaps(draft: PartyRelationshipDraft): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  const blank = (path: string, value: string) => {
    if (value.trim() === '') problems[path] = [partyRelationshipRequiredBlank];
  };
  blank('relationships[0].relationshipId', draft.relationshipId);
  blank('relationships[0].holder', draft.holder);
  blank('relationships[0].counterparty', draft.counterparty);
  blank('relationships[0].role', draft.role);
  if (draft.role !== authorizedDeliveringPartyRole) blank('relationships[0].scope', draft.scope);
  blank('relationships[0].basis', draft.basis);
  blank('relationships[0].effectiveStartsAt', draft.effectiveStartsAt);
  return problems;
}

// 页脚点名和焦点都按人在表单上遇到格子的顺序，不按载荷键的顺序。关系编号在表单最后一格，载荷里却排在最前。
const partyRelationshipBlankOrder = [
  'relationships[0].holder',
  'relationships[0].counterparty',
  'relationships[0].role',
  'relationships[0].scope',
  'relationships[0].effectiveStartsAt',
  'relationships[0].basis',
  'relationships[0].relationshipId',
  'relationships[0].effectiveEndsAt',
  'relationships[0].approval.reference',
  'relationships[0].approval.approvedAt',
] as const;

/** 第一处还空着的格子；没有就 null。 */
export function firstPartyRelationshipGapPath(gaps: Record<string, string[]>): string | null {
  return partyRelationshipBlankOrder.find((path) => (gaps[path]?.length ?? 0) > 0) ?? null;
}

/** 页脚那一句。没有空格就没有这句话。 */
export function partyRelationshipGapSummary(
  gaps: Record<string, string[]>,
  labelOf: (path: string) => string = (path) => partyRelationshipGapLabels[path] ?? path,
): string | null {
  const names = partyRelationshipBlankOrder.flatMap((path) => ((gaps[path]?.length ?? 0) > 0 ? [labelOf(path)] : []));
  if (names.length === 0) return null;
  return `还有 ${names.length} 项要填：${names.join('、')}`;
}

/**
 * 服务端退回里点名到路径的那一句，挂回格子。必填空格已经在页上标着时不再挂第二遍。空值类的原话在人把格子填上之后
 * 撤下；其余原话留到下一次保存，人改了什么服务端还没再看过。
 */
export function partyRelationshipServerProblems(
  detail: string | undefined,
  draft: PartyRelationshipDraft,
  alreadyShown: Record<string, string[]>,
): Record<string, string[]> {
  if (detail === undefined || detail.trim() === '') return {};
  const pointed = pointedFieldProblem(detail);
  if (pointed === null) return {};
  const problems: Record<string, string[]> = {};
  for (const path of pointed.paths) {
    if ((alreadyShown[path]?.length ?? 0) > 0) continue;
    if (pointed.raw.startsWith('party commercial: blank value') && draftValue(draft, path).trim() !== '') continue;
    problems[path] = [pointed.raw];
  }
  return problems;
}

function draftValue(draft: PartyRelationshipDraft, path: string): string {
  switch (path) {
    case 'relationships[0].relationshipId':
      return draft.relationshipId;
    case 'relationships[0].holder':
      return draft.holder;
    case 'relationships[0].counterparty':
      return draft.counterparty;
    case 'relationships[0].role':
      return draft.role;
    case 'relationships[0].scope':
      return draft.role === authorizedDeliveringPartyRole ? draft.scopeCustomerAccount : draft.scope;
    case 'relationships[0].basis':
      return draft.basis;
    case 'relationships[0].effectiveStartsAt':
      return draft.effectiveStartsAt;
    case 'relationships[0].effectiveEndsAt':
      return draft.effectiveEndsAt;
    case 'relationships[0].approval.reference':
      return draft.approvalReference;
    case 'relationships[0].approval.approvedAt':
      return draft.approvedAt;
    default:
      return '';
  }
}
