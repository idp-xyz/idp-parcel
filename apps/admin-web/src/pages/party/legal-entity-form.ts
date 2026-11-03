// 责任法人身份登记表单的纯逻辑（票 admin-web-group-legal-entities/02，身份层三格随票 legal-entity-profile/04 加入；
// ADR-0101 决定八自裁：格少、低频、无矩阵——直接逐字段表单）。全部是纯函数，node:test 钉着；组件
// LegalEntityRegistrationForm.tsx 只负责摆。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）。参与方在不在册、届时
// 是否已生效、修订号连不连续、依据是否为空，一律原样送上去让服务端逐格答；这里只做编码层的事：
// 修订号编成 JSON 整数、墙钟时刻换成 RFC 3339、可缺的键缺席而不是空串。
//
// **载荷不带租户格。** 镜像受控 CLI `parcel-commercial register-parties` 输入里 `legalEntities` 数组的一项，
// 但去掉整批的 `tenantId`：在线 Intake 要把认证结果填进租户格、只从载荷取行内容（register_party_identity.go
// 包注释）；载荷里带上租户就是请求方自报租户。今天端点挂 UnconfiguredIntake{} 必答 403，这份形状没有服务端
// 消费者，真 Intake 落地时以它为准重谈，不把这里当已发布 Schema（api.ts 那段注释的原话）。
//
// **建档一体化**（票 company-subject-ux/02）：这家公司还没有往来单位档案时，保存先打往来单位口再打法人口，两步串行、
// 不加后端复合命令（spec「不变的决定」）。串行的状态机在 runPairedSave 一族，往来单位那一步的载荷借 business-party-form.ts
// 的草稿形状组成，这里只决定送什么、发不发，不判任何一步答复的领域含义。

import type { ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';
import type { GroupLegalEntityRecord, LifetimeRegistrationNumberRecord, RegistrationNumberTypeRecord } from './api';
import { wallTimeToRfc3339 } from '../moment';
import type { BusinessPartyDraft } from './business-party-form';
import { revisionOf, suggestedNextRevision } from './registration-form';

/** 终身注册号的一行草稿：类型码与号各是文本框原值。 */
export interface LifetimeNumberDraft {
  typeCode: string;
  number: string;
}

/**
 * 往来单位档案从哪来（票 company-subject-ux/02 建档一体化）：`together` 是这家公司还没有档案、保存时先登记一份再登记法人；
 * `existing` 是从参与方册上选一份已有的。缺省 `together`——新增公司主体的人多半是在建一家新公司，先去另一页登记档案
 * 再回来选，正是这张票要收掉的三步路。
 */
export type CompanionPartyMode = 'together' | 'existing';

export interface LegalEntityDraft {
  legalEntityId: string;
  /** 选已有档案时选中的参与方标识；一并登记时不看这一格，送出的参与方标识由 effectivePartyIdOf 算。 */
  partyId: string;
  partyMode: CompanionPartyMode;
  /** 一并登记时的公司名称原值，登在往来单位档案上。 */
  partyName: string;
  /** 一并登记时往来单位编码的手填值；留空即取公司主体编码（companionPartyIdOf）。 */
  companionPartyId: string;
  /**
   * 第一步落册后登记成的往来单位编码；空串即还没落。它钉住两件事：之后的保存只发法人那一步，以及法人钉的参与方就是这个编码——
   * 落册之后操作者再改公司主体编码，不能把法人钉到一个没登记过的参与方上。
   */
  companionRegisteredPartyId: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟），留空即缺席。 */
  effectiveFrom: string;
  /** 注册国家 / 地区码原值（ADR-0145 决定一），留空即缺席。 */
  registrationCountry: string;
  /** 终身注册号逐行；类型与号都空的行当作没填。 */
  lifetimeNumbers: LifetimeNumberDraft[];
  /** 身份更正依据原值（ADR-0145 决定二：录错走更正修订并携带依据），留空即缺席。 */
  identityCorrectionBasis: string;
}

export interface LegalEntityRegistrationItem {
  legalEntityId: string;
  partyId: string;
  revision?: number;
  basis: string;
  effectiveFrom?: string;
  registrationCountry?: string;
  lifetimeRegistrationNumbers?: LifetimeRegistrationNumberRecord[];
  identityCorrectionBasis?: string;
}

export interface LegalEntityRegistrationPayload {
  legalEntities: LegalEntityRegistrationItem[];
}

export function emptyLegalEntityDraft(): LegalEntityDraft {
  return {
    legalEntityId: '',
    partyId: '',
    partyMode: 'together',
    partyName: '',
    companionPartyId: '',
    companionRegisteredPartyId: '',
    revision: '',
    basis: '',
    effectiveFrom: '',
    registrationCountry: '',
    lifetimeNumbers: [{ typeCode: '', number: '' }],
    identityCorrectionBasis: '',
  };
}

/**
 * 草稿里有没有还没提交的输入——登记视图离开前要不要先确认（票 group-legal-entities-uiux/04）。只有空白或只有空的注册号行
 * 不算输入，免得空表单也拦人。`landed` 是最近一次落册时送出的那份：内容没再动就不算没提交；修订号格不比，送出那份
 * 带的是建议值，草稿里那一格可能还是空的。
 */
export function legalEntityDraftUnsaved(draft: LegalEntityDraft, landed: LegalEntityDraft | null): boolean {
  if (!draftHasInput(draft)) return false;
  return landed === null || contentKey(draft) !== contentKey(landed);
}

function draftHasInput(draft: LegalEntityDraft): boolean {
  // 档案来源的选择本身不算输入（缺省就是一并登记）；一并登记那组的两格与已落册的编码算——第一步落了、第二步没落的表单
  // 正是最不该被静默离开的那种。
  const cells = [
    draft.legalEntityId,
    draft.partyId,
    draft.partyName,
    draft.companionPartyId,
    draft.companionRegisteredPartyId,
    draft.revision,
    draft.basis,
    draft.effectiveFrom,
    draft.registrationCountry,
    draft.identityCorrectionBasis,
    ...draft.lifetimeNumbers.flatMap((row) => [row.typeCode, row.number]),
  ];
  return cells.some((cell) => cell.trim() !== '');
}

function contentKey(draft: LegalEntityDraft): string {
  const { revision: _revision, ...content } = draft;
  return JSON.stringify(content);
}

/** 表单认领的 JSON 路径，服务端逐格问题按它点名；一项一批，下标固定 0。 */
export const legalEntityFieldPaths = [
  'legalEntities[0].legalEntityId',
  'legalEntities[0].partyId',
  'legalEntities[0].revision',
  'legalEntities[0].basis',
  'legalEntities[0].effectiveFrom',
  'legalEntities[0].registrationCountry',
  'legalEntities[0].lifetimeRegistrationNumbers',
  'legalEntities[0].identityCorrectionBasis',
] as const;

export type LegalEntityFieldPath = (typeof legalEntityFieldPaths)[number];

/**
 * 草稿 → 载荷。各格去首尾空白后原样带（空串照送，那是服务端要点名的格）；修订号编不进正整数
 * 与生效时刻换不出来时**该键缺席**——不造一个假数顶上，由 legalEntityLocalProblems 在送之前拦。
 */
export function legalEntityPayloadOf(draft: LegalEntityDraft, timeZone: string): LegalEntityRegistrationPayload {
  const item: LegalEntityRegistrationItem = {
    legalEntityId: draft.legalEntityId.trim(),
    partyId: effectivePartyIdOf(draft),
    basis: draft.basis.trim(),
  };
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  if (draft.effectiveFrom.trim() !== '') {
    const moment = wallTimeToRfc3339(draft.effectiveFrom, timeZone);
    if (moment !== null) item.effectiveFrom = moment;
  }
  // 身份层三格留空即**键缺席**，不送空串：Intake 对这三键「给了就得合形」，空串会被当成给了而答形状错；缺席才轮到
  // 用例按 ADR-0145 决定一答「未受理」并点名缺的是哪格——那才是操作者要看的答案。
  const country = draft.registrationCountry.trim();
  if (country !== '') item.registrationCountry = country;
  const numbers = filledNumberRows(draft.lifetimeNumbers);
  if (numbers.length > 0) item.lifetimeRegistrationNumbers = numbers;
  const correction = draft.identityCorrectionBasis.trim();
  if (correction !== '') item.identityCorrectionBasis = correction;
  return { legalEntities: [item] };
}

/**
 * 一并登记时往来单位的编码：手填了就用手填的，留空取公司主体编码。两头都裁首尾空白——这个串要同时送进两口（先登往来
 * 单位、再作法人钉的参与方），本册的载荷裁空白（legalEntityPayloadOf），往来单位那一步得与它说同一个对象。
 */
export function companionPartyIdOf(draft: Pick<LegalEntityDraft, 'legalEntityId' | 'companionPartyId'>): string {
  const typed = draft.companionPartyId.trim();
  return typed !== '' ? typed : draft.legalEntityId.trim();
}

/**
 * 法人载荷里钉的参与方标识：选已有档案 → 选中的那一个；一并登记 → 第一步已落册就是登记成的那个编码，还没落就是此刻算出的
 * 编码。落册之后不再跟着公司主体编码变，理由在 companionRegisteredPartyId 的注释。
 */
export function effectivePartyIdOf(draft: LegalEntityDraft): string {
  if (draft.partyMode === 'existing') return draft.partyId.trim();
  return draft.companionRegisteredPartyId !== '' ? draft.companionRegisteredPartyId : companionPartyIdOf(draft);
}

/**
 * 一并登记那一步送往往来单位口的草稿。来源单据与生效时间就用法人这一笔的：往来单位生效时间 = 法人生效时间，法人生效时参与方
 * 已生效这条领域规则由此在数据上成立，管理台仍不判它——送上去让服务端答。修订号固定为 1：这一组的语义是「这家公司还没有
 * 档案」，就是首笔；编码已在册时服务端按册面答（同内容「已登记过」、异内容「内容冲突」），不在本地替它算下一修订——那会把
 * 「一并登记」悄悄变成「更新已有档案的名称」。
 */
export function companionPartyDraftOf(draft: LegalEntityDraft): BusinessPartyDraft {
  return {
    partyId: companionPartyIdOf(draft),
    name: draft.partyName.trim(),
    revision: '1',
    basis: draft.basis.trim(),
    effectiveFrom: draft.effectiveFrom,
  };
}

/** 这一轮保存要不要先发往来单位那一步：一并登记且还没落册才发；选已有档案或第一步已落过都只发法人。 */
export function pairedSavePlan(draft: LegalEntityDraft): { party: boolean } {
  return { party: draft.partyMode === 'together' && draft.companionRegisteredPartyId === '' };
}

/**
 * 往来单位那一步算不算落了：答 `REGISTERED` 当然算；答 `ALREADY_REGISTERED`（同内容已在册）也算——档案在册就是第二步要的
 * 前提，典型是第二步失败后刷新了页面、再保存时第一步重放。判的只是答案代数里的两格线上名（partyIdentityOutcomeLabels 的键），
 * 不看中文、不读散文原因。
 */
export function partyStepLanded(answer: ApiResult<RegistrationResponseBody>): boolean {
  return answer.kind === 'outcome' && (answer.body.outcome === 'REGISTERED' || answer.body.outcome === 'ALREADY_REGISTERED');
}

/** 第一步落册后的草稿：钉上登记成的编码（之后只发第二步、法人钉它）。 */
export function withCompanionRegistered(draft: LegalEntityDraft, partyId: string): LegalEntityDraft {
  return { ...draft, companionRegisteredPartyId: partyId };
}

export interface PairedSaveSenders {
  party: (companion: BusinessPartyDraft) => Promise<ApiResult<RegistrationResponseBody>>;
  entity: (draft: LegalEntityDraft) => Promise<ApiResult<RegistrationResponseBody>>;
}

export interface PairedSaveResult {
  /** 这一轮发了第一步时它的答复；没发（选的是已有档案，或第一步早已落册）为 null。 */
  party: ApiResult<RegistrationResponseBody> | null;
  /** 法人那一步的答复；第一步没过为 null——第二步没发。 */
  entity: ApiResult<RegistrationResponseBody> | null;
  /** 这一轮之后的草稿：第一步落册时钉上了编码，其余情形原样交回。 */
  draft: LegalEntityDraft;
}

/**
 * 建档一体化的两步串行：先登记往来单位，落了再登记法人；第一步没过就停在那里，第二步不发。两步各自原样送、原样交回答复，
 * 这里不解答复里任何领域含义（伞票 admin-write-faces/07 硬句）。第一步落册后草稿钉上编码，所以第二步失败再保存时
 * pairedSavePlan 只会放第二步——「只重发第二步」不是这里记的状态，是草稿带着的。发送函数由组件注入，这一层不碰网络。
 */
export async function runPairedSave(draft: LegalEntityDraft, send: PairedSaveSenders): Promise<PairedSaveResult> {
  if (!pairedSavePlan(draft).party) return { party: null, entity: await send.entity(draft), draft };
  const companion = companionPartyDraftOf(draft);
  const party = await send.party(companion);
  if (!partyStepLanded(party)) return { party, entity: null, draft };
  const pinned = withCompanionRegistered(draft, companion.partyId);
  return { party, entity: await send.entity(pinned), draft: pinned };
}

/** 去首尾空白后，类型与号至少一格有字的行；都空的行当作没填。半填的行照带，由 legalEntityLocalProblems 在送之前拦。 */
function filledNumberRows(rows: readonly LifetimeNumberDraft[]): LifetimeRegistrationNumberRecord[] {
  return rows
    .map((row) => ({ typeCode: row.typeCode.trim(), number: row.number.trim() }))
    .filter((row) => row.typeCode !== '' || row.number !== '');
}

/**
 * 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。只有三种：修订号编不进正整数、
 * 生效时刻换不成 RFC 3339、终身注册号有一行只填了类型或只填了号（Intake 收的每一项两格都得有，半行编不成一项）。
 * 空格、引用是否在册、国家在不在目录里、号合不合格式、修订是否连续都不在这里判，那是服务端的话。
 */
export function legalEntityLocalProblems(draft: LegalEntityDraft, timeZone: string): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['legalEntities[0].revision'] = ['修订号要填正整数'];
  }
  if (draft.effectiveFrom.trim() !== '' && wallTimeToRfc3339(draft.effectiveFrom, timeZone) === null) {
    problems['legalEntities[0].effectiveFrom'] = ['生效时刻要是可解析的日期时间'];
  }
  const halfRows = draft.lifetimeNumbers
    .map((row, index) => ({ index, typeCode: row.typeCode.trim(), number: row.number.trim() }))
    .filter((row) => (row.typeCode === '') !== (row.number === ''));
  if (halfRows.length > 0) {
    problems['legalEntities[0].lifetimeRegistrationNumbers'] = halfRows.map(
      (row) => `第 ${row.index + 1} 行：类型与号要成对填`,
    );
  }
  return problems;
}

/** 选单一项；与 PublicationFormFields 的 PickerOption 同形，纯逻辑这层不去引 .tsx。 */
export interface IdentityCatalogueOption {
  value: string;
  label: string;
}

/**
 * 注册国家 / 地区候选：目录里登过身份层类型的国家码，去重、按码排序。不按状态过滤——表单不裁，停用与否由服务端
 * 登记时按目录判。
 */
export function identityCountryOptions(types: readonly RegistrationNumberTypeRecord[]): IdentityCatalogueOption[] {
  const codes = new Set(types.filter((type) => type.layer === 'IDENTITY').map((type) => type.countryCode));
  return [...codes].sort().map((code) => ({ value: code, label: code }));
}

/**
 * 某国家 / 地区的终身注册号类型候选：只列身份层（资料层的号不收进身份，ADR-0145 决定一），国家码去首尾空白后比。
 * 已停用的类型照样列、括注「已停用」：候选是给人看的，拒不拒由服务端判。
 */
export function identityTypeOptions(
  types: readonly RegistrationNumberTypeRecord[],
  country: string,
): IdentityCatalogueOption[] {
  const code = country.trim();
  return types
    .filter((type) => type.layer === 'IDENTITY' && type.countryCode === code)
    .map((type) => ({
      value: type.typeCode,
      label: `${type.typeCode} · ${type.typeName}${type.deactivatedAt !== undefined ? '（已停用）' : ''}`,
    }));
}

/**
 * 修订号建议值（规则在 suggestedNextRevision，这里只交本册取键的字段）。标识先裁首尾空白再找：本册的载荷裁空白
 * （legalEntityPayloadOf，票 02 验收过、legal-entity-form.test.ts 钉着），建议得与送上去的串说同一个对象。裁在这一层
 * 而不进共用体——其余各册不裁，是服务端各口判据的差别，不是共用体该替谁定的；要统一先改票 02 的判据与用例（票 13 第 3 条
 * 那半不做的理由）。
 */
export function suggestedRevision(rows: readonly GroupLegalEntityRecord[] | null, legalEntityId: string): number {
  return suggestedNextRevision(rows, legalEntityId.trim(), (row) => row.legalEntityId);
}
