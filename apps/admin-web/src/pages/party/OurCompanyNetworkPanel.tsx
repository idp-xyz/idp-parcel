import { Building2, ChevronDown, ChevronRight, Link2, Route, Users, X, type LucideIcon } from 'lucide-react';
import { Button, DropdownMenu, MenuItem } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import type { BusinessPartyRecord } from './api';
import { formatCount, type PartyTileKind } from './business-parties-workbench';
import { BusinessPartyTypeTags } from './BusinessPartyHeaderParts';
import { PanelBand, panelLinkClass } from './BusinessPartyPanelParts';
import { RefText } from './copy-affordance';
import { Fact } from './detail-primitives';
import { objectHash as legalEntityObjectHash } from './group-legal-entities-address';
import { ourCompanyLiveText, roleCountNoteOf, roleCountsBySide, type OurCompanyNetworkRow, type RoleCount } from './our-company-network';
import { OverviewSectionStateNote } from './PartyOverviewSection';
import { relationshipsOfParty, sectionOf } from './party-overview';
import { PartyRelationshipGraph } from './PartyRelationshipGraph';
import { relationshipGraphOf, type RelationshipGraphLimits } from './party-relationship-graph';
import { partyRelationshipsComplete } from './party-relationships-of';
import { PartyTile, partyRoleTileKind } from './party-role-visuals';
import { labelOf, partyRoleLabels } from './presentation';
import { usePartyRelationships } from './use-party-relationships';

// 「我方关联」一册的右栏（票 business-party-relationship-graph/02 立；外框与两张汇总自票 business-party-redesign/04）：以选中的
// 那家我方公司为中心。头区与往来单位右栏同一种排法——图标块、名称与状态、类型标签，右端是去处与关闭，下面一行事实；正文通栏
// 三段，段头是带底色的窄带（BusinessPartyPanelParts.tsx 的 PanelBand）：关系图，再按「谁对它担任角色」「它对谁担任角色」
// 各一张按角色的汇总，每行点了去合作关系视图看这家公司的这一类关系。
//
// 图与汇总画的是这家公司名下的关系，栏里按它筛着读（票 business-party-redesign/09，party-relationships-of.ts）：不从合作关系册
// 的第一页里挑，册再大也读得全。段头上「几段在用」是读口在这家公司那一行上答的数。
//
// 图与往来单位「关联」签是同一个组件、同一套摆法（party-relationship-graph.ts）；这里地方更大，合成门槛放宽一些，一家公司名下
// 十来个客户也逐家摆出。
const networkLimits: RelationshipGraphLimits = { role: 8, column: 12 };

/** 按角色的汇总：一个角色一行——图标块、角色与一句小字、段数，点了去合作关系视图看这家公司的这一类关系。 */
function RoleSummary({
  icon,
  title,
  counts,
  emptyNote,
  onPick,
}: {
  icon: LucideIcon;
  title: string;
  counts: readonly RoleCount[];
  emptyNote: string;
  onPick: (role: string) => void;
}) {
  return (
    <section className="border-b border-idpxyz-border">
      <PanelBand icon={icon} title={title} />
      {counts.length === 0 ? (
        <p className="px-6 py-3 type-body leading-5 text-idpxyz-breadcrumb">{emptyNote}</p>
      ) : (
        <ul>
          {counts.map((count) => (
            <li key={count.role} className="border-b border-idpxyz-border last:border-b-0">
              <button
                type="button"
                title="在「合作关系」里看这家公司的这一类关系"
                onClick={() => onPick(count.role)}
                className="flex w-full items-center gap-2.5 px-6 py-2.5 text-left hover:bg-idpxyz-hover focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-idpxyz-accent"
              >
                <PartyTile small kind={partyRoleTileKind(count.role)} />
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate type-value leading-5 text-idpxyz-text">{labelOf(partyRoleLabels, count.role)}</span>
                  <span className="truncate type-body leading-4 text-idpxyz-breadcrumb">{roleCountNoteOf(count)}</span>
                </span>
                <span className="shrink-0 whitespace-nowrap type-value tabular-nums text-idpxyz-textBright">{formatCount(count.count)} 段</span>
                <ChevronRight className="h-4 w-4 shrink-0 text-idpxyz-breadcrumb opacity-60" aria-hidden="true" />
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

export function OurCompanyNetworkPanel({
  row,
  party,
  roles,
  tileKindOf,
  onClose,
  onShowRelationships,
  onShowInRoster,
}: {
  row: OurCompanyNetworkRow;
  /** 这家公司在往来单位读口上的那一行，头区的状态取它；这一视图的行就是从那一问来的，读到之前是 null，状态先不摆。 */
  party: BusinessPartyRecord | null;
  /** 它持有的角色（读口在那一行上答的类别），头区的类型标签按它称呼；「我方公司」那一枚总在。 */
  roles: readonly string[];
  /** 关系图上各家单位的图标块按它自己的类型取，与名录的行同一份；页面认不得的交 null，由图按这段关系里的角色画。 */
  tileKindOf?: (partyId: string) => PartyTileKind | null;
  onClose: () => void;
  /** 去合作关系视图看这家公司名下的关系；给了角色就连角色一起带上。 */
  onShowRelationships: (role?: string) => void;
  /** 去单位名录，选中这一家。 */
  onShowInRoster: () => void;
}) {
  const name = row.partyName;
  // 这家公司名下的合作关系：没答成时图画不出来，原处说明是怎么回事，不拿一张空图冒充「没有关系」。
  const own = usePartyRelationships(row.partyId);
  const relationshipState = sectionOf(own.answer, (body) => relationshipsOfParty(row.partyId, body.relationships));
  const complete = partyRelationshipsComplete(own.answer);
  const openLegalEntity = (legalEntityId: string) => {
    window.location.hash = legalEntityObjectHash(legalEntityId);
  };
  const actionClass = 'type-body text-idpxyz-breadcrumb';
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`${name}的关联`}>
      <div className="space-y-3 border-b border-idpxyz-border px-6 pb-3.5 pt-4">
        {/* 与往来单位右栏的头区同一种排法：名称那一块要 280 才排得开，放不下时动作整组折到下一行、靠右。 */}
        <div className="flex flex-wrap items-start gap-x-3.5 gap-y-2.5">
          <PartyTile large kind="OURS" />
          <div className="min-w-0 flex-[1_1_280px]">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <h2 className="line-clamp-2 min-w-0 break-words type-display font-semibold leading-7 text-idpxyz-textBright" title={name}>
                {name}
              </h2>
              {party ? <StatusPill code={party.status} /> : null}
            </div>
            <div className="mt-1.5">
              <BusinessPartyTypeTags ours roles={roles} />
            </div>
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-1.5">
            {/* 钉着这家公司的公司主体通常只有一个，按钮直接去；不止一个时展开选哪一个。编号在下面那一行事实里。 */}
            {row.legalEntityIds.length === 1 ? (
              <Button size="sm" variant="outline" className={actionClass} onClick={() => openLegalEntity(row.legalEntityIds[0])}>
                <Building2 className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                打开公司主体
              </Button>
            ) : row.legalEntityIds.length > 1 ? (
              <DropdownMenu
                align="right"
                trigger={
                  <Button size="sm" variant="outline" className={actionClass}>
                    <Building2 className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                    打开公司主体
                    <ChevronDown className="ml-1 h-3.5 w-3.5" aria-hidden="true" />
                  </Button>
                }
              >
                {row.legalEntityIds.map((id) => (
                  <MenuItem key={id} onSelect={() => openLegalEntity(id)}>
                    <span className="font-mono">{id}</span>
                  </MenuItem>
                ))}
              </DropdownMenu>
            ) : null}
            <Button size="sm" variant="outline" className={actionClass} onClick={onShowInRoster}>
              在单位名录里看
            </Button>
            <Button className="text-idpxyz-breadcrumb" variant="ghost" size="icon" onClick={onClose} aria-label="关闭详情" title="关闭（Esc）">
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        </div>
        <dl className="flex min-w-0 flex-wrap items-center gap-x-5 gap-y-1">
          <Fact inline label="编码">
            <RefText value={row.partyId} name="编码" />
          </Fact>
          {/* 法人册没读到时编号列不出来，这一格不摆：不留一个空着的标签。 */}
          {row.legalEntityIds.length > 0 ? (
            <Fact inline label="公司主体">
              {row.legalEntityIds.map((id, index) => (
                <span key={id}>
                  {index > 0 ? '、' : null}
                  <RefText value={id} name="公司主体编号" />
                </span>
              ))}
            </Fact>
          ) : null}
        </dl>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <section className="border-b border-idpxyz-border">
          <PanelBand icon={Link2} title="关系图" count={ourCompanyLiveText(row)}>
            <button type="button" className={panelLinkClass} onClick={() => onShowRelationships()}>
              在「合作关系」里看全部
            </button>
          </PanelBand>
          {relationshipState.kind !== 'answered' ? (
            <div className="px-6 py-4">
              <OverviewSectionStateNote state={relationshipState} what="合作关系" retry={own.retry} />
            </div>
          ) : (
            <>
              {complete ? null : (
                <p className="border-b border-idpxyz-border px-6 py-2.5 type-body leading-5 text-idpxyz-breadcrumb">
                  这家公司名下的合作关系不止一页，图与下面按角色的段数只按已取到的那一部分。
                </p>
              )}
              <div className="overflow-x-auto px-6 pb-[18px] pt-4">
                <PartyRelationshipGraph
                  graph={relationshipGraphOf(relationshipState.rows, networkLimits)}
                  centerName={row.partyName}
                  centerId={row.partyId}
                  ours
                  tileKindOf={tileKindOf}
                  onShowDetail={() => onShowRelationships()}
                />
              </div>
            </>
          )}
        </section>
        {relationshipState.kind === 'answered' ? (
          <>
            <RoleSummary
              icon={Users}
              title="对它担任角色的单位"
              counts={roleCountsBySide(relationshipState.rows).towardIt}
              emptyNote="还没有单位对它担任角色。"
              onPick={onShowRelationships}
            />
            <RoleSummary
              icon={Route}
              title="它对别的单位担任的角色"
              counts={roleCountsBySide(relationshipState.rows).fromIt}
              emptyNote="它还没有对谁担任角色。"
              onPick={onShowRelationships}
            />
          </>
        ) : null}
      </div>
    </div>
  );
}
