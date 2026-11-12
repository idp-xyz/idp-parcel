import { Building2, ChevronDown, ChevronRight, Link2, Route, Users, X, type LucideIcon } from 'lucide-react';
import { Button, DropdownMenu, MenuItem } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import type { ApiResult } from '../catalogue-api';
import type { BusinessPartyRecord, PartyRelationshipListResponseBody } from './api';
import { formatCount, type PartyTileKind } from './business-parties-workbench';
import { BusinessPartyTypeTags } from './BusinessPartyHeaderParts';
import { PanelBand, panelLinkClass } from './BusinessPartyPanelParts';
import { RefText } from './copy-affordance';
import { Fact } from './detail-primitives';
import { objectHash as legalEntityObjectHash } from './group-legal-entities-address';
import { ourCompanyLiveText, roleCountNoteOf, type OurCompanyNetworkRow, type RoleCount } from './our-company-network';
import { OverviewSectionStateNote } from './PartyOverviewSection';
import { sectionOf } from './party-overview';
import { PartyRelationshipGraph } from './PartyRelationshipGraph';
import { relationshipGraphOf, type RelationshipGraphLimits } from './party-relationship-graph';
import { PartyTile, partyRoleTileKind } from './party-role-visuals';
import { labelOf, partyRoleLabels } from './presentation';

// 「我方关联」一册的右栏（票 business-party-relationship-graph/02 立；外框与两张汇总自票 business-party-redesign/04）：以选中的
// 那家我方公司为中心。头区与往来单位右栏同一种排法——图标块、名称与状态、类型标签，右端是去处与关闭，下面一行事实；正文通栏
// 三段，段头是带底色的窄带（BusinessPartyPanelParts.tsx 的 PanelBand）：关系图，再按「谁对它担任角色」「它对谁担任角色」
// 各一张按角色的汇总，每行点了去合作关系视图看这家公司的这一类关系。
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
  relationships,
  relationshipsComplete,
  retryRelationships,
  tileKindOf,
  onClose,
  onShowRelationships,
  onShowInRoster,
}: {
  row: OurCompanyNetworkRow;
  /**
   * 这家公司在往来单位册上的那一行，头区的状态取它。页面只在不带条件的那一页里找：册不止一页、那一页里没有它时是 null，
   * 状态不摆——不拿公司主体的状态顶，那是另一本册上的另一个状态。
   */
  party: BusinessPartyRecord | null;
  /** 它持有的角色（与名录类型格同一份索引），头区的类型标签按它称呼；「我方公司」那一枚总在。 */
  roles: readonly string[];
  /** 合作关系册不带条件那一问的答复：没答成时图画不出来，原处说明是怎么回事，不拿一张空图冒充「没有关系」。 */
  relationships: ApiResult<PartyRelationshipListResponseBody> | null;
  /** 那一问已是整册（complete-register.ts）：不是就写明图与数只按到手的那一页。 */
  relationshipsComplete: boolean;
  retryRelationships: () => void;
  /** 关系图上各家单位的图标块按它自己的类型取，与名录的行同一份。 */
  tileKindOf?: (partyId: string) => PartyTileKind | null;
  onClose: () => void;
  /** 去合作关系视图看这家公司名下的关系；给了角色就连角色一起带上。 */
  onShowRelationships: (role?: string) => void;
  /** 去单位名录，选中这一家。 */
  onShowInRoster: () => void;
}) {
  const name = row.partyName ?? row.partyId;
  // 只借段状态的那几种说法（还在读、未配置、无权、出错）；答出来之后的行在 row 里，这里不再切一遍。
  const relationshipState = sectionOf(relationships, () => []);
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
          <Fact inline label="公司主体">
            {row.legalEntityIds.map((id, index) => (
              <span key={id}>
                {index > 0 ? '、' : null}
                <RefText value={id} name="公司主体编号" />
              </span>
            ))}
          </Fact>
        </dl>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <section className="border-b border-idpxyz-border">
          <PanelBand icon={Link2} title="关系图" count={relationshipState.kind === 'answered' ? ourCompanyLiveText(row) : undefined}>
            <button type="button" className={panelLinkClass} onClick={() => onShowRelationships()}>
              在「合作关系」里看全部
            </button>
          </PanelBand>
          {relationshipState.kind !== 'answered' ? (
            <div className="px-6 py-4">
              <OverviewSectionStateNote state={relationshipState} what="合作关系" retry={retryRelationships} />
            </div>
          ) : (
            <>
              {relationshipsComplete ? null : (
                <p className="border-b border-idpxyz-border px-6 py-2.5 type-body leading-5 text-idpxyz-breadcrumb">
                  合作关系册不止一页，图与下面的段数只按已取到的那一页。
                </p>
              )}
              <div className="overflow-x-auto px-6 pb-[18px] pt-4">
                <PartyRelationshipGraph
                  graph={relationshipGraphOf(row.views, networkLimits)}
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
              counts={row.towardIt}
              emptyNote="还没有单位对它担任角色。"
              onPick={onShowRelationships}
            />
            <RoleSummary
              icon={Route}
              title="它对别的单位担任的角色"
              counts={row.fromIt}
              emptyNote="它还没有对谁担任角色。"
              onPick={onShowRelationships}
            />
          </>
        ) : null}
      </div>
    </div>
  );
}
