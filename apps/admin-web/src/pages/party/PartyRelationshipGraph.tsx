import { ArrowRight, Building2, Users } from 'lucide-react';
import { statusToneText } from '../../components/status-tone';
import { businessPartyObjectHash } from './business-parties-address';
import type { RelationshipGraph, RelationshipGraphNode } from './party-relationship-graph';
import { partyNameUnknownNote, partyRoleLabels, relationshipStatusLabels } from './presentation';

// 往来单位「关联」签的关系图（票 business-party-relationship-graph/01）。摆法在 party-relationship-graph.ts 的
// relationshipGraphOf；这里只画。
//
// 画法是五列的 CSS 网格：左邻居 · 连线 · 中心 · 连线 · 右邻居，中心纵跨所有行，每个邻居与它那段连线同一行。不用画布或
// SVG 量坐标：节点数随数据变，网格自己排得开，窄的右栏里也不会错位；每个邻居是一个带完整关系句的链接，读屏照样读得出
// 「谁对谁担任什么、状态如何」，连线只是给眼睛看的，对读屏隐藏。

const roleLabel = (role: string) => partyRoleLabels[role as keyof typeof partyRoleLabels] ?? role;
const statusLabel = (status: string) => relationshipStatusLabels[status as keyof typeof relationshipStatusLabels] ?? status;

/** 待确认的段画虚线、用警示色；已生效的画实线、用主色。合成节点只要有一段待确认就按待确认画，提示组里有没定下来的。 */
function pending(node: RelationshipGraphNode): boolean {
  return node.kind === 'group' ? node.candidates > 0 : node.status === 'CANDIDATE';
}

/** 连线下方那半句：单段写状态，合成节点写其中待确认几段（全都已生效就不写）。角色写在连线上方，两半分开放，窄栏里也不截断。 */
function edgeStatus(node: RelationshipGraphNode): string | null {
  if (node.kind === 'group') return node.candidates > 0 ? `${node.candidates} 段待确认` : null;
  return statusLabel(node.status);
}

function Connector({ node }: { node: RelationshipGraphNode }) {
  const tone = pending(node) ? statusToneText.warning : 'text-idpxyz-accent';
  return (
    <div className={`relative flex h-full min-h-[3.5rem] items-center ${tone}`} aria-hidden="true">
      <span className={`w-full border-t-2 border-current ${pending(node) ? 'border-dashed' : ''}`} />
      <ArrowRight className="absolute -right-1 h-3.5 w-3.5" />
      <span className="absolute bottom-1/2 left-1/2 mb-1 -translate-x-1/2 whitespace-nowrap type-caption text-idpxyz-text">
        {roleLabel(node.role)}
      </span>
      {edgeStatus(node) ? (
        <span className="absolute left-1/2 top-1/2 mt-1 -translate-x-1/2 whitespace-nowrap type-caption">{edgeStatus(node)}</span>
      ) : null}
    </div>
  );
}

/** 关系句：左邻居是持有方、中心是相对方；右邻居反过来。读屏与悬停都读这一句。 */
function sentence(node: RelationshipGraphNode, center: string, side: 'toward' | 'from'): string {
  if (node.kind === 'group') {
    const who = `${node.count} 家单位`;
    const tail = node.candidates > 0 ? `，其中 ${node.candidates} 段待确认` : '';
    return side === 'toward' ? `${who}对${center}担任${roleLabel(node.role)}${tail}；点开看明细` : `${center}对${who}担任${roleLabel(node.role)}${tail}；点开看明细`;
  }
  const other = node.name ?? `${node.partyId}（${partyNameUnknownNote}）`;
  const relation = `担任${roleLabel(node.role)}（${statusLabel(node.status)}）`;
  return side === 'toward' ? `${other}对${center}${relation}` : `${center}对${other}${relation}`;
}

function Neighbor({
  node,
  center,
  side,
  onShowDetail,
}: {
  node: RelationshipGraphNode;
  center: string;
  side: 'toward' | 'from';
  onShowDetail: () => void;
}) {
  const label = sentence(node, center, side);
  const frame = `flex w-full min-w-0 flex-col gap-0.5 rounded-md border px-2.5 py-1.5 text-left hover:bg-idpxyz-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent ${
    pending(node) ? 'border-dashed border-idpxyz-border' : 'border-idpxyz-border'
  } bg-idpxyz-bg`;
  if (node.kind === 'group') {
    return (
      <button type="button" className={frame} onClick={onShowDetail} aria-label={label} title={label}>
        <span className="flex items-center gap-1.5 type-body text-idpxyz-text">
          <Users className="h-3.5 w-3.5 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
          <span className="truncate">
            {roleLabel(node.role)} · {node.count} 家
          </span>
        </span>
        <span className="type-caption text-idpxyz-breadcrumb">点开看明细</span>
      </button>
    );
  }
  return (
    <a href={businessPartyObjectHash(node.partyId)} className={frame} aria-label={label} title={label}>
      <span className="line-clamp-2 break-words type-body text-idpxyz-text">
        {node.name ?? <span className="text-idpxyz-breadcrumb">{partyNameUnknownNote}</span>}
      </span>
      <span className="truncate font-mono type-caption text-idpxyz-breadcrumb">{node.partyId}</span>
    </a>
  );
}

function EmptySide({ text }: { text: string }) {
  return <p className="rounded-md border border-dashed border-idpxyz-border px-2.5 py-1.5 type-caption text-idpxyz-breadcrumb">{text}</p>;
}

export function PartyRelationshipGraph({
  graph,
  centerName,
  centerId,
  ours,
  onShowDetail,
}: {
  graph: RelationshipGraph;
  /** 中心那家的名称；不知道时显标识。 */
  centerName: string | null;
  centerId: string;
  /** 法人册上有公司主体钉着它：中心标「我方公司」。 */
  ours: boolean;
  /** 合成节点与「已结束」那句点了去明细。 */
  onShowDetail: () => void;
}) {
  const center = centerName ?? centerId;
  const rows = Math.max(graph.towardIt.length, graph.fromIt.length, 1);
  const startOf = (count: number) => Math.floor((rows - Math.max(count, 1)) / 2) + 1;
  const towardStart = startOf(graph.towardIt.length);
  const fromStart = startOf(graph.fromIt.length);

  return (
    <div>
      <div className="mb-1.5 grid grid-cols-[minmax(0,1fr)_4.5rem_minmax(0,1fr)_4.5rem_minmax(0,1fr)] type-caption text-idpxyz-breadcrumb">
        <span>对它担任角色的单位</span>
        <span />
        <span className="text-center">这家单位</span>
        <span />
        <span className="text-right">它对其担任角色的单位</span>
      </div>
      <div
        role="group"
        aria-label={`${center}的合作关系图`}
        className="grid grid-cols-[minmax(0,1fr)_4.5rem_minmax(0,1fr)_4.5rem_minmax(0,1fr)] items-center gap-y-2"
        style={{ gridTemplateRows: `repeat(${rows}, minmax(3.5rem, auto))` }}
      >
        {graph.towardIt.length === 0 ? (
          <div style={{ gridColumn: 1, gridRow: towardStart }}>
            <EmptySide text="还没有单位对它担任角色" />
          </div>
        ) : (
          graph.towardIt.map((node, index) => (
            <div key={node.key} className="contents">
              <div style={{ gridColumn: 1, gridRow: towardStart + index }} className="min-w-0">
                <Neighbor node={node} center={center} side="toward" onShowDetail={onShowDetail} />
              </div>
              <div style={{ gridColumn: 2, gridRow: towardStart + index }} className="h-full px-1">
                <Connector node={node} />
              </div>
            </div>
          ))
        )}

        <div style={{ gridColumn: 3, gridRow: `1 / span ${rows}` }} className="min-w-0 self-center">
          <div className="flex min-w-0 flex-col items-center gap-1 rounded-md border-2 border-idpxyz-accent bg-idpxyz-bg px-3 py-2 text-center">
            <Building2 className="h-4 w-4 text-idpxyz-accent" aria-hidden="true" />
            <span className="line-clamp-2 max-w-full break-words type-body font-medium text-idpxyz-textBright" title={center}>
              {center}
            </span>
            <span className="max-w-full truncate font-mono type-caption text-idpxyz-breadcrumb">{centerId}</span>
            {ours ? <span className="rounded border border-idpxyz-border px-1.5 type-caption text-idpxyz-text">我方公司</span> : null}
          </div>
        </div>

        {graph.fromIt.length === 0 ? (
          <div style={{ gridColumn: 5, gridRow: fromStart }}>
            <EmptySide text="它还没有对谁担任角色" />
          </div>
        ) : (
          graph.fromIt.map((node, index) => (
            <div key={node.key} className="contents">
              <div style={{ gridColumn: 4, gridRow: fromStart + index }} className="h-full px-1">
                <Connector node={node} />
              </div>
              <div style={{ gridColumn: 5, gridRow: fromStart + index }} className="min-w-0">
                <Neighbor node={node} center={center} side="from" onShowDetail={onShowDetail} />
              </div>
            </div>
          ))
        )}
      </div>
      <p className="mt-2 type-caption text-idpxyz-breadcrumb">
        箭头从担任角色的一方指向对方；虚线是待确认的关系。
        {graph.ended > 0 ? (
          <>
            {' '}
            另有 {graph.ended} 段已结束的关系不上图，
            <button type="button" className="text-idpxyz-accent hover:underline" onClick={onShowDetail}>
              在明细里看
            </button>
            。
          </>
        ) : null}
      </p>
    </div>
  );
}
