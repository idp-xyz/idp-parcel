import type { ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import {
  listCommercialPolicies,
  listCustomerAccounts,
  listGroupLegalEntities,
  listSupplierAgreements,
  type PartyRelationshipListResponseBody,
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

// 参与方详情抽屉的「全景」签（票 operator-workspace-gaps/01）。判读全在 party-overview.ts，这里只摆。
// 关系册答案由页面持有、传进来（与关系签同一份，登一段关系后两处一起更新）；另四册在打开时各取一次，一段未配置或出错只影响那一段。

// useRegisterList 要稳定引用：带参数的读口包成模块级函数，写成内联箭头会每次渲染重取。
const listSettlementPolicies = () => listCommercialPolicies('SETTLEMENT_POLICY');

const note = 'text-[12px] text-idpxyz-breadcrumb';

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
}: {
  title: string;
  /** 这段事实所在的页；缺席即就在本页另一签。 */
  moduleId?: string;
  /** 带去目标页作检索词，见 overviewSectionHash。 */
  partyId?: string;
  children: ReactNode;
  state: OverviewSectionState<Row>;
}) {
  const target = moduleId ? moduleInfoById[moduleId] : undefined;
  const href = moduleId ? overviewSectionHash(moduleId, partyId ?? '') : '';
  return (
    <section className="py-3 border-b border-idpxyz-border last:border-b-0">
      <header className="mb-2 flex items-center justify-between gap-2">
        <h3 className="text-[12px] font-medium text-idpxyz-text">
          {title}
          <span className="ml-1.5 font-mono text-idpxyz-breadcrumb">{countText(state)}</span>
        </h3>
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
      </header>
      {children}
    </section>
  );
}

function RowCard({ children }: { children: ReactNode }) {
  return <li className="rounded border border-idpxyz-border px-2.5 py-2 text-[12px] text-idpxyz-text">{children}</li>;
}

function Line({ label, children, mono = false }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <p className="mt-0.5">
      <span className="text-idpxyz-breadcrumb">{label}：</span>
      <span className={mono ? 'font-mono' : ''}>{children}</span>
    </p>
  );
}

function RoleTags({ label, roles }: { label: string; roles: string[] }) {
  if (roles.length === 0) return null;
  return (
    <p className="mt-1 flex flex-wrap items-center gap-1.5 text-[12px]">
      <span className="text-idpxyz-breadcrumb">{label}</span>
      {roles.map((role) => (
        <span key={role} className="rounded border border-idpxyz-border px-1.5 py-0.5 text-idpxyz-text">
          {labelOf(partyRoleLabels, role)}
        </span>
      ))}
    </p>
  );
}

export function PartyOverviewSection({
  partyId,
  relationships,
  retryRelationships,
}: {
  partyId: string;
  relationships: ApiResult<PartyRelationshipListResponseBody> | null;
  retryRelationships: () => void;
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
        <h3 className="text-[12px] font-medium text-idpxyz-text">它对我们是什么</h3>
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
        <p className={`mt-2 ${note}`}>
          合作关系 {countText(relationshipState)} · 客户账户 {countText(accountState)} · 公司主体 {countText(entityState)} · 供应商协议{' '}
          {countText(agreementState)} · 结算政策 {countText(settlementState)}
        </p>
        <p className={`mt-1 ${note}`}>
          授信、余额与运单不在这里显示；角色只数已生效的关系。
        </p>
      </section>

      <Section title="合作关系" state={relationshipState}>
        <SectionBody
          state={relationshipState}
          what="合作关系"
          emptyNote="还没有与这家单位的合作关系。"
          retry={retryRelationships}
          renderRow={(view) => (
            <RowCard key={view.relationshipId}>
              <div className="flex items-center justify-between gap-2">
                <span>
                  {view.side === 'HOLDER' ? '担任' : '对方担任'}{labelOf(partyRoleLabels, view.role)}
                </span>
                {statusBadge(relationshipStatusLabels, view.status)}
              </div>
              <Line label="对方">
                <span className="font-mono">{view.otherPartyId}</span>{' '}
                {view.otherPartyNameKnown ? view.otherPartyName : <UnknownPartyName />}
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
            </RowCard>
          )}
        />
        <p className={`mt-2 ${note}`}>全部关系在本页「合作关系」册可筛看。</p>
      </Section>

      <Section title="客户账户" moduleId="party-contracts" partyId={partyId} state={accountState}>
        <SectionBody
          state={accountState}
          what="客户账户"
          emptyNote="还没有关联到这家单位的客户账户。"
          retry={accounts.retry}
          renderRow={(row) => (
            <RowCard key={row.accountId}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.accountId} r{row.revision}
                </span>
                {statusBadge(identityStatusLabels, row.status)}
              </div>
              <Line label="生效时间" mono>
                <Instant value={row.effectiveFrom} />
              </Line>
            </RowCard>
          )}
        />
      </Section>

      <Section title="公司主体" moduleId="group-legal-entities" partyId={partyId} state={entityState}>
        <SectionBody
          state={entityState}
          what="公司主体"
          emptyNote="这家单位不是我们登记的公司主体。"
          retry={entities.retry}
          renderRow={(row) => (
            <RowCard key={row.legalEntityId}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.legalEntityId} r{row.revision}
                </span>
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
