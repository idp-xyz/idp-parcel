// 客户服务规则逐字段表单的纯逻辑（票 admin-write-faces/18；ADR-0101 决定八逐册裁形：逐字段表单，适用对象二选一控件 +
// 父行两格 + 两张子表可加行，期限种类的下拉由服务端词表读口供）。草稿形状、草稿 → 载荷、表单认领的 JSON 路径、
// 整数格的本地问题、词表取集、两张表的行增删改。全部是纯函数，node:test 钉着；组件只负责摆，五步由公共半边
// PublicationDraftFlow 走。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。空字段、种类在不在集内、引用在不在册一律送上去让
// 服务端答——预览口逐格 problems 回来挂到对应格旁；适用对象恰一、两项合起来至少一行、每一种期限与每一种索赔类型
// 至多一行都是服务端的门（前者判成 applicability 一格问题，后两条由构造门在预览上答成成因）。本地只做一件不算裁的
// 事：时长格填了编不进 JSON 整数的文本（根本组不出载荷，判据同 pre-acceptance-financial-control-policy-form 的顺序格）。
//
// 本册特有的两句：起算事件、日历、索赔类型与材料都是开放引用（解释权在 visibility-exception），本文件不替它们造一份
// 枚举；VE 那侧的两本册（通知义务、索赔类型覆盖）与本册正文归谁未裁（pc-gaps/05），这里只发布 PC 这一侧的正文。

import { integerOf, integerProblem, normalizeMoment } from './publication-form-shared';
import type {
  ClaimDeadlineRulePayload,
  CommercialPublicationPayload,
  CustomerServiceRuleBodyPayload,
  MinimumMaterialsRulePayload,
  VocabularySetRecord,
} from './publication-draft-api';

/**
 * 适用对象二选一控件的取值，存的是载荷键名原词；空串即未选。恰一在场由服务端裁（票 18「选形与理由」），控件只让
 * 操作者一次只能说一种——那是呈现，不是裁门：两格都填在这个控件上根本表达不出来，服务端那一格问题留给 JSON 镜像口。
 */
export type ServiceRuleAppliesTo = '' | 'serviceProduct' | 'customerContract';

/** 一行索赔期限的草稿，四格全是文本：一个码（空即未选）、两个开放引用串、一个整数文本。 */
export interface ClaimDeadlineDraft {
  kind: string;
  startEvent: string;
  days: string;
  calendar: string;
}

/** 一行最低材料的草稿：索赔类型引用串，加多行文本的材料清单——一行一项。 */
export interface MinimumMaterialsDraft {
  claimKind: string;
  materials: string;
}

/**
 * 草稿：版本壳五格 + 适用对象（控件取值 + 标识）+ 父行两格 + 两张子表。壳上的范围与区间是**版本**的（登记册逐列比对
 * 的项）；`ruleScope` 是正文自己的适用范围（0023 父行一格，CONTEXT「必须按服务产品和客户合同明确适用范围」）——两处
 * 各是各的格，表单不替操作者把一格抄进另一格。
 */
export interface ServiceRuleDraft {
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  appliesTo: ServiceRuleAppliesTo;
  appliesToId: string;
  responsible: string;
  ruleScope: string;
  claimDeadlines: ClaimDeadlineDraft[];
  minimumMaterials: MinimumMaterialsDraft[];
}

export function emptyClaimDeadlineDraft(): ClaimDeadlineDraft {
  return { kind: '', startEvent: '', days: '', calendar: '' };
}

export function emptyMinimumMaterialsDraft(): MinimumMaterialsDraft {
  return { claimKind: '', materials: '' };
}

/**
 * 空草稿：适用对象未选，两张表零行。与 pre-acceptance-financial-control-policy-form 预开一行空行不同：那一册的表至少要
 * 一项，预开的是可填的格；这里两张表各自都可以是空的（「这一版对期限无客户差异」是正文说出的真话），预开一行就替
 * 操作者预设了「这张表有内容」，要表达空表还得先删行。
 */
export function emptyServiceRuleDraft(): ServiceRuleDraft {
  return {
    objectId: '',
    version: '',
    scope: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    appliesTo: '',
    appliesToId: '',
    responsible: '',
    ruleScope: '',
    claimDeadlines: [],
    minimumMaterials: [],
  };
}

const bodyPath = 'customerServiceRule';
const deadlineRowPath = (index: number) => `${bodyPath}.claimDeadlines[${index}]`;
const materialsRowPath = (index: number) => `${bodyPath}.minimumMaterials[${index}]`;

/** 壳上指名引用的键：与正文里的适用对象是同一个值（判据同 customer-contract-form 的规则包一格）。 */
const shellReferenceKeyOf: Record<Exclude<ServiceRuleAppliesTo, ''>, 'SERVICE_PRODUCT' | 'CUSTOMER_CONTRACT'> = {
  serviceProduct: 'SERVICE_PRODUCT',
  customerContract: 'CUSTOMER_CONTRACT',
};

/**
 * 表单渲染的 JSON 路径（载荷键名原词），随控件取值与行数走。公共半边拿它分「本表单的格」与「未认领的路径」：服务端
 * 点名的路径不在这张表里就单列出来，不静默丢。适用对象一格同时认领正文那一键与壳上那条引用（页面上是同一格）；
 * `applicability` 是服务端对「两键恰一」的合成路径，载荷里没有这个键，也认领——那条问题要显在适用对象格旁。子表按行
 * 展开，材料按项展开；行本身（claimDeadlines[i] / minimumMaterials[i]）也认领——各格立得住而行拼不成时服务端点名整行。
 */
export function serviceRuleFieldPaths(draft: ServiceRuleDraft): string[] {
  const paths = ['objectId', 'version', 'scope', 'effectiveStartsAt', 'effectiveEndsAt', `${bodyPath}.applicability`];
  if (draft.appliesTo !== '') {
    paths.push(`references.${shellReferenceKeyOf[draft.appliesTo]}`, `${bodyPath}.${draft.appliesTo}`);
  }
  paths.push(`${bodyPath}.responsible`, `${bodyPath}.scope`);
  draft.claimDeadlines.forEach((_, index) => {
    const row = deadlineRowPath(index);
    paths.push(row, `${row}.kind`, `${row}.startEvent`, `${row}.days`, `${row}.calendar`);
  });
  // 材料清单只认领到项（materials[j]），不认领 `materials` 这一键本身：服务端对整张清单（空、重复）的话记在行上，
  // 对某一项的话记在那一项上，没有一条会落在键本身。
  draft.minimumMaterials.forEach((row, index) => {
    const path = materialsRowPath(index);
    paths.push(path, `${path}.claimKind`);
    materialLinesOf(row.materials).forEach((_, item) => paths.push(`${path}.materials[${item}]`));
  });
  return paths;
}

/**
 * 材料清单的多行文本 → 一项一串：按行切、去首尾空白、空行不是项。这是读一种输入格式，不是裁门——重复的项与整张空
 * 清单照发，「至少一项且不重复」由 NewMinimumMaterialsRule 答。
 */
function materialLinesOf(text: string): string[] {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line !== '');
}

/**
 * 本地编不进 JSON 类型的格，按 JSON 路径归组；空对象即可送预览。**只有各行的时长一格**——这不是校验，是组不出载荷：
 * 零与负数照发，让服务端说「须为正整数天」。
 */
export function serviceRuleLocalProblems(draft: ServiceRuleDraft): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  draft.claimDeadlines.forEach((row, index) => {
    const problem = integerProblem(row.days);
    if (problem !== null) problems[`${deadlineRowPath(index)}.days`] = [problem];
  });
  return problems;
}

/**
 * 草稿 → 产品定义的载荷。可缺的上界缺席而不是空串：服务端按键在场与否分辨「没有上界」。适用对象按控件取值只送两键
 * 之一（未选两键皆缺席），同一个标识同时进壳上的指名引用——标识留空时壳上不带一个空引用，正文那一键照送空串让服务端
 * 点名。码与引用原样送（只去首尾空白）：集内不集内由服务端答，改大小写或查表就是本地在裁。行序原样送、不按种类重排
 * ——文档按种类序与索赔类型序归一是服务端规范化的事。零行仍送两张 `[]`：「两项合起来至少一行」由构造门答。**载荷里
 * 没有身份也没有摘要**：租户与录入者由接入渠道的操作者信封给，摘要只有服务端算。
 */
export function serviceRulePayloadOf(draft: ServiceRuleDraft): CommercialPublicationPayload {
  const body: CustomerServiceRuleBodyPayload = {
    responsible: draft.responsible.trim(),
    scope: draft.ruleScope.trim(),
    claimDeadlines: draft.claimDeadlines.map((row) => {
      const item: ClaimDeadlineRulePayload = {
        kind: row.kind.trim(),
        startEvent: row.startEvent.trim(),
        calendar: row.calendar.trim(),
      };
      const days = integerOf(row.days);
      if (days !== undefined) item.days = days;
      return item;
    }),
    minimumMaterials: draft.minimumMaterials.map((row) => {
      const item: MinimumMaterialsRulePayload = {
        claimKind: row.claimKind.trim(),
        materials: materialLinesOf(row.materials),
      };
      return item;
    }),
  };
  const payload: CommercialPublicationPayload = {
    kind: 'CUSTOMER_SERVICE_RULE',
    objectId: draft.objectId.trim(),
    version: draft.version.trim(),
    scope: draft.scope.trim(),
    effectiveStartsAt: normalizeMoment(draft.effectiveStartsAt),
  };
  if (draft.effectiveEndsAt.trim() !== '') {
    payload.effectiveEndsAt = normalizeMoment(draft.effectiveEndsAt);
  }
  if (draft.appliesTo !== '') {
    const target = draft.appliesToId.trim();
    body[draft.appliesTo] = target;
    if (target !== '') payload.references = { [shellReferenceKeyOf[draft.appliesTo]]: target };
  }
  payload.customerServiceRule = body;
  return payload;
}

/**
 * 从词表答复里按名取一集的码（票 20：集合名就是载荷里那格的键名——本册只有 `kind` 一集）。没有那一集交回 null——
 * 本文件不内置 FIRST_CLAIM / MATERIAL_SUPPLEMENT / CONCLUSION_REVIEW 作回退，表单据 null 显占位；空数组是「有这一集
 * 但服务端今天没给码」，与缺席分开。
 */
export function serviceRuleCodesOf(sets: readonly VocabularySetRecord[], name: string): string[] | null {
  const set = sets.find((candidate) => candidate.name === name);
  return set ? [...set.codes] : null;
}

export function withClaimDeadlineRowAdded(draft: ServiceRuleDraft): ServiceRuleDraft {
  return { ...draft, claimDeadlines: [...draft.claimDeadlines, emptyClaimDeadlineDraft()] };
}

export function withClaimDeadlineRowRemoved(draft: ServiceRuleDraft, index: number): ServiceRuleDraft {
  return { ...draft, claimDeadlines: draft.claimDeadlines.filter((_, candidate) => candidate !== index) };
}

export function withClaimDeadlineRowPatched(
  draft: ServiceRuleDraft,
  index: number,
  patch: Partial<ClaimDeadlineDraft>,
): ServiceRuleDraft {
  return {
    ...draft,
    claimDeadlines: draft.claimDeadlines.map((row, candidate) => (candidate === index ? { ...row, ...patch } : row)),
  };
}

export function withMinimumMaterialsRowAdded(draft: ServiceRuleDraft): ServiceRuleDraft {
  return { ...draft, minimumMaterials: [...draft.minimumMaterials, emptyMinimumMaterialsDraft()] };
}

export function withMinimumMaterialsRowRemoved(draft: ServiceRuleDraft, index: number): ServiceRuleDraft {
  return { ...draft, minimumMaterials: draft.minimumMaterials.filter((_, candidate) => candidate !== index) };
}

export function withMinimumMaterialsRowPatched(
  draft: ServiceRuleDraft,
  index: number,
  patch: Partial<MinimumMaterialsDraft>,
): ServiceRuleDraft {
  return {
    ...draft,
    minimumMaterials: draft.minimumMaterials.map((row, candidate) => (candidate === index ? { ...row, ...patch } : row)),
  };
}
