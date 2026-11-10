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
