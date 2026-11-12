// 往来单位「关联」签的关系图怎么摆（票 business-party-relationship-graph/01）。纯函数，node:test 钉着；组件
// PartyRelationshipGraph.tsx 只负责画。
//
// 形状取法：参考原型（主树 ux/IDP_Parcel_Partner_Fiori_Prototype_V1.0.html）的「业务关系视图」画的是「我方主体 → 外部单位」
// 一对节点，前提是关系总在我方与对方之间。本仓的合作关系是「持有方 · 担任某角色 · 对相对方」（CONTEXT「参与方关系」），相对方
// 不一定是我方公司——授权交付方就是对货主担任的；我方公司名下又可能挂着上百个客户。所以这里画以这家单位为中心的一张图：
// 左列是对它担任角色的单位，右列是它对其担任角色的单位，箭头一律从持有方指向相对方，左右读下来就是关系册「持有方 → 相对方」
// 的字段次序。同一角色的单位多了合成一个计数节点，不让一列拖成长表；结束了的关系不上图，只报段数，明细里看。

import { liveRelationshipStatuses, type PartyRelationshipView } from './party-overview';
import { partyRoleLabels } from './presentation';

/** 同一侧同一角色超过这么多家就合成一个计数节点：再多，图就退化成一张竖着的名单。 */
export const relationshipGraphRoleLimit = 3;
/** 一侧最多摆几个节点；超了就把这一侧每个角色都合成计数节点。 */
export const relationshipGraphColumnLimit = 6;

/** 图上的一个邻居：一段关系的另一方，或同一角色合成的一组。 */
export type RelationshipGraphNode =
  | {
      kind: 'party';
      key: string;
      partyId: string;
      /** 名称转写不到时为 null，组件照「名称未知」那一套显标识。 */
      name: string | null;
      role: string;
      status: string;
    }
  | {
      kind: 'group';
      key: string;
      role: string;
      count: number;
      /** 组里待确认的段数：连线画成虚线要看它。 */
      candidates: number;
    }
  | {
      /** 登记表单里正在填、还没有保存的那一段（票 business-party-redesign/11）：不在册上，只是让人填的时候看得见它会落在哪。 */
      kind: 'draft';
      key: string;
      partyId: string;
      /** 候选里认不出这个标识（手填的、读面没读回）时为 null。 */
      name: string | null;
      role: string;
    };

export interface RelationshipGraph {
  /** 对这家单位担任角色的一侧（它是相对方），画在左边。 */
  towardIt: RelationshipGraphNode[];
  /** 这家单位对其担任角色的一侧（它是持有方），画在右边。 */
  fromIt: RelationshipGraphNode[];
  /** 已结束（到期、撤销、替代）的段数：不上图，图下报一句。 */
  ended: number;
}

const roleOrder = Object.keys(partyRoleLabels);

function roleRank(role: string): number {
  const rank = roleOrder.indexOf(role);
  return rank === -1 ? roleOrder.length : rank;
}

function partyNode(view: PartyRelationshipView): RelationshipGraphNode {
  return {
    kind: 'party',
    key: view.relationshipId,
    partyId: view.otherPartyId,
    name: view.otherPartyNameKnown && view.otherPartyName ? view.otherPartyName : null,
    role: view.role,
    status: view.status,
  };
}

function groupNode(side: string, role: string, views: readonly PartyRelationshipView[]): RelationshipGraphNode {
  return {
    kind: 'group',
    key: `${side}:${role}`,
    role,
    count: views.length,
    candidates: views.filter((view) => view.status === 'CANDIDATE').length,
  };
}

/** 合成的门槛。缺省照上面两个常量；地方更大的视图（「我方关联」的右栏）可以放宽一些。 */
export interface RelationshipGraphLimits {
  role: number;
  column: number;
}

const defaultLimits: RelationshipGraphLimits = { role: relationshipGraphRoleLimit, column: relationshipGraphColumnLimit };

function columnOf(side: string, views: readonly PartyRelationshipView[], limits: RelationshipGraphLimits): RelationshipGraphNode[] {
  const byRole = new Map<string, PartyRelationshipView[]>();
  for (const view of views) byRole.set(view.role, [...(byRole.get(view.role) ?? []), view]);
  const roles = [...byRole.keys()].sort((left, right) => roleRank(left) - roleRank(right) || left.localeCompare(right));
  const nameOf = (view: PartyRelationshipView) => (view.otherPartyNameKnown && view.otherPartyName) || view.otherPartyId;
  const spelled = (collapseAll: boolean) =>
    roles.flatMap((role) => {
      const members = byRole.get(role) ?? [];
      if (members.length > limits.role || (collapseAll && members.length > 1)) return [groupNode(side, role, members)];
      return [...members].sort((left, right) => nameOf(left).localeCompare(nameOf(right), 'zh-CN')).map(partyNode);
    });
  const nodes = spelled(false);
  return nodes.length > limits.column ? spelled(true) : nodes;
}

/** 把一家单位名下的关系摆成图：只摆还在起作用的段（liveRelationshipStatuses），左右按它在关系里站哪一侧分。 */
export function relationshipGraphOf(
  views: readonly PartyRelationshipView[],
  limits: RelationshipGraphLimits = defaultLimits,
): RelationshipGraph {
  const live = views.filter((view) => liveRelationshipStatuses.has(view.status));
  return {
    towardIt: columnOf('toward', live.filter((view) => view.side === 'COUNTERPARTY'), limits),
    fromIt: columnOf('from', live.filter((view) => view.side === 'HOLDER'), limits),
    ended: views.length - live.length,
  };
}

/** 正在填的那一段在图上的样子：中心这家单位站哪一侧、另一方是谁、什么角色。 */
export interface RelationshipGraphDraft {
  /** 中心这家单位在这一段里站哪一侧：它担任角色（HOLDER，另一方画在右边），还是别人对它担任（COUNTERPARTY，画在左边）。 */
  side: 'HOLDER' | 'COUNTERPARTY';
  otherPartyId: string;
  otherPartyName: string | null;
  role: string;
}

/**
 * 登记表单里的三格折成图上那一段。双方与角色都选了、其中一方正是中心这家单位、另一方不是它自己，才画得出来；少一样就是
 * null——图上不摆半段。这只是给填的人看的预览，不是校验：双方能不能是这两家、角色配不配，照旧送上去由登记册答。
 */
export function relationshipGraphDraftOf(
  centerId: string,
  form: { holder: string; counterparty: string; role: string },
  nameOf: (partyId: string) => string | null,
): RelationshipGraphDraft | null {
  const holder = form.holder.trim();
  const counterparty = form.counterparty.trim();
  const role = form.role.trim();
  if (role === '' || holder === '' || counterparty === '' || holder === counterparty) return null;
  if (holder === centerId) return { side: 'HOLDER', otherPartyId: counterparty, otherPartyName: nameOf(counterparty), role };
  if (counterparty === centerId) return { side: 'COUNTERPARTY', otherPartyId: holder, otherPartyName: nameOf(holder), role };
  return null;
}

/**
 * 把正在填的那一段并进图：排在它那一侧的最后——已在册的各段不因为人在填表而换位置，新的一段出现在末尾。不参与合成：同一
 * 角色已经合成了计数节点时它也单独一枚，人要看见的正是「这一段」。没有草稿就原样交回。
 */
export function relationshipGraphWithDraft(graph: RelationshipGraph, draft: RelationshipGraphDraft | null): RelationshipGraph {
  if (draft === null) return graph;
  const node: RelationshipGraphNode = { kind: 'draft', key: 'draft', partyId: draft.otherPartyId, name: draft.otherPartyName, role: draft.role };
  return draft.side === 'HOLDER' ? { ...graph, fromIt: [...graph.fromIt, node] } : { ...graph, towardIt: [...graph.towardIt, node] };
}

/**
 * 图排成哪一种：两侧都有邻居（both），只有对它担任角色的一侧有（toward-only），只有它对其担任角色的一侧有（from-only），
 * 两侧都没有（none）。只有一侧有时图不为空着的那一侧留位置，三列靠左排（PartyRelationshipGraph.tsx）——大多数单位都是这一种：
 * 客户只对我方担任角色，我方公司多半只是别人的相对方；照五列排半张图是空的，窄栏里有内容的那一半名称却被截断。正在填的那一段
 * 也算邻居：它落在空着的那一侧时，图随之展开成两侧。
 */
export type RelationshipGraphShape = 'both' | 'toward-only' | 'from-only' | 'none';

export function relationshipGraphShapeOf(graph: Pick<RelationshipGraph, 'towardIt' | 'fromIt'>): RelationshipGraphShape {
  const toward = graph.towardIt.length > 0;
  const from = graph.fromIt.length > 0;
  if (toward && from) return 'both';
  if (toward) return 'toward-only';
  return from ? 'from-only' : 'none';
}
