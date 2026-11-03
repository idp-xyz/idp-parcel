import type { ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { moduleInfoById } from '../../navigation';
import { listCommercialPolicies, listSupplierAgreements, type SettlementPolicyRecord, type SupplierAgreementRecord } from './api';
import { InstantRange, statusBadge } from './detail-primitives';
import {
  referenceLineText,
  referenceRegisterHash,
  referenceRowHash,
  settlementPoliciesOfLegalEntity,
  supplierAgreementsOfLegalEntity,
  type ReferenceRowKind,
} from './legal-entity-references';
import { sectionOf, type OverviewSectionState } from './party-overview';
import { commercialStatusLabels, labelOf, problemNote, settlementMethodLabels } from './presentation';
import { useRegisterList } from './register-list';

// 公司主体的「关联」签（票 company-subject-ux/03）：这家公司名下的供应商协议与结算政策。判读全在 legal-entity-references.ts，
// 这里只摆。两册在打开对象页 / 详情栏时各取一次，一册未配置或出错只影响那一段；首屏「被引用」一格用的是同一份答案
// （useLegalEntityReferences 由页面调一次、两处共用），不为指标另读。

// useRegisterList 要稳定引用：带参数的读口包成模块级函数，写成内联箭头会每次渲染重取。
const listSettlementPolicies = () => listCommercialPolicies('SETTLEMENT_POLICY');

const note = 'text-[12px] text-idpxyz-breadcrumb';

export interface LegalEntityReferences {
  agreements: OverviewSectionState<SupplierAgreementRecord>;
  policies: OverviewSectionState<SettlementPolicyRecord>;
  retryAgreements: () => void;
  retryPolicies: () => void;
}

/** 两册各读一次，按法人标识在已取回的行上关联；换一家公司不重读，只换关联。 */
export function useLegalEntityReferences(legalEntityId: string): LegalEntityReferences {
  const agreements = useRegisterList(listSupplierAgreements);
  const settlement = useRegisterList(listSettlementPolicies);
  return {
    agreements: sectionOf(agreements.answer, (body) => supplierAgreementsOfLegalEntity(legalEntityId, body.agreements)),
    policies: sectionOf(settlement.answer, (body) =>
      body.kind === 'SETTLEMENT_POLICY' ? settlementPoliciesOfLegalEntity(legalEntityId, body.policies) : null,
    ),
    retryAgreements: agreements.retry,
    retryPolicies: settlement.retry,
  };
}

function ReferenceBody<Row>({
  state,
  what,
  emptyNote,
  retry,
  renderRow,
}: {
  state: OverviewSectionState<Row>;
  /** 这一段读的是什么，用在读不到时的说明里；读的人是运营，不给端点。 */
  what: string;
  emptyNote: string;
  retry: () => void;
  renderRow: (row: Row) => ReactNode;
}) {
  switch (state.kind) {
    case 'loading':
      return <p className={note}>正在读取…</p>;
    case 'unconfigured':
      return <p className={note}>暂时读不到{what}：接入渠道还没有配置。这不代表没有记录；配置好之后重新打开即可。</p>;
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
        <ul className="flex flex-col gap-1.5">{state.rows.map(renderRow)}</ul>
      );
  }
}

/**
 * 一册一段：段首一行「供应商协议 4 · 在用 2」可折叠，右侧去那一册的页并带这家公司的标识作检索词。默认展开——
 * 来看「关联」签的人就是来看这几行的。
 */
function Register<Row>({
  title,
  kind,
  moduleId,
  legalEntityId,
  state,
  withInUse,
  children,
}: {
  title: string;
  kind: ReferenceRowKind;
  moduleId: string;
  legalEntityId: string;
  state: OverviewSectionState<Row & { status?: string }>;
  withInUse: boolean;
  children: ReactNode;
}) {
  const target = moduleInfoById[moduleId];
  return (
    <details open className="border-b border-idpxyz-border py-3 last:border-b-0">
      <summary className="flex cursor-pointer select-none items-center justify-between gap-2 text-[12px] font-medium text-idpxyz-text">
        <span>{referenceLineText(title, state, withInUse)}</span>
        {target ? (
          <a
            className="font-normal text-idpxyz-accent hover:underline"
            href={referenceRegisterHash(kind, legalEntityId)}
            onClick={(event) => event.stopPropagation()}
          >
            去「{target.title}」
          </a>
        ) : null}
      </summary>
      <div className="mt-2">{children}</div>
    </details>
  );
}

/** 一行一个版本，整行是链接：去对象所在的页并带它的标识作检索词（机制同参与方全景）。 */
function RowLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <li>
      <a
        href={href}
        className="block rounded border border-idpxyz-border px-2.5 py-2 text-[12px] text-idpxyz-text hover:border-idpxyz-accent hover:bg-idpxyz-hover"
      >
        {children}
      </a>
    </li>
  );
}

function Line({ label, children, mono = false }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <p className="mt-0.5">
      <span className="text-idpxyz-breadcrumb">{label}：</span>
      <span className={mono ? 'font-mono' : ''}>{children}</span>
    </p>
  );
}

export function LegalEntityReferencesSection({
  legalEntityId,
  references,
}: {
  legalEntityId: string;
  references: LegalEntityReferences;
}) {
  return (
    <div>
      <Register
        title="供应商协议"
        kind="supplier-agreement"
        moduleId="supplier-agreements"
        legalEntityId={legalEntityId}
        state={references.agreements}
        withInUse
      >
        <ReferenceBody
          state={references.agreements}
          what="供应商协议"
          emptyNote="还没有以这家公司为我方主体的供应商协议。"
          retry={references.retryAgreements}
          renderRow={(row) => (
            <RowLink key={`${row.objectId}@${row.version}`} href={referenceRowHash('supplier-agreement', row.objectId)}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.objectId}@{row.version}
                </span>
                {statusBadge(commercialStatusLabels, row.status)}
              </div>
              <Line label="供应商" mono>
                {row.supplier}
              </Line>
              <Line label="有效区间" mono>
                <InstantRange from={row.effectiveStartsAt} to={row.effectiveEndsAt} />
              </Line>
            </RowLink>
          )}
        />
        {references.agreements.kind === 'answered' ? (
          <p className={`mt-2 ${note}`}>只发布了版本、还没登记正文的协议看不出我方主体，所以不在此列。</p>
        ) : null}
      </Register>

      <Register
        title="结算政策"
        kind="settlement-policy"
        moduleId="commercial-policies"
        legalEntityId={legalEntityId}
        state={references.policies}
        withInUse={false}
      >
        <ReferenceBody
          state={references.policies}
          what="结算政策"
          emptyNote="还没有以这家公司为我方主体的结算政策。"
          retry={references.retryPolicies}
          renderRow={(row) => (
            <RowLink key={`${row.objectId}@${row.version}`} href={referenceRowHash('settlement-policy', row.objectId)}>
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono">
                  {row.objectId}@{row.version}
                </span>
                <span>{labelOf(settlementMethodLabels, row.method)}</span>
              </div>
              <Line label="客户" mono>
                {row.counterparty}
              </Line>
              <Line label="合同" mono>
                {row.contractLabel}
              </Line>
              <Line label="有效区间" mono>
                <InstantRange from={row.effectiveStartsAt} to={row.effectiveEndsAt} />
              </Line>
            </RowLink>
          )}
        />
      </Register>

      <p className={`mt-3 ${note}`}>
        客户合同与客户账户的记录上没有「我方主体」一格，这里列不出；合同经结算政策的「合同」一格可见。授信、余额与运单不在这里显示。
      </p>
    </div>
  );
}
