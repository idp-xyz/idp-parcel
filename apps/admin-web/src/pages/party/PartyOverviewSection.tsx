import type { ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { SectionError } from '../../components/states';
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

const note = 'text-[12px] text-idpxyz-textMuted';

function SectionBody<Row>({
  state,
  endpoint,
  emptyNote,
  retry,
  renderRow,
}: {
  state: OverviewSectionState<Row>;
  endpoint: string;
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
          访问通道尚未配置：{endpoint} 当前不可用（403）。这不是「没有」——今天没有问到；配置该上下文的访问通道后重新打开抽屉。
        </p>
      );
    case 'callerProblem':
      return (
        <p className={note}>
          调用方式问题（HTTP {state.status}）：{problemNote(state.code)}
          {state.detail ? ` ${state.detail}` : ''}
        </p>
      );
    case 'noAnswer':
      return <SectionError title={`服务端未形成答案（HTTP ${state.status}）`} description={problemNote(state.code)} onRetry={retry} />;
    case 'transport':
      return <SectionError title="无法连接主数据读取服务" description={state.message} onRetry={retry} />;
    case 'mismatch':
      return (
        <p className={note}>
          答案回显的册种与所问（{endpoint}）不符，已丢弃；请检查前端与服务端版本是否一致。
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
  children,
  state,
}: {
  title: string;
  /** 这段事实所在的页；缺席即就在本页另一签。 */
  moduleId?: string;
  children: ReactNode;
  state: OverviewSectionState<Row>;
}) {
  const target = moduleId ? moduleInfoById[moduleId] : undefined;
  return (
    <section className="py-3 border-b border-idpxyz-border last:border-b-0">
      <header className="mb-2 flex items-center justify-between gap-2">
        <h3 className="text-[12px] font-medium text-idpxyz-text">
          {title}
          <span className="ml-1.5 font-mono text-idpxyz-textMuted">{countText(state)}</span>
        </h3>
        {target ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              window.location.hash = `#/${moduleId}`;
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
      <span className="text-idpxyz-textMuted">{label}：</span>
      <span className={mono ? 'font-mono' : ''}>{children}</span>
    </p>
  );
}

function RoleTags({ label, roles }: { label: string; roles: string[] }) {
  if (roles.length === 0) return null;
  return (
    <p className="mt-1 flex flex-wrap items-center gap-1.5 text-[12px]">
      <span className="text-idpxyz-textMuted">{label}</span>
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
          <p className={`mt-1 ${note}`}>关系册尚未答复，角色摘要见下方「参与方关系」一段。</p>
        ) : roles.asHolder.length === 0 && roles.asCounterparty.length === 0 ? (
          <p className={`mt-1 ${note}`}>关系册上没有本参与方状态为「已生效」的关系。</p>
        ) : (
          <>
            <RoleTags label="作为持有方：" roles={roles.asHolder} />
            <RoleTags label="作为相对方：" roles={roles.asCounterparty} />
          </>
        )}
        <p className={`mt-2 ${note}`}>
          关系 {countText(relationshipState)} · 客户账户 {countText(accountState)} · 责任法人 {countText(entityState)} · 供应商协议{' '}
          {countText(agreementState)} · 结算政策 {countText(settlementState)}
        </p>
        <p className={`mt-1 ${note}`}>
          只列参与方与商业自己的册；授信、余额与委托不在此处，归各自上下文。角色摘要只看登记为「已生效」的关系。
        </p>
      </section>

      <Section title="参与方关系" state={relationshipState}>
        <SectionBody
          state={relationshipState}
          endpoint="GET /commercial-party-relationships"
          emptyNote="关系册上没有以本参与方为持有方或相对方的关系。"
          retry={retryRelationships}
          renderRow={(view) => (
            <RowCard key={view.relationshipId}>
              <div className="flex items-center justify-between gap-2">
                <span>
                  {view.side === 'HOLDER' ? '本方为持有方' : '本方为相对方'} · {labelOf(partyRoleLabels, view.role)}
                </span>
                {statusBadge(relationshipStatusLabels, view.status)}
              </div>
              <Line label={view.side === 'HOLDER' ? '相对方' : '持有方'}>
                <span className="font-mono">{view.otherPartyId}</span>{' '}
                {view.otherPartyNameKnown ? view.otherPartyName : <UnknownPartyName />}
              </Line>
              <Line label="适用范围" mono>
                {view.scope}
              </Line>
              <Line label="有效区间" mono>
                <InstantRange from={view.effectiveStartsAt} to={view.effectiveEndsAt} />
              </Line>
              <Line label="关系标识" mono>
                {view.relationshipId} r{view.revision}
              </Line>
            </RowCard>
          )}
        />
        <p className={`mt-2 ${note}`}>全部关系在本页「参与方关系」签可筛看。</p>
      </Section>

      <Section title="货主客户账户" moduleId="party-contracts" state={accountState}>
        <SectionBody
          state={accountState}
          endpoint="GET /commercial-customer-accounts"
          emptyNote="客户账户册上没有关联到本参与方的货主客户账户。"
          retry={accounts.retry}
          renderRow={(row) => (
            <RowCard key={row.accountId}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.accountId} r{row.revision}
                </span>
                {statusBadge(identityStatusLabels, row.status)}
              </div>
              <Line label="生效时点" mono>
                <Instant value={row.effectiveFrom} />
              </Line>
            </RowCard>
          )}
        />
      </Section>

      <Section title="责任法人" moduleId="group-legal-entities" state={entityState}>
        <SectionBody
          state={entityState}
          endpoint="GET /commercial-group-legal-entities"
          emptyNote="本参与方不是本集团登记的责任法人（法人册上没有钉着它的法人）。"
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

      <Section title="供应商协议" moduleId="supplier-agreements" state={agreementState}>
        <SectionBody
          state={agreementState}
          endpoint="GET /commercial-supplier-agreements"
          emptyNote="供应商协议册上没有供应商是本参与方的已登记正文。"
          retry={agreements.retry}
          renderRow={(row) => (
            <RowCard key={`${row.objectId}@${row.version}`}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.objectId}@{row.version}
                </span>
                <span>{labelOf(commercialStatusLabels, row.status)}</span>
              </div>
              <Line label="责任法人" mono>
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
          <p className={`mt-2 ${note}`}>正文未登记的协议版本壳没有「供应商」一格，归不进任何参与方，不在此列。</p>
        ) : null}
      </Section>

      <Section title="结算政策（本方为客户相对方）" moduleId="commercial-policies" state={settlementState}>
        <SectionBody
          state={settlementState}
          endpoint="GET /commercial-policies?kind=SETTLEMENT_POLICY"
          emptyNote="结算政策册上没有以本参与方为客户相对方的政策。"
          retry={settlement.retry}
          renderRow={(row) => (
            <RowCard key={`${row.objectId}@${row.version}`}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.objectId}@{row.version}
                </span>
                <span>{labelOf(settlementMethodLabels, row.method)}</span>
              </div>
              <Line label="责任法人" mono>
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
