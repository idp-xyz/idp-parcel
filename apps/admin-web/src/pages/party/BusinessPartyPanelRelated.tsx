import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react';
import { Building2, ChevronRight, FileText, Handshake, SlidersHorizontal, Users, type LucideIcon } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import type { BusinessPartyRecord, PartyRelationshipListResponseBody, PartyRelationshipRecord } from './api';
import { businessPartyObjectHash } from './business-parties-address';
import { formatCount, type PartyTileKind } from './business-parties-workbench';
import {
  leadingRows,
  relatedRelationshipLimit,
  type PanelRelatedObjectSection,
  type PanelRelatedTarget,
} from './business-party-panel-content';
import { PanelBand, RelationshipPeriod, panelLinkClass, panelTextLinkClass } from './BusinessPartyPanelParts';
import { RefText } from './copy-affordance';
import { Instant, InstantRange, UnknownPartyName, statusBadge } from './detail-primitives';
import {
  OverviewSectionStateNote,
  partyOverviewSectionTitles,
  useCustomerAccountWrites,
  type CustomerAccountWrites,
  type PartyOverviewLists,
  type PartyOverviewStates,
} from './PartyOverviewSection';
import { countText, overviewSectionHash, type OverviewSectionState, type PartyRelationshipView } from './party-overview';
import { PartyRelationshipEndActions } from './PartyRelationshipEndActions';
import { PartyRelationshipGraph } from './PartyRelationshipGraph';
import { relationshipGraphOf, relationshipGraphWithDraft, type RelationshipGraphDraft } from './party-relationship-graph';
import { PartyRoleTag, PartyTile, partyRoleTileKind } from './party-role-visuals';
import { commercialStatusLabels, identityLayerAbsentNote, identityStatusLabels, labelOf, settlementMethodLabels } from './presentation';

// 往来单位工作台右栏的「关联」签（票 business-party-redesign/05）：通栏，不套卡片。先是合作关系——图说「谁对谁担任什么」，
// 表给每一段的对方、关系、状态与有效期，行点开是这一段的其余几格与「结束…」；再是另外四册里与这家单位有关的行。
// 段状态、读不到时的那几句、客户账户的写入口与对象页的「关联」分区共用（PartyOverviewSection.tsx），这里只换摆法。

/** 落到表里某一行时在它上面让出的高（表头一行的高）：行不顶着滚动区的上沿，看得出它上面是表头还是别的行。 */
const rowScrollMargin = 36;

function StripFact({ label, mono = false, children }: { label: string; mono?: boolean; children: ReactNode }) {
  return (
    <div className="flex min-w-0 gap-1.5">
      <dt className="shrink-0 text-idpxyz-breadcrumb">{label}</dt>
      <dd className={`min-w-0 break-words text-idpxyz-text ${mono ? 'font-mono' : ''}`}>{children}</dd>
    </div>
  );
}

/**
 * 合作关系表的一行与它点开后的那一条。行上只摆要一行一行比着看的四格；对方的编码、适用范围、到秒的有效区间、关系编号
 * 与「结束…」在点开的那一条里——结束件与合作关系视图右栏是同一个（PartyRelationshipEndActions）。
 */
function RelationshipRow({
  view,
  record,
  tileKind,
  open,
  onToggle,
  ending,
  onEndingChange,
  onEnded,
}: {
  view: PartyRelationshipView;
  record: PartyRelationshipRecord | undefined;
  tileKind: PartyTileKind;
  open: boolean;
  onToggle: () => void;
  /** 这一段的「结束」开着。 */
  ending: boolean;
  onEndingChange: (ending: boolean) => void;
  /** 有写授予的页面给；不给就只读，不摆「结束…」。 */
  onEnded?: () => void;
}) {
  const detailId = useId();
  const other = view.otherPartyNameKnown && view.otherPartyName ? view.otherPartyName : null;
  return (
    <>
      <div
        role="row"
        data-relationship-row={view.relationshipId}
        className={`business-party-relationship-row min-h-12 cursor-pointer border-b border-idpxyz-border py-1.5 hover:bg-idpxyz-hover ${open ? 'bg-idpxyz-hover' : ''}`}
        onClick={() => {
          // 拖着选了字不算点：选中对方名称去复制的人不想顺手把这一行点开。
          if (window.getSelection()?.isCollapsed === false) return;
          onToggle();
        }}
      >
        <div role="cell" className="flex min-w-0 items-center gap-2.5 pl-6 pr-3">
          <PartyTile small kind={tileKind} />
          {other ? (
            <span className="truncate text-idpxyz-textBright" title={other}>
              {other}
            </span>
          ) : (
            <span className="truncate font-mono type-body text-idpxyz-text">{view.otherPartyId}</span>
          )}
        </div>
        <div role="cell" className="min-w-0 px-3">
          <span className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
            <span className="text-idpxyz-breadcrumb">{view.side === 'HOLDER' ? '本单位对它担任' : '它对本单位担任'}</span>
            <PartyRoleTag role={view.role} />
          </span>
          <span className="business-party-relationship-period-inline mt-0.5 block type-body leading-4 tabular-nums text-idpxyz-breadcrumb">
            <RelationshipPeriod view={view} />
          </span>
        </div>
        <div role="cell" className="min-w-0 px-3">
          <StatusPill code={view.status} />
        </div>
        <div role="cell" className="business-party-relationship-period-column min-w-0 px-3 tabular-nums text-idpxyz-text">
          <RelationshipPeriod view={view} />
        </div>
        <div role="cell" className="flex justify-end pr-6">
          <button
            type="button"
            aria-expanded={open}
            aria-controls={detailId}
            aria-label={`${open ? '收起' : '展开'}这段合作关系的明细`}
            className="grid h-7 w-7 place-items-center rounded text-idpxyz-breadcrumb hover:text-idpxyz-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent"
            onClick={(event) => {
              event.stopPropagation();
              onToggle();
            }}
          >
            <ChevronRight className={`h-4 w-4 transition-transform ${open ? 'rotate-90' : ''}`} aria-hidden="true" />
          </button>
        </div>
      </div>
      {open ? (
        <div role="row" id={detailId} className="border-b border-idpxyz-border bg-idpxyz-editor">
          <div role="cell" className="px-6 py-3">
            <dl className="flex flex-wrap gap-x-6 gap-y-1.5 type-body leading-5">
              <StripFact label="对方">
                <a className={panelTextLinkClass} href={businessPartyObjectHash(view.otherPartyId)}>
                  {other ?? view.otherPartyId}
                </a>{' '}
                <RefText value={view.otherPartyId} name="编码" className="text-idpxyz-breadcrumb" />
                {view.otherPartyNameKnown ? null : (
                  <>
                    {' '}
                    <UnknownPartyName />
                  </>
                )}
              </StripFact>
              <StripFact label="适用范围" mono>
                {view.scope}
              </StripFact>
              <StripFact label="有效区间" mono>
                <Instant value={view.effectiveStartsAt} /> → {view.effectiveEndsAt ? <Instant value={view.effectiveEndsAt} /> : '长期有效'}
              </StripFact>
              <StripFact label="关系编号">
                <RefText value={view.relationshipId} name="关系编号" /> <span className="font-mono">r{view.revision}</span>
              </StripFact>
            </dl>
            {onEnded && record && view.status === 'EFFECTIVE' ? (
              ending ? (
                <div className="mt-2 border-t border-idpxyz-border pt-2">
                  <PartyRelationshipEndActions
                    key={view.relationshipId}
                    row={record}
                    onEnded={() => {
                      onEndingChange(false);
                      onEnded();
                    }}
                  />
                  <Button variant="ghost" size="sm" className="mt-1 h-6 px-1.5 type-body text-idpxyz-breadcrumb" onClick={() => onEndingChange(false)}>
                    收起
                  </Button>
                </div>
              ) : (
                <Button variant="ghost" size="sm" className="-ml-1.5 mt-1.5 h-6 px-1.5 type-body text-idpxyz-breadcrumb" onClick={() => onEndingChange(true)}>
                  结束…
                </Button>
              )
            ) : null}
          </div>
        </div>
      ) : null}
    </>
  );
}

function RowFacts({ children }: { children: ReactNode }) {
  return <dl className="mt-1 flex flex-wrap gap-x-5 gap-y-0.5 type-body leading-5">{children}</dl>;
}

/** 另外四册里的一行：第一行是编号与状态，下面是这一行的几格。 */
function ObjectRow({ lead, status, children }: { lead: ReactNode; status: ReactNode; children: ReactNode }) {
  return (
    <li className="border-b border-idpxyz-border px-6 py-2.5 last:border-b-0">
      <div className="flex items-center justify-between gap-3">
        <span className="min-w-0 break-words font-mono type-body leading-5 text-idpxyz-text">{lead}</span>
        <span className="flex shrink-0 items-center gap-2 type-body text-idpxyz-text">{status}</span>
      </div>
      <RowFacts>{children}</RowFacts>
    </li>
  );
}

/**
 * 另外四册的一段：段头是册名、与这家单位有关的条数（没答出显「—」）与去那一册的链接（带上这家单位作检索词，
 * overviewSectionHash），下面是行；没答出时是那一句说明，答出了却没有是一句「没有」。
 */
function ObjectSection<Row>({
  section,
  icon,
  title,
  state,
  what,
  emptyNote,
  retry,
  moduleId,
  partyId,
  action,
  form,
  renderRow,
  footnote,
}: {
  section: PanelRelatedObjectSection;
  icon: LucideIcon;
  title: string;
  state: OverviewSectionState<Row>;
  /** 这一段读的是什么，用在读不到时的说明里。 */
  what: string;
  emptyNote: string;
  retry: () => void;
  moduleId: string;
  partyId: string;
  /** 段头右端的写动作（「新增客户账户」）。 */
  action?: ReactNode;
  /** 段头下就地开着的表单。 */
  form?: ReactNode;
  renderRow: (row: Row) => ReactNode;
  /** 行下面的一句说明，只在答出时摆。 */
  footnote?: string;
}) {
  return (
    <section data-related-section={section} className="border-b border-idpxyz-border">
      <PanelBand icon={icon} title={title} count={countText(state)}>
        {action}
        <a className={panelLinkClass} href={overviewSectionHash(moduleId, partyId)}>
          去「{moduleInfoById[moduleId].title}」
        </a>
      </PanelBand>
      {form ? <div className="border-b border-idpxyz-border px-6 py-4">{form}</div> : null}
      {state.kind !== 'answered' ? (
        <div className="px-6 py-4">
          <OverviewSectionStateNote state={state} what={what} retry={retry} />
        </div>
      ) : state.rows.length === 0 ? (
        <p className="px-6 py-3 type-body leading-5 text-idpxyz-breadcrumb">{emptyNote}</p>
      ) : (
        <ul>{state.rows.map(renderRow)}</ul>
      )}
      {footnote && state.kind === 'answered' ? (
        <p className="border-t border-idpxyz-border px-6 py-2.5 type-body leading-5 text-idpxyz-breadcrumb">{footnote}</p>
      ) : null}
    </section>
  );
}

export function BusinessPartyPanelRelated({
  row,
  ours,
  centerKind,
  tileKindOf,
  overview,
  lists,
  relationships,
  retryRelationships,
  relationshipsComplete,
  relationshipRegistration,
  relationshipDraft = null,
  onRelationshipEnded,
  accountWrites,
  onShowRelationships,
  target,
  onTargetHandled,
}: {
  row: BusinessPartyRecord;
  /** 法人册上有在用的公司主体钉着它：关系图中心标「我方公司」。 */
  ours: boolean;
  /** 关系图中心那一枚图标块，与右栏头区、名录的行同一份。 */
  centerKind: PartyTileKind;
  /** 对方的图标块按它自己的类型取：页面认得这家单位就交出来，认不得交 null；不给或交 null 就按这段关系里的角色画。 */
  tileKindOf?: (partyId: string) => PartyTileKind | null;
  overview: PartyOverviewStates;
  lists: PartyOverviewLists;
  /** 关系册答案：「结束…」要整行记录（修订号、依据），段状态里的只是这家单位看出去的那一面。 */
  relationships: ApiResult<PartyRelationshipListResponseBody> | null;
  retryRelationships: () => void;
  /** 合作关系册不带条件那一问已是整册；不是的话到手的只是一页，段数不报，并写明。 */
  relationshipsComplete: boolean;
  /** 就地登记合作关系：有写授予的页面给；`active` 为真时段头下摆 `form`，段头的入口收起。不给就只读。 */
  relationshipRegistration?: { active: boolean; onOpen: () => void; form: ReactNode };
  /** 登记表单里正在填的那一段（双方与角色都选了才有）：关系图上先把它画出来，虚线、标「未保存」。 */
  relationshipDraft?: RelationshipGraphDraft | null;
  /** 生效中的关系就地「结束…」，答 `ENDED` 时回调（页面借它重取关系册）。不给就只读。 */
  onRelationshipEnded?: () => void;
  /** 客户账户的写入口（useCustomerAccountWrites）；不给就只读。 */
  accountWrites?: CustomerAccountWrites;
  /** 去「合作关系」视图看这家单位名下的全部关系。 */
  onShowRelationships: () => void;
  /** 从概要跳进来时要落到的那一段；落过之后经 `onTargetHandled` 交还，不留到下一次挂载。 */
  target: PanelRelatedTarget | null;
  onTargetHandled: () => void;
}) {
  const rootRef = useRef<HTMLDivElement>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [ending, setEnding] = useState<string | null>(null);
  const accountFlow = useCustomerAccountWrites(row.partyId, lists.accounts, accountWrites);

  // 落到某一处：量它到这一签滚动区顶边的距离再滚过去。不用 scrollIntoView——它会连外层能滚的容器一起带动。
  const scrollTo = useCallback((selector: string, margin = 0) => {
    const root = rootRef.current;
    const scroller = root?.closest<HTMLElement>('[data-panel-scroller]');
    const element = root?.querySelector<HTMLElement>(selector);
    if (!scroller || !element) return;
    scroller.scrollTop += element.getBoundingClientRect().top - scroller.getBoundingClientRect().top - margin;
  }, []);

  useEffect(() => {
    if (target === null) return;
    if (target.relationshipId) {
      setExpanded(target.relationshipId);
      setEnding(null);
      scrollTo(`[data-relationship-row="${CSS.escape(target.relationshipId)}"]`, rowScrollMargin);
    } else {
      scrollTo(`[data-related-section="${target.section}"]`);
    }
    onTargetHandled();
  }, [target, onTargetHandled, scrollTo]);

  const relationshipState = overview.relationships;
  const records = relationships?.kind === 'outcome' ? relationships.body.relationships : [];
  const tileOf = (view: PartyRelationshipView): PartyTileKind =>
    tileKindOf?.(view.otherPartyId) ?? (view.side === 'COUNTERPARTY' ? partyRoleTileKind(view.role) : 'NONE');
  const registration = relationshipRegistration;

  return (
    <div ref={rootRef}>
      <section data-related-section="relationships" className="border-b border-idpxyz-border">
        <PanelBand
          icon={Handshake}
          title={partyOverviewSectionTitles.relationships}
          count={relationshipState.kind === 'answered' && relationshipsComplete ? `${formatCount(relationshipState.rows.length)} 段` : undefined}
        >
          {registration && !registration.active ? (
            <button type="button" className={panelLinkClass} onClick={registration.onOpen}>
              登记合作关系
            </button>
          ) : null}
        </PanelBand>
        {registration?.active ? <div className="border-b border-idpxyz-border px-6 py-4">{registration.form}</div> : null}
        {relationshipState.kind !== 'answered' ? (
          <div className="px-6 py-4">
            <OverviewSectionStateNote state={relationshipState} what="合作关系" retry={retryRelationships} />
          </div>
        ) : (
          <>
            {relationshipsComplete ? null : (
              <p className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-idpxyz-border px-6 py-2.5 type-body leading-5 text-idpxyz-breadcrumb">
                合作关系册不止一页，这里只按已取到的那一页列。
                <button type="button" className={panelLinkClass} onClick={onShowRelationships}>
                  在「合作关系」里看全部
                </button>
              </p>
            )}
            {relationshipState.rows.length === 0 && relationshipDraft === null ? (
              relationshipsComplete ? (
                <p className="px-6 py-6 text-center type-value leading-6 text-idpxyz-breadcrumb">还没有与这家单位的合作关系。</p>
              ) : null
            ) : (
              <>
                {/* 一段关系都没有、只有正在填的那一段时，图照画，表不摆（没有行的表只剩一条表头）；图下没有表，就不再画那道分隔线。 */}
                <div
                  className={`overflow-x-auto px-6 pb-[18px] pt-4 ${relationshipState.rows.length === 0 ? '' : 'border-b border-idpxyz-border'}`}
                >
                  <PartyRelationshipGraph
                    graph={relationshipGraphWithDraft(relationshipGraphOf(relationshipState.rows), relationshipDraft)}
                    centerName={row.partyName}
                    centerId={row.partyId}
                    ours={ours}
                    centerKind={centerKind}
                    tileKindOf={tileKindOf}
                    onShowDetail={onShowRelationships}
                    onShowEnded={() => scrollTo('[data-relationship-table]')}
                  />
                </div>
                {relationshipState.rows.length === 0 ? null : (
                  <div role="table" aria-label="合作关系" data-relationship-table className="type-value leading-5">
                    <div role="rowgroup">
                      <div
                        role="row"
                        className="business-party-relationship-row h-9 border-b border-idpxyz-border bg-idpxyz-editor type-body text-idpxyz-breadcrumb"
                      >
                        <span role="columnheader" className="pl-6 pr-3">
                          对方
                        </span>
                        <span role="columnheader" className="px-3">
                          关系
                        </span>
                        <span role="columnheader" className="px-3">
                          状态
                        </span>
                        <span role="columnheader" className="business-party-relationship-period-column px-3">
                          有效期
                        </span>
                        <span role="columnheader" className="pr-6">
                          <span className="sr-only">明细</span>
                        </span>
                      </div>
                    </div>
                    <div role="rowgroup">
                      {leadingRows(relationshipState.rows, relatedRelationshipLimit).listed.map((view) => (
                        <RelationshipRow
                          key={view.relationshipId}
                          view={view}
                          record={records.find((candidate) => candidate.relationshipId === view.relationshipId)}
                          tileKind={tileOf(view)}
                          open={expanded === view.relationshipId}
                          onToggle={() => {
                            setExpanded((current) => (current === view.relationshipId ? null : view.relationshipId));
                            setEnding(null);
                          }}
                          ending={ending === view.relationshipId}
                          onEndingChange={(next) => setEnding(next ? view.relationshipId : null)}
                          onEnded={onRelationshipEnded}
                        />
                      ))}
                    </div>
                  </div>
                )}
                {relationshipState.rows.length > relatedRelationshipLimit ? (
                  <p className="flex flex-wrap items-center gap-x-3 gap-y-1 px-6 py-2.5 type-body leading-5 text-idpxyz-breadcrumb">
                    这里只列最近 {relatedRelationshipLimit} 段
                    <button type="button" className={panelLinkClass} onClick={onShowRelationships}>
                      {relationshipsComplete ? `在「合作关系」里看全部 ${formatCount(relationshipState.rows.length)} 段` : '在「合作关系」里看全部'}
                    </button>
                  </p>
                ) : null}
              </>
            )}
          </>
        )}
      </section>

      <ObjectSection
        section="accounts"
        icon={Users}
        title={partyOverviewSectionTitles.accounts}
        state={overview.accounts}
        what="客户账户"
        emptyNote="还没有关联到这家单位的客户账户。"
        retry={lists.accounts.retry}
        moduleId="party-contracts"
        partyId={row.partyId}
        action={
          accountFlow.canRegister ? (
            <button type="button" className={panelLinkClass} onClick={accountFlow.openForm}>
              新增客户账户
            </button>
          ) : undefined
        }
        form={accountFlow.form}
        renderRow={(account) => (
          <ObjectRow
            key={account.accountId}
            lead={`${account.accountId} r${account.revision}`}
            status={
              <>
                {accountFlow.canDeactivate(account) ? (
                  <Button variant="ghost" size="sm" className="h-6 px-1.5 type-body text-idpxyz-breadcrumb" onClick={() => accountFlow.startDeactivation(account)}>
                    停用…
                  </Button>
                ) : null}
                {statusBadge(identityStatusLabels, account.status)}
              </>
            }
          >
            <StripFact label="生效时间" mono>
              <Instant value={account.effectiveFrom} precision="day" />
            </StripFact>
            {account.deactivatedAt ? (
              <StripFact label="停用时间" mono>
                <Instant value={account.deactivatedAt} precision="day" />
              </StripFact>
            ) : null}
          </ObjectRow>
        )}
      />
      {accountFlow.dialogs}

      <ObjectSection
        section="agreements"
        icon={FileText}
        title={partyOverviewSectionTitles.agreements}
        state={overview.agreements}
        what="供应商协议"
        emptyNote="还没有以这家单位为供应商的协议。"
        retry={lists.agreements.retry}
        moduleId="supplier-agreements"
        partyId={row.partyId}
        footnote="只发布了版本、还没登记正文的协议看不出供应商，所以不在此列。"
        renderRow={(agreement) => (
          <ObjectRow
            key={`${agreement.objectId}@${agreement.version}`}
            lead={`${agreement.objectId}@${agreement.version}`}
            status={statusBadge(commercialStatusLabels, agreement.status)}
          >
            <StripFact label="公司主体" mono>
              {agreement.legalEntity}
            </StripFact>
            <StripFact label="采购方案" mono>
              {agreement.purchasePlan}
            </StripFact>
            <StripFact label="协议范围" mono>
              {agreement.agreementScope}
            </StripFact>
            {agreement.agreementEffectiveStartsAt ? (
              <StripFact label="协议区间" mono>
                <InstantRange from={agreement.agreementEffectiveStartsAt} to={agreement.agreementEffectiveEndsAt} />
              </StripFact>
            ) : null}
          </ObjectRow>
        )}
      />

      <ObjectSection
        section="settlement"
        icon={SlidersHorizontal}
        title="结算政策（作为客户）"
        state={overview.settlement}
        what="结算政策"
        emptyNote="还没有以这家单位为客户的结算政策。"
        retry={lists.settlement.retry}
        moduleId="commercial-policies"
        partyId={row.partyId}
        renderRow={(policy) => (
          <ObjectRow key={`${policy.objectId}@${policy.version}`} lead={`${policy.objectId}@${policy.version}`} status={labelOf(settlementMethodLabels, policy.method)}>
            <StripFact label="公司主体" mono>
              {policy.legalEntity}
            </StripFact>
            <StripFact label="合同" mono>
              {policy.contractLabel}
            </StripFact>
            <StripFact label="费用范围 · 币种" mono>
              {policy.chargeScope} · {policy.currency}
            </StripFact>
            <StripFact label="有效区间" mono>
              <InstantRange from={policy.effectiveStartsAt} to={policy.effectiveEndsAt} />
            </StripFact>
          </ObjectRow>
        )}
      />

      <ObjectSection
        section="entities"
        icon={Building2}
        title={partyOverviewSectionTitles.entities}
        state={overview.entities}
        what="公司主体"
        emptyNote="这家单位不是我们登记的公司主体。"
        retry={lists.entities.retry}
        moduleId="group-legal-entities"
        partyId={row.partyId}
        renderRow={(entity) => (
          <ObjectRow
            key={entity.legalEntityId}
            lead={
              // 点了去公司主体的对象页，不是带检索词的列表。
              <a className={panelTextLinkClass} href={`#/group-legal-entities/${encodeURIComponent(entity.legalEntityId)}`}>
                {entity.legalEntityId} r{entity.revision}
              </a>
            }
            status={statusBadge(identityStatusLabels, entity.status)}
          >
            <StripFact label="注册国家 / 地区" mono>
              {entity.identityLayerRegistered ? entity.registrationCountry : identityLayerAbsentNote}
            </StripFact>
          </ObjectRow>
        )}
      />

      <p className="px-6 py-3 type-body leading-5 text-idpxyz-breadcrumb">授信、余额与运单不在这里显示。</p>
    </div>
  );
}
