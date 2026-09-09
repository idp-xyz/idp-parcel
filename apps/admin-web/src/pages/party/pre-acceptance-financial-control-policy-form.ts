// 接受前财务控制策略逐字段表单的纯逻辑（票 admin-write-faces/13；ADR-0101 决定八逐册裁形：逐字段表单，共同通过条件
// 一格 + 控制项可加行，三个封闭集的下拉由服务端词表读口供）。草稿形状、草稿 → 载荷、表单认领的 JSON 路径、整数格的
// 本地问题、词表取集、重复提示、行的增删改。全部是纯函数，node:test 钉着；组件只负责摆，五步由公共半边
// PublicationDraftFlow 走。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。空字段、条件 / 种类 / 处置在不在集内、范围与责任引用
// 在不在册一律送上去让服务端答——预览口逐格 problems 回来挂到对应格旁；至少一项、顺序唯一、（种类 × 范围）唯一是
// 跨行的门，由构造门在预览上答成成因。本地只做两件不算裁的事：整数格填了编不进 JSON 整数的文本（根本组不出载荷，
// 判据同 credit-policy-form）；重复的行**提示**一句——票面允许「提交前提示重复」，但拒绝的话由服务端说，提示不进
// problems、不挡预览。
//
// 本册特有的一句：「无控制」不在这里——那是客户合同的声明（ADR-0115 Decision 一），控制种类下拉只列服务端词表给的码，
// 本文件不内置任何一格，自然也长不出它。

import { integerOf, integerProblem, normalizeMoment } from './publication-form-shared';
import type {
  CommercialPublicationPayload,
  PreAcceptanceControlItemPayload,
  PreAcceptanceFinancialControlPolicyBodyPayload,
  VocabularySetRecord,
} from './publication-draft-api';

/** 一行控制项的草稿，五格全是文本：三个码（空即未选）、两个开放引用串、一个整数文本。 */
export interface ControlItemDraft {
  control: string;
  chargeScope: string;
  order: string;
  onFailure: string;
  responsibility: string;
}

/**
 * 草稿：版本壳五格 + 共同通过条件一格 + 控制项若干行。壳上的区间是**版本**的（登记册逐列比对的项）；本册正文没有
 * 自己的区间与范围——策略正文只说控制怎么做（ADR-0115），适用哪个范围由客户合同按费用范围绑上来。`jointPassCondition`
 * 存的是服务端词表给的码，空串即未选——首发只有一值仍不预选：那是租户说出来的，不是产品替它默认的。
 */
export interface ControlPolicyDraft {
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  jointPassCondition: string;
  controls: ControlItemDraft[];
}

export function emptyControlItemDraft(): ControlItemDraft {
  return { control: '', chargeScope: '', order: '', onFailure: '', responsibility: '' };
}

/** 空草稿带一行空控制项：那是可填的格，不是值——正文是「一格 + 一张几行的表」，零行的表没有地方可填。 */
export function emptyControlPolicyDraft(): ControlPolicyDraft {
  return {
    objectId: '',
    version: '',
    scope: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    jointPassCondition: '',
    controls: [emptyControlItemDraft()],
  };
}

const bodyPath = 'preAcceptanceFinancialControlPolicy';
const rowPath = (index: number) => `${bodyPath}.controls[${index}]`;

/**
 * 表单渲染的 JSON 路径（载荷键名原词），随行数走。公共半边拿它分「本表单的格」与「未认领的路径」：服务端点名的路径
 * 不在这张表里就单列出来，不静默丢。子表按行展开到 controls[i].<键>，服务端逐格问题就落在那里；行本身
 * （controls[i]）也认领——五格各自立得住而行拼不成时服务端点名整行，那条要显在行下不是显成别处的问题。
 */
export function controlPolicyFieldPaths(draft: ControlPolicyDraft): string[] {
  const paths = ['objectId', 'version', 'scope', 'effectiveStartsAt', 'effectiveEndsAt', `${bodyPath}.jointPassCondition`];
  draft.controls.forEach((_, index) => {
    const row = rowPath(index);
    paths.push(row, `${row}.control`, `${row}.chargeScope`, `${row}.order`, `${row}.onFailure`, `${row}.responsibility`);
  });
  return paths;
}

/**
 * 本地编不进 JSON 类型的格，按 JSON 路径归组；空对象即可送预览。**只有各行的判断顺序一格**——这不是校验，是组不出
 * 载荷：零与负数照发，让服务端说「须从 1 起」。整数文本的判据出共享层（integerOf / integerProblem），与载荷那一侧同一条。
 */
export function controlPolicyLocalProblems(draft: ControlPolicyDraft): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  draft.controls.forEach((row, index) => {
    const problem = integerProblem(row.order);
    if (problem !== null) problems[`${rowPath(index)}.order`] = [problem];
  });
  return problems;
}

/**
 * 草稿 → 产品定义的载荷。可缺的上界缺席而不是空串：服务端按键在场与否分辨「没有上界」。三个码原样送（只去首尾空白）：
 * 集内不集内由服务端答，改大小写或查表就是本地在裁。行序原样送、不按顺序重排——文档按判断顺序归一是服务端规范化的事。
 * 零行仍送 `controls: []`：「至少一项」由构造门答。**载荷里没有身份也没有摘要**：租户与录入者由接入渠道的操作者信封给，
 * 摘要只有服务端算。
 */
export function controlPolicyPayloadOf(draft: ControlPolicyDraft): CommercialPublicationPayload {
  const body: PreAcceptanceFinancialControlPolicyBodyPayload = {
    jointPassCondition: draft.jointPassCondition.trim(),
    controls: draft.controls.map((row) => {
      const item: PreAcceptanceControlItemPayload = {
        control: row.control.trim(),
        chargeScope: row.chargeScope.trim(),
        onFailure: row.onFailure.trim(),
        responsibility: row.responsibility.trim(),
      };
      const order = integerOf(row.order);
      if (order !== undefined) item.order = order;
      return item;
    }),
  };
  const payload: CommercialPublicationPayload = {
    kind: 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY',
    objectId: draft.objectId.trim(),
    version: draft.version.trim(),
    scope: draft.scope.trim(),
    effectiveStartsAt: normalizeMoment(draft.effectiveStartsAt),
    preAcceptanceFinancialControlPolicy: body,
  };
  if (draft.effectiveEndsAt.trim() !== '') {
    payload.effectiveEndsAt = normalizeMoment(draft.effectiveEndsAt);
  }
  return payload;
}

/**
 * 从词表答复里按名取一集的码（票 20：集合名就是载荷里那格的键名——本册是 jointPassCondition / control / onFailure）。
 * 没有那一集交回 null——本文件不内置 ALL_CONTROLS_PASS / PREPAID_FREEZE / REJECT 作回退，表单据 null 显占位；
 * 空数组是「有这一集但服务端今天没给码」，与缺席分开。
 */
export function controlPolicyCodesOf(sets: readonly VocabularySetRecord[], name: string): string[] | null {
  const set = sets.find((candidate) => candidate.name === name);
  return set ? [...set.codes] : null;
}

/**
 * 提交前的重复提示，一句一条：两行抢同一个判断顺序、同一范围上同一种控制两行。只提示不拒——拒绝由服务端说（票 13
 * 「选形与理由」），所以它不进 problems、不挡预览。没填全的格不参与比对：空值不算撞，撞不撞等填完再看。
 * 行号从 1 数，与操作者眼前的表一致。
 */
export function controlRowHints(draft: ControlPolicyDraft): string[] {
  const hints: string[] = [];
  const byOrder = new Map<string, number[]>();
  const byKey = new Map<string, { control: string; chargeScope: string; rows: number[] }>();
  draft.controls.forEach((row, index) => {
    const order = row.order.trim();
    if (order !== '') byOrder.set(order, [...(byOrder.get(order) ?? []), index + 1]);
    const control = row.control.trim();
    const chargeScope = row.chargeScope.trim();
    if (control !== '' && chargeScope !== '') {
      const key = JSON.stringify([control, chargeScope]);
      const seen = byKey.get(key) ?? { control, chargeScope, rows: [] };
      seen.rows.push(index + 1);
      byKey.set(key, seen);
    }
  });
  for (const [order, rows] of byOrder) {
    if (rows.length > 1) hints.push(`判断顺序 ${order} 有 ${rows.length} 行在抢（第 ${rows.join('、')} 行）——顺序版本内唯一，拒绝由服务端说`);
  }
  for (const { control, chargeScope, rows } of byKey.values()) {
    if (rows.length > 1) hints.push(`范围 ${chargeScope} 上同一种控制 ${control} 出现 ${rows.length} 行（第 ${rows.join('、')} 行）——（种类 × 范围）唯一，拒绝由服务端说`);
  }
  return hints;
}

export function withControlRowAdded(draft: ControlPolicyDraft): ControlPolicyDraft {
  return { ...draft, controls: [...draft.controls, emptyControlItemDraft()] };
}

export function withControlRowRemoved(draft: ControlPolicyDraft, index: number): ControlPolicyDraft {
  return { ...draft, controls: draft.controls.filter((_, candidate) => candidate !== index) };
}

export function withControlRowPatched(
  draft: ControlPolicyDraft,
  index: number,
  patch: Partial<ControlItemDraft>,
): ControlPolicyDraft {
  return {
    ...draft,
    controls: draft.controls.map((row, candidate) => (candidate === index ? { ...row, ...patch } : row)),
  };
}
