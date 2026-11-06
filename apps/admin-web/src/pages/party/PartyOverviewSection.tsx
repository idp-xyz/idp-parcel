import { useState, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button, ConfirmDialog, Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@idpxyz/ui-primitives';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import {
  commercialPolicyLoaders,
  listCommercialPolicies,
  listCustomerAccounts,
  listGroupLegalEntities,
  listSupplierAgreements,
  type BusinessPartyListResponseBody,
  type CustomerAccountRecord,
  type PartyRelationshipListResponseBody,
  type PartyRelationshipRecord,
} from './api';
import { Instant, InstantRange, UnknownPartyName, statusBadge } from './detail-primitives';
import {
  countText,
  customerAccountsOfParty,
  effectiveRolesOf,
  legalEntitiesOfParty,
  relationshipsOfParty,
  sectionOf,
  settlementPoliciesOfParty,
  supplierAgreementsOfParty,
  type OverviewSectionState,
  type PartyRelationshipView,
  overviewSectionHash,
} from './party-overview';
import {
  commercialStatusLabels,
  identityLayerAbsentNote,
  identityStatusLabels,
  labelOf,
  partyRoleLabels,
  problemNote,
  relationshipStatusLabels,
  settlementMethodLabels,
} from './presentation';
import { useRegisterList } from './register-list';
import { PartyRelationshipEndActions } from './PartyRelationshipEndActions';
import { CustomerAccountRegistrationForm } from './CustomerAccountRegistrationForm';
import { IdentityDeactivationForm } from './IdentityDeactivationForm';
import { businessPartyObjectHash } from './business-parties-address';

// 参与方详情抽屉的「全景」签（票 operator-workspace-gaps/01）。判读全在 party-overview.ts，这里只摆。
// 关系册答案由页面持有、传进来（与关系签同一份，登一段关系后两处一起更新）；另四册在打开时各取一次，一段未配置或出错只影响那一段。

// useRegisterList 要稳定引用：带参数的读口包成模块级函数，写成内联箭头每次渲染都是新键、永远读不完。
const listSettlementPolicies = commercialPolicyLoaders.SETTLEMENT_POLICY;

const note = 'type-body text-idpxyz-breadcrumb';

function SectionBody<Row>({
  state,
  what,
  emptyNote,
  retry,
  renderRow,
}: {
  state: OverviewSectionState<Row>;
  /** 这一段读的是什么，用在读不到时的说明里；读的人是配置员，不给端点。 */
  what: string;
  emptyNote: string;
  retry: () => void;
  renderRow: (row: Row) => ReactNode;
}) {
  switch (state.kind) {
    case 'loading':
      return <p className={note}>正在读取…</p>;
    case 'unconfigured':
      return (
        <p className={note}>
          暂时读不到{what}：接入渠道还没有配置。这不代表没有记录；配置好之后重新打开即可。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={state.kind} what={what} className={note} />;
    case 'callerProblem':
      return (
        <div className={note}>
          <p>
            调用方式问题：{problemNote(state.code)}
            {state.detail ? ` ${state.detail}` : ''}
          </p>
          <TechnicalDetails rows={httpRows(state.status, state.code)} />
        </div>
      );
    case 'noAnswer':
      return (
        <SectionError
          title="没有得到答案"
          description={problemNote(state.code)}
          technical={httpRows(state.status, state.code)}
          onRetry={retry}
        />
      );
    case 'transport':
      return <SectionError title="读不到数据" description={state.message} onRetry={retry} />;
    case 'mismatch':
      return (
        <p className={note}>
          读回的数据与所问的{what}对不上，已丢弃；请刷新后再试，仍不对就报障。
          <Button variant="ghost" size="sm" className="ml-2" onClick={retry}>
            重试
          </Button>
        </p>
      );
    case 'answered':
      return state.rows.length === 0 ? (
        <p className={note}>{emptyNote}</p>
      ) : (
        <ul className="flex flex-col gap-2">{state.rows.map(renderRow)}</ul>
      );
  }
}

function Section<Row>({
  title,
  moduleId,
  partyId,
  children,
  state,
  action,
}: {
  title: string;
  /** 这段事实所在的页；缺席即就在本页另一签。 */
  moduleId?: string;
  /** 带去目标页作检索词，见 overviewSectionHash。 */
  partyId?: string;
  children: ReactNode;
  state: OverviewSectionState<Row>;
  /** 标题条右端的动作（「登记合作关系」「新增客户账户」）；与「去某页」并排时动作在前（票 business-parties-object-page/03）。 */
  action?: ReactNode;
}) {
  const target = moduleId ? moduleInfoById[moduleId] : undefined;
  const href = moduleId ? overviewSectionHash(moduleId, partyId ?? '') : '';
  return (
    <section className="py-3 border-b border-idpxyz-border last:border-b-0">
      <header className="mb-2 flex items-center justify-between gap-2">
        <h3 className="type-body font-medium text-idpxyz-text">
          {title}
          <span className="ml-1.5 font-mono text-idpxyz-breadcrumb">{countText(state)}</span>
        </h3>
        <span className="flex items-center gap-1">
          {action ?? null}
          {target ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                window.location.hash = href;
              }}
            >
              去「{target.title}」
            </Button>
          ) : null}
        </span>
      </header>
      {children}
    </section>
  );
}

function RowCard({ children }: { children: ReactNode }) {
  return <li className="rounded border border-idpxyz-border px-2.5 py-2 type-body text-idpxyz-text">{children}</li>;
}

function Line({ label, children, mono = false }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <p className="mt-0.5">
      <span className="text-idpxyz-breadcrumb">{label}：</span>
      <span className={mono ? 'font-mono' : ''}>{children}</span>
    </p>
  );
}

// 还在起作用的关系：候选与已生效；到期、撤销、替代都是终局（CONTEXT「参与方关系」生命周期），收进「已结束」。
const liveRelationshipStatuses: ReadonlySet<string> = new Set(['CANDIDATE', 'EFFECTIVE']);

/**
 * 合作关系块的行（票 business-parties-object-page/02）：还在起作用的照列，生效中的可就地「结束…」；已结束的收在「已结束 N 条」下，
 * 默认折起——来对象页的人多半只关心现在的关系，历史关系留着可查。对方单位名可点到它的对象页。
 */
function RelationshipRows({
  state,
  records,
  retry,
  onEnded,
}: {
  state: OverviewSectionState<PartyRelationshipView>;
  records: readonly PartyRelationshipRecord[];
  retry: () => void;
  onEnded?: () => void;
}) {
  const [ending, setEnding] = useState<string | null>(null);
  if (state.kind !== 'answered') {
    return <SectionBody state={state} what="合作关系" emptyNote="还没有与这家单位的合作关系。" retry={retry} renderRow={() => null} />;
  }
  const live = state.rows.filter((view) => liveRelationshipStatuses.has(view.status));
  const ended = state.rows.filter((view) => !liveRelationshipStatuses.has(view.status));
  const row = (view: PartyRelationshipView, endable: boolean) => {
    const record = records.find((candidate) => candidate.relationshipId === view.relationshipId);
    return (
      <RowCard key={view.relationshipId}>
        <div className="flex items-center justify-between gap-2">
          <span>
            {view.side === 'HOLDER' ? '担任' : '对方担任'}
            {labelOf(partyRoleLabels, view.role)}
          </span>
          <span className="flex items-center gap-2">
            {endable && onEnded && record && view.status === 'EFFECTIVE' && ending !== view.relationshipId ? (
              <Button variant="ghost" size="sm" className="h-6 px-1.5 type-body text-idpxyz-breadcrumb" onClick={() => setEnding(view.relationshipId)}>
                结束…
              </Button>
            ) : null}
            {statusBadge(relationshipStatusLabels, view.status)}
          </span>
        </div>
        <Line label="对方">
          <a className="text-idpxyz-accent hover:underline" href={businessPartyObjectHash(view.otherPartyId)}>
            {view.otherPartyNameKnown ? view.otherPartyName : view.otherPartyId}
          </a>{' '}
          <span className="font-mono text-idpxyz-breadcrumb">{view.otherPartyId}</span>
          {view.otherPartyNameKnown ? null : <UnknownPartyName />}
        </Line>
        <Line label="适用范围" mono>
          {view.scope}
        </Line>
        <Line label="有效区间" mono>
          <InstantRange from={view.effectiveStartsAt} to={view.effectiveEndsAt} />
        </Line>
        <Line label="关系编号" mono>
          {view.relationshipId} r{view.revision}
        </Line>
        {ending === view.relationshipId && record && onEnded ? (
          <div className="mt-2 border-t border-idpxyz-border pt-2">
            <PartyRelationshipEndActions
              key={view.relationshipId}
              row={record}
              onEnded={() => {
                setEnding(null);
                onEnded();
              }}
            />
            <Button variant="ghost" size="sm" className="mt-1 h-6 px-1.5 type-body text-idpxyz-breadcrumb" onClick={() => setEnding(null)}>
              收起
            </Button>
          </div>
        ) : null}
      </RowCard>
    );
  };
  return (
    <>
      {live.length === 0 ? (
        <p className={note}>{ended.length === 0 ? '还没有与这家单位的合作关系。' : '没有还在起作用的合作关系。'}</p>
      ) : (
        <ul className="flex flex-col gap-2">{live.map((view) => row(view, true))}</ul>
      )}
      {ended.length > 0 ? (
        <details className="mt-2">
          <summary className={`cursor-pointer ${note}`}>已结束 {ended.length} 条</summary>
          <ul className="mt-2 flex flex-col gap-2">{ended.map((view) => row(view, false))}</ul>
        </details>
      ) : null}
    </>
  );
}

// 还能停用的账户：已生效与待生效（CONTEXT「参与方身份」：未到生效时点的登记也可以停用，那是撤下一笔还没生效的登记）。
const deactivatableAccountStatuses: ReadonlySet<string> = new Set(['REGISTERED', 'EFFECTIVE']);

/**
 * 客户账户块（票 business-parties-object-page/03）：账户行显状态；有写授予时块头「新增客户账户」就地换成表单（所属客户锁成本单位），
 * 每个还能停用的账户一行「停用…」，对话框里是停用表单锁成这个账户。有没提交的输入时「取消」先问一句。落册后重读账户册。
 */
function CustomerAccountsBlock({
  partyId,
  state,
  accounts,
  writes,
}: {
  partyId: string;
  state: OverviewSectionState<CustomerAccountRecord>;
  accounts: { answer: ApiResult<{ accounts: CustomerAccountRecord[] }> | null; retry: () => void };
  writes?: { parties: ApiResult<BusinessPartyListResponseBody> | null };
}) {
  const [registering, setRegistering] = useState(false);
  const [unsaved, setUnsaved] = useState(false);
  const [confirmingDiscard, setConfirmingDiscard] = useState(false);
  const [deactivating, setDeactivating] = useState<CustomerAccountRecord | null>(null);
  const knownAccounts = accounts.answer?.kind === 'outcome' ? accounts.answer.body.accounts : null;
  return (
    <>
      <Section
        title="客户账户"
        moduleId="party-contracts"
        partyId={partyId}
        state={state}
        action={
          writes && !registering ? (
            <Button variant="ghost" size="sm" className="type-body text-idpxyz-breadcrumb" onClick={() => setRegistering(true)}>
              新增客户账户
            </Button>
          ) : undefined
        }
      >
        {registering && writes ? (
          <div className="mb-2 rounded-md border border-idpxyz-border bg-idpxyz-bg p-3">
            <CustomerAccountRegistrationForm
              key={partyId}
              knownAccounts={knownAccounts}
              parties={writes.parties}
              preset={{ customerPartyId: partyId }}
              onRegistered={() => {
                setUnsaved(false);
                setRegistering(false);
                accounts.retry();
              }}
              onUnsavedChange={setUnsaved}
              inline={{ onCancel: () => (unsaved ? setConfirmingDiscard(true) : setRegistering(false)) }}
            />
          </div>
        ) : null}
        <SectionBody
          state={state}
          what="客户账户"
          emptyNote="还没有关联到这家单位的客户账户。"
          retry={accounts.retry}
          renderRow={(row) => (
            <RowCard key={row.accountId}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.accountId} r{row.revision}
                </span>
                <span className="flex items-center gap-2">
                  {writes && deactivatableAccountStatuses.has(row.status) ? (
                    <Button variant="ghost" size="sm" className="h-6 px-1.5 type-body text-idpxyz-breadcrumb" onClick={() => setDeactivating(row)}>
                      停用…
                    </Button>
                  ) : null}
                  {statusBadge(identityStatusLabels, row.status)}
                </span>
              </div>
              <Line label="生效时间" mono>
                <Instant value={row.effectiveFrom} precision="day" />
              </Line>
              {row.deactivatedAt ? (
                <Line label="停用时间" mono>
                  <Instant value={row.deactivatedAt} precision="day" />
                </Line>
              ) : null}
            </RowCard>
          )}
        />
      </Section>
      {writes ? (
        <Dialog open={deactivating !== null} onOpenChange={(open) => (open ? undefined : setDeactivating(null))}>
          <DialogContent size="xl" className="max-h-[85vh] overflow-y-auto">
            {deactivating ? (
              <>
                <DialogHeader className="items-start">
                  <div>
                    <DialogTitle>停用客户账户 {deactivating.accountId}</DialogTitle>
                    <DialogDescription className="mt-1">从停用时间起，这个账户不再接新的委托与合同；已有记录保留，停用后不能撤回。</DialogDescription>
                  </div>
                  <Button type="button" variant="ghost" size="icon" aria-label="关闭" onClick={() => setDeactivating(null)}>
                    <X className="h-4 w-4" aria-hidden="true" />
                  </Button>
                </DialogHeader>
                <DialogBody>
                  <IdentityDeactivationForm
                    key={deactivating.accountId}
                    parties={writes.parties}
                    locked={{ kind: 'CUSTOMER_ACCOUNT', id: deactivating.accountId }}
                    subjectLabel="客户账户"
                    onDeactivated={() => {
                      setDeactivating(null);
                      accounts.retry();
                    }}
                  />
                </DialogBody>
              </>
            ) : null}
          </DialogContent>
        </Dialog>
      ) : null}
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={confirmingDiscard}
        tone="warning"
        title="放弃修改？"
        message="有内容还没有保存，取消后填写的内容不会保留。"
        confirmLabel="放弃"
        cancelLabel="继续编辑"
        onConfirm={() => {
          setConfirmingDiscard(false);
          setUnsaved(false);
          setRegistering(false);
        }}
        onCancel={() => setConfirmingDiscard(false)}
      />
    </>
  );
}

function RoleTags({ label, roles }: { label: string; roles: string[] }) {
  if (roles.length === 0) return null;
  return (
    <p className="mt-1 flex flex-wrap items-center gap-1.5 type-body">
      <span className="text-idpxyz-breadcrumb">{label}</span>
      {roles.map((role) => (
        <span key={role} className="rounded border border-idpxyz-border px-1.5 py-0.5 text-idpxyz-text">
          {labelOf(partyRoleLabels, role)}
        </span>
      ))}
    </p>
  );
}

/**
 * 全景各段的合计（对象页「关联」签上的数，票 business-parties-object-page/01）：与块里那行「合作关系 N · 客户账户 N ……」同一份读法、
 * 同一批缓存的读口，各段数法与那一行逐字相同。任一段还没答出或答不出就是 undefined——签上不显数，
 * 不拿半截数冒充总数。
 */
/** 全景五段各自的段状态，键即段序。右栏头区的计数行（票 business-party-ux/08）与对象页「关联」签的合计都从它算。 */
export interface PartyOverviewStates {
  relationships: OverviewSectionState<PartyRelationshipView>;
  accounts: OverviewSectionState<CustomerAccountRecord>;
  entities: OverviewSectionState<unknown>;
  agreements: OverviewSectionState<unknown>;
  settlement: OverviewSectionState<unknown>;
}

/** 五段各自的标题，与块里的段名同词。 */
export const partyOverviewSectionTitles: Record<keyof PartyOverviewStates, string> = {
  relationships: '合作关系',
  accounts: '客户账户',
  entities: '公司主体',
  agreements: '供应商协议',
  settlement: '结算政策',
};

/**
 * 读四册并按参与方切出五段状态。读口经取数层缓存（register-list.ts 文件头）：头区与「关联」签两处挂着也只取一次，
 * 哪一册没答出那一段就不是 answered。
 */
export function usePartyOverviewStates(
  partyId: string,
  relationships: ApiResult<PartyRelationshipListResponseBody> | null,
): PartyOverviewStates {
  const accounts = useRegisterList(listCustomerAccounts);
  const entities = useRegisterList(listGroupLegalEntities);
  const agreements = useRegisterList(listSupplierAgreements);
  const settlement = useRegisterList(listSettlementPolicies);
  return {
    relationships: sectionOf(relationships, (body) => relationshipsOfParty(partyId, body.relationships)),
    accounts: sectionOf(accounts.answer, (body) => customerAccountsOfParty(partyId, body.accounts)),
    entities: sectionOf(entities.answer, (body) => legalEntitiesOfParty(partyId, body.entities)),
    agreements: sectionOf(agreements.answer, (body) => supplierAgreementsOfParty(partyId, body.agreements)),
    settlement: sectionOf(settlement.answer, (body) =>
      body.kind === 'SETTLEMENT_POLICY' ? settlementPoliciesOfParty(partyId, body.policies) : null,
    ),
  };
}

export function usePartyOverviewCount(
  partyId: string,
  relationships: ApiResult<PartyRelationshipListResponseBody> | null,
): number | undefined {
  const states = usePartyOverviewStates(partyId, relationships);
  let total = 0;
  for (const state of Object.values(states)) {
    if (state.kind !== 'answered') return undefined;
    total += state.rows.length;
  }
  return total;
}

export function PartyOverviewSection({
  partyId,
  relationships,
  retryRelationships,
  relationshipRegistration,
  onRelationshipEnded,
  accountWrites,
  summaryCounts,
}: {
  partyId: string;
  relationships: ApiResult<PartyRelationshipListResponseBody> | null;
  retryRelationships: () => void;
  /**
   * 「合作关系」块就地登记（票 business-party-edit-mode/03）：有写授予的页面给；`active` 为真时块体换成 `form`，标题条的
   * 「登记合作关系」收起。不给就只读。
   */
  relationshipRegistration?: { active: boolean; onOpen: () => void; form: ReactNode };
  /**
   * 生效中的关系就地「结束…」（票 business-parties-object-page/02）：有写授予的页面给，答 `ENDED` 时回调（页面借它重取关系册）。
   * 不给就只读。结束件与合作关系册右栏是同一个（PartyRelationshipEndActions），撤销 / 到期的判据一字不改。
   */
  onRelationshipEnded?: () => void;
  /**
   * 客户账户块的写入口（票 business-parties-object-page/03）：有写授予的页面给，带上页面持有的往来单位列表答案（新增表单的候选、
   * 停用表单的版本建议都从它取）。给了才摆「新增客户账户」与各账户的「停用…」；不给就只读。
   */
  accountWrites?: { parties: ApiResult<BusinessPartyListResponseBody> | null };
  /**
   * 顶部「它对我们是什么」里要不要带那行五册计数。右栏把计数摆在头区（票 business-party-ux/08），签里就不再重复——一个事实
   * 首屏只出现一处；对象页没有那一行，照旧带。缺省带。
   */
  summaryCounts?: boolean;
}) {
  const accounts = useRegisterList(listCustomerAccounts);
  const entities = useRegisterList(listGroupLegalEntities);
  const agreements = useRegisterList(listSupplierAgreements);
  const settlement = useRegisterList(listSettlementPolicies);

  const relationshipState = sectionOf(relationships, (body) => relationshipsOfParty(partyId, body.relationships));
  const accountState = sectionOf(accounts.answer, (body) => customerAccountsOfParty(partyId, body.accounts));
  const entityState = sectionOf(entities.answer, (body) => legalEntitiesOfParty(partyId, body.entities));
  const agreementState = sectionOf(agreements.answer, (body) => supplierAgreementsOfParty(partyId, body.agreements));
  const settlementState = sectionOf(settlement.answer, (body) =>
    body.kind === 'SETTLEMENT_POLICY' ? settlementPoliciesOfParty(partyId, body.policies) : null,
  );

  const roles = relationshipState.kind === 'answered' ? effectiveRolesOf(relationshipState.rows) : null;

  return (
    <div>
      <section className="py-3 border-b border-idpxyz-border">
        <h3 className="type-body font-medium text-idpxyz-text">它对我们是什么</h3>
        {roles === null ? (
          <p className={`mt-1 ${note}`}>合作关系还没读到，角色见下方「合作关系」一段。</p>
        ) : roles.asHolder.length === 0 && roles.asCounterparty.length === 0 ? (
          <p className={`mt-1 ${note}`}>还没有已生效的合作关系。</p>
        ) : (
          <>
            <RoleTags label="担任：" roles={roles.asHolder} />
            <RoleTags label="对方担任：" roles={roles.asCounterparty} />
          </>
        )}
        {summaryCounts !== false ? (
          <p className={`mt-2 ${note}`}>
            合作关系 {countText(relationshipState)} · 客户账户 {countText(accountState)} · 公司主体 {countText(entityState)} · 供应商协议{' '}
            {countText(agreementState)} · 结算政策 {countText(settlementState)}
          </p>
        ) : null}
        <p className={`mt-1 ${note}`}>
          授信、余额与运单不在这里显示；角色只数已生效的关系。
        </p>
      </section>

      <Section
        title="合作关系"
        state={relationshipState}
        action={
          relationshipRegistration && !relationshipRegistration.active ? (
            <Button variant="ghost" size="sm" className="type-body text-idpxyz-breadcrumb" onClick={relationshipRegistration.onOpen}>
              登记合作关系
            </Button>
          ) : undefined
        }
      >
        {relationshipRegistration?.active ? (
          <div className="rounded-md border border-idpxyz-border bg-idpxyz-bg p-3">{relationshipRegistration.form}</div>
        ) : (
        <>
        <RelationshipRows
          key={partyId}
          state={relationshipState}
          records={relationships?.kind === 'outcome' ? relationships.body.relationships : []}
          retry={retryRelationships}
          onEnded={onRelationshipEnded}
        />
        <p className={`mt-2 ${note}`}>全部关系在本页「合作关系」册可筛看。</p>
        </>
        )}
      </Section>

      {/* 按单位重挂：工作台右栏换行时不重挂整栏，块里开着的表单、对话框与「结束」展开不能串到下一家单位。 */}
      <CustomerAccountsBlock key={partyId} partyId={partyId} state={accountState} accounts={accounts} writes={accountWrites} />

      <Section title="公司主体" moduleId="group-legal-entities" partyId={partyId} state={entityState}>
        <SectionBody
          state={entityState}
          what="公司主体"
          emptyNote="这家单位不是我们登记的公司主体。"
          retry={entities.retry}
          renderRow={(row) => (
            <RowCard key={row.legalEntityId}>
              <div className="flex items-center justify-between gap-2">
                {/* 点了去公司主体的对象页（地址形状见 group-legal-entities-address.ts 文件头），不是带检索词的列表。 */}
                <a
                  href={`#/group-legal-entities/${encodeURIComponent(row.legalEntityId)}`}
                  className="font-mono text-idpxyz-accent hover:underline focus-visible:underline"
                >
                  {row.legalEntityId} r{row.revision}
                </a>
                {statusBadge(identityStatusLabels, row.status)}
              </div>
              <Line label="注册国家 / 地区" mono>
                {row.identityLayerRegistered ? row.registrationCountry : identityLayerAbsentNote}
              </Line>
            </RowCard>
          )}
        />
      </Section>

      <Section title="供应商协议" moduleId="supplier-agreements" partyId={partyId} state={agreementState}>
        <SectionBody
          state={agreementState}
          what="供应商协议"
          emptyNote="还没有以这家单位为供应商的协议。"
          retry={agreements.retry}
          renderRow={(row) => (
            <RowCard key={`${row.objectId}@${row.version}`}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.objectId}@{row.version}
                </span>
                {statusBadge(commercialStatusLabels, row.status)}
              </div>
              <Line label="公司主体" mono>
                {row.legalEntity}
              </Line>
              <Line label="采购方案" mono>
                {row.purchasePlan}
              </Line>
              <Line label="协议范围" mono>
                {row.agreementScope}
              </Line>
              {row.agreementEffectiveStartsAt ? (
                <Line label="协议区间" mono>
                  <InstantRange from={row.agreementEffectiveStartsAt} to={row.agreementEffectiveEndsAt} />
                </Line>
              ) : null}
            </RowCard>
          )}
        />
        {agreementState.kind === 'answered' ? (
          <p className={`mt-2 ${note}`}>只发布了版本、还没登记正文的协议看不出供应商，所以不在此列。</p>
        ) : null}
      </Section>

      <Section title="结算政策（作为客户）" moduleId="commercial-policies" partyId={partyId} state={settlementState}>
        <SectionBody
          state={settlementState}
          what="结算政策"
          emptyNote="还没有以这家单位为客户的结算政策。"
          retry={settlement.retry}
          renderRow={(row) => (
            <RowCard key={`${row.objectId}@${row.version}`}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.objectId}@{row.version}
                </span>
                <span>{labelOf(settlementMethodLabels, row.method)}</span>
              </div>
              <Line label="公司主体" mono>
                {row.legalEntity}
              </Line>
              <Line label="合同" mono>
                {row.contractLabel}
              </Line>
              <Line label="费用范围 · 币种" mono>
                {row.chargeScope} · {row.currency}
              </Line>
              <Line label="有效区间" mono>
                <InstantRange from={row.effectiveStartsAt} to={row.effectiveEndsAt} />
              </Line>
            </RowCard>
          )}
        />
      </Section>
    </div>
  );
}
