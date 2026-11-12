import { statusToneText } from '../../components/status-tone';
import { businessPartyObjectHash } from './business-parties-address';
import { formatCount, type PartyTileKind } from './business-parties-workbench';
import { ourCompanyLabel } from './business-party-list';
import type { RelationshipGraph, RelationshipGraphNode } from './party-relationship-graph';
import { PartyTile, partyRoleTileKind } from './party-role-visuals';
import { partyNameUnknownNote, partyRoleLabels, relationshipStatusLabels } from './presentation';
import './party-relationship-graph.css';

// 往来单位「关联」签的关系图（票 business-party-relationship-graph/01）。摆法在 party-relationship-graph.ts 的
// relationshipGraphOf；这里只画。
//
// 画法是五列的 CSS 网格：左邻居 · 连线 · 中心 · 连线 · 右邻居，中心纵跨所有行，每个邻居与它那段连线同一行；列宽在
// party-relationship-graph.css。不用画布或 SVG 量坐标：节点数随数据变，网格自己排得开，窄的右栏里也不会错位；每个邻居是一个
// 带完整关系句的链接，读屏照样读得出「谁对谁担任什么、状态如何」，连线只是给眼睛看的，对读屏隐藏。
// 节点与名录的行同一种长相（图标块、名称、编码），一眼认得出图上的这一家就是名录里的那一家。

const roleLabel = (role: string) => partyRoleLabels[role as keyof typeof partyRoleLabels] ?? role;
const statusLabel = (status: string) => relationshipStatusLabels[status as keyof typeof relationshipStatusLabels] ?? status;

/** 待确认的段画虚线、用警示色；已生效的画实线、用主色。合成节点只要有一段待确认就按待确认画，提示组里有没定下来的。 */
function pending(node: RelationshipGraphNode): boolean {
  if (node.kind === 'draft') return false;
  return node.kind === 'group' ? node.candidates > 0 : node.status === 'CANDIDATE';
}

/** 正在填、还没有保存的那一段的连线下方那两个字；图下的说明也用它，两处同一个词。 */
const draftEdgeLabel = '未保存';

/** 连线下方那半句：单段写状态，合成节点写其中待确认几段（全都已生效就不写）。角色写在连线上方，两半分开放，窄栏里也不截断。 */
function edgeStatus(node: RelationshipGraphNode): string | null {
  if (node.kind === 'group') return node.candidates > 0 ? `${node.candidates} 段待确认` : null;
  if (node.kind === 'draft') return draftEdgeLabel;
  return statusLabel(node.status);
}

/**
 * 正在填的那一段（node.kind === 'draft'）也画虚线，但用弱色而不是警示色：它不在册上，不该和「待确认」读成同一种东西，
 * 也不抢已在册各段的颜色。
 */
function Connector({ node }: { node: RelationshipGraphNode }) {
  const waiting = pending(node);
  const draft = node.kind === 'draft';
  return (
    <div
      className={`flex h-12 flex-col justify-center px-1 text-center type-body ${draft ? 'text-idpxyz-breadcrumb' : waiting ? statusToneText.warning : 'text-idpxyz-accent'}`}
      aria-hidden="true"
    >
      <span className="truncate leading-4 text-idpxyz-text">{roleLabel(node.role)}</span>
      <span className={`party-relationship-graph-line relative my-0.5 mr-1.5 block h-0 border-t-2 border-current ${waiting || draft ? 'border-dashed' : ''}`} />
      <span className="min-h-4 truncate leading-4">{edgeStatus(node) ?? ''}</span>
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
  const relation = node.kind === 'draft' ? `担任${roleLabel(node.role)}（正在填，还没有保存）` : `担任${roleLabel(node.role)}（${statusLabel(node.status)}）`;
  return side === 'toward' ? `${other}对${center}${relation}` : `${center}对${other}${relation}`;
}

function Neighbor({
  node,
  center,
  side,
  tileKind,
  onShowDetail,
}: {
  node: RelationshipGraphNode;
  center: string;
  side: 'toward' | 'from';
  tileKind: PartyTileKind;
  onShowDetail: () => void;
}) {
  const label = sentence(node, center, side);
  const box = 'flex h-12 w-full min-w-0 items-center gap-2 rounded-lg border border-idpxyz-border bg-idpxyz-bg px-2.5 text-left';
  const frame = `${box} hover:bg-idpxyz-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent ${
    pending(node) ? 'border-dashed' : ''
  }`;
  const name = 'truncate type-value leading-[18px] text-idpxyz-textBright';
  const sub = 'truncate type-body leading-4 text-idpxyz-breadcrumb';
  // 正在填的那一段不是链接：它还不在册上，点它去那家单位的页会把没保存的表单带走。
  if (node.kind === 'draft') {
    return (
      <div role="img" className={`${box} border-dashed`} aria-label={label} title={label}>
        <PartyTile small kind={tileKind} />
        <span className="flex min-w-0 flex-col">
          <span className={name}>{node.name ?? <span className="text-idpxyz-breadcrumb">{partyNameUnknownNote}</span>}</span>
          <span className={`font-mono ${sub}`}>{node.partyId}</span>
        </span>
      </div>
    );
  }
  if (node.kind === 'group') {
    return (
      <button type="button" className={frame} onClick={onShowDetail} aria-label={label} title={label}>
        <PartyTile small kind={tileKind} />
        <span className="flex min-w-0 flex-col">
          <span className={name}>{formatCount(node.count)} 家单位</span>
          <span className={sub}>点开看明细</span>
        </span>
      </button>
    );
  }
  return (
    <a href={businessPartyObjectHash(node.partyId)} className={frame} aria-label={label} title={label}>
      <PartyTile small kind={tileKind} />
      <span className="flex min-w-0 flex-col">
        <span className={name}>{node.name ?? <span className="text-idpxyz-breadcrumb">{partyNameUnknownNote}</span>}</span>
        <span className={`font-mono ${sub}`}>{node.partyId}</span>
      </span>
    </a>
  );
}

export function PartyRelationshipGraph({
  graph,
  centerName,
  centerId,
  ours,
  centerKind,
  tileKindOf,
  onShowDetail,
  onShowEnded,
}: {
  graph: RelationshipGraph;
  /** 中心那家的名称；不知道时显标识。 */
  centerName: string | null;
  centerId: string;
  /** 法人册上有公司主体钉着它：中心标「我方公司」。 */
  ours: boolean;
  /** 中心那家的图标块；不给时我方公司按我方公司画，其余按「都不是」画。 */
  centerKind?: PartyTileKind;
  /**
   * 邻居的图标块按它自己的类型取（与名录的行同一份）：调用方认得这家单位就交出来，认不得交 null。不给或交 null 就按这段关系猜：对中心担任角色的
   * 一侧按那个角色画，另一侧不知道它是什么，按「都不是」画。
   */
  tileKindOf?: (partyId: string) => PartyTileKind | null;
  /** 合成节点点了去哪看这一组的明细。 */
  onShowDetail: () => void;
  /** 「已结束」那句点了去哪看；不给就与合成节点同一处。 */
  onShowEnded?: () => void;
}) {
  const center = centerName ?? centerId;
  const rows = Math.max(graph.towardIt.length, graph.fromIt.length, 1);
  const startOf = (count: number) => Math.floor((rows - Math.max(count, 1)) / 2) + 1;
  const towardStart = startOf(graph.towardIt.length);
  const fromStart = startOf(graph.fromIt.length);
  const tileOf = (node: RelationshipGraphNode, side: 'toward' | 'from'): PartyTileKind => {
    if (node.kind === 'group') return partyRoleTileKind(node.role);
    return tileKindOf?.(node.partyId) ?? (side === 'toward' ? partyRoleTileKind(node.role) : 'NONE');
  };
  const hasDraft = [...graph.towardIt, ...graph.fromIt].some((node) => node.kind === 'draft');

  return (
    <div className="party-relationship-graph">
      <div className="party-relationship-graph-columns mb-2 type-body text-idpxyz-breadcrumb">
        <span>对它担任角色的单位</span>
        <span className="col-start-5 text-right">它对其担任角色的单位</span>
      </div>
      <div
        role="group"
        aria-label={`${center}的合作关系图`}
        className="party-relationship-graph-columns items-center gap-y-2"
        style={{ gridTemplateRows: `repeat(${rows}, minmax(3rem, auto))` }}
      >
        {graph.towardIt.length === 0 ? (
          <p style={{ gridColumn: '1 / span 2', gridRow: towardStart }} className="type-body text-idpxyz-breadcrumb">
            还没有单位对它担任角色
          </p>
        ) : (
          graph.towardIt.map((node, index) => (
            <div key={node.key} className="contents">
              <div style={{ gridColumn: 1, gridRow: towardStart + index }} className="min-w-0">
                <Neighbor node={node} center={center} side="toward" tileKind={tileOf(node, 'toward')} onShowDetail={onShowDetail} />
              </div>
              <div style={{ gridColumn: 2, gridRow: towardStart + index }} className="min-w-0">
                <Connector node={node} />
              </div>
            </div>
          ))
        )}

        <div
          style={{ gridColumn: 3, gridRow: `1 / span ${rows}` }}
          className="mx-1.5 flex min-h-[72px] min-w-0 flex-col items-center justify-center gap-1.5 self-stretch rounded-[10px] border border-idpxyz-accent bg-idpxyz-activeItem px-2.5 py-3 text-center"
        >
          <PartyTile kind={centerKind ?? (ours ? 'OURS' : 'NONE')} />
          <span className="line-clamp-2 max-w-full break-words type-value leading-[18px] text-idpxyz-textBright" title={center}>
            {center}
          </span>
          <span className="type-body leading-4 text-idpxyz-breadcrumb">{ours ? ourCompanyLabel : '本单位'}</span>
        </div>

        {graph.fromIt.length === 0 ? (
          <p style={{ gridColumn: '4 / span 2', gridRow: fromStart }} className="text-right type-body text-idpxyz-breadcrumb">
            它还没有对谁担任角色
          </p>
        ) : (
          graph.fromIt.map((node, index) => (
            <div key={node.key} className="contents">
              <div style={{ gridColumn: 4, gridRow: fromStart + index }} className="min-w-0">
                <Connector node={node} />
              </div>
              <div style={{ gridColumn: 5, gridRow: fromStart + index }} className="min-w-0">
                <Neighbor node={node} center={center} side="from" tileKind={tileOf(node, 'from')} onShowDetail={onShowDetail} />
              </div>
            </div>
          ))
        )}
      </div>
      <p className="mt-2.5 type-body text-idpxyz-breadcrumb">
        箭头从担任角色的一方指向对方；虚线是待确认的关系。
        {hasDraft ? `标着「${draftEdgeLabel}」的灰色虚线是正在填的这一段，保存之后才算数。` : null}
        {graph.ended > 0 ? (
          <>
            {' '}
            另有 {graph.ended} 段已结束的关系不上图，
            <button type="button" className="text-[color:var(--ds-text-link-hover)] hover:underline" onClick={onShowEnded ?? onShowDetail}>
              在明细里看
            </button>
            。
          </>
        ) : null}
      </p>
    </div>
  );
}
