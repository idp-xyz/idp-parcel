import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatRange } from '../catalogue-view';
import {
  listCommercialPolicies,
  type CommercialPoliciesResponseBody,
  type CommercialPolicyKind,
} from './api';
import {
  bindingConversionLabel,
  checkGroupLabel,
  commercialDirectionLabel,
  commercialPolicyKindLabel,
  commercialStatusLabel,
  controlRequirementLabel,
  judgmentTypeLabel,
  settlementMethodLabel,
} from './presentation';

const info = moduleInfoById['commercial-policies'];

interface PolicyRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(id: string, header: string, mono = false): ListColumn<PolicyRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

const commonColumns = [
  col('identity', '政策对象 / 版本', true),
  col('scopeReference', '适用范围', true),
  col('effective', '有效区间', true),
  col('status', '生命周期状态'),
];

const kinds: ReadonlyArray<{
  id: CommercialPolicyKind;
  columns: ListColumn<PolicyRow>[];
}> = [
  {
    id: 'acceptance-control',
    columns: [
      ...commonColumns,
      col('checkGroupType', '校验组'),
      col('receivablesAccountReference', '应收账户引用', true),
    ],
  },
  {
    id: 'price',
    columns: [
      ...commonColumns,
      col('direction', '政策方向'),
      col('pricingPlanReference', '定价方案引用', true),
      col('planDirection', '方案方向'),
      col('bindingConversion', '绑定转换'),
    ],
  },
  {
    id: 'settlement',
    columns: [
      ...commonColumns,
      col('method', '结算方式'),
      col('legalEntityReference', '责任法人', true),
      col('counterpartyReference', '相对方', true),
      col('contractVersion', '合同版本', true),
      col('chargeScopeReference', '费用范围', true),
      col('currency', '币种', true),
    ],
  },
  {
    id: 'contract-control',
    columns: [
      ...commonColumns,
      col('requirement', '接受前财务控制'),
      col('notApplicableBasis', '不适用依据', true),
    ],
  },
  {
    id: 'as-of',
    columns: [
      ...commonColumns,
      col('judgmentType', '判断类型'),
      col('semanticsReference', '时点语义引用', true),
      col('policyVersion', '时点政策版本', true),
    ],
  },
];

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

function baseValues(record: {
  objectId: string;
  version: string;
  scopeReference: string;
  effectiveFrom: string;
  effectiveTo?: string;
  status: string;
}): Record<string, string> {
  return {
    identity: `${record.objectId}@${record.version}`,
    scopeReference: record.scopeReference,
    effective: formatRange(record.effectiveFrom, record.effectiveTo),
    status: commercialStatusLabel(record.status),
  };
}

function rowsOf(body: CommercialPoliciesResponseBody): PolicyRow[] {
  switch (body.outcome) {
    case 'ACCEPTANCE_CONTROL_POLICIES_LISTED':
      return body.policies.map((record) => ({
        key: `acceptance:${record.objectId}@${record.version}:${record.checkGroupType}`,
        values: {
          ...baseValues(record),
          checkGroupType: checkGroupLabel(record.checkGroupType),
          receivablesAccountReference: record.receivablesAccountReference,
        },
      }));
    case 'PRICE_POLICIES_LISTED':
      return body.policies.map((record) => ({
        key: `price:${record.objectId}@${record.version}`,
        values: {
          ...baseValues(record),
          direction: commercialDirectionLabel(record.direction),
          pricingPlanReference: record.pricingPlanReference,
          planDirection: commercialDirectionLabel(record.planDirection),
          bindingConversion: bindingConversionLabel(record.bindingConversion),
        },
      }));
    case 'SETTLEMENT_POLICIES_LISTED':
      return body.policies.map((record) => ({
        key: `settlement:${record.objectId}@${record.version}`,
        values: {
          ...baseValues(record),
          method: settlementMethodLabel(record.method),
          legalEntityReference: record.legalEntityReference,
          counterpartyReference: record.counterpartyReference,
          contractVersion: record.contractVersion,
          chargeScopeReference: record.chargeScopeReference,
          currency: record.currency,
        },
      }));
    case 'CONTRACT_CONTROL_DECLARATIONS_LISTED':
      return body.policies.map((record) => ({
        key: `contract-control:${record.objectId}@${record.version}`,
        values: {
          ...baseValues(record),
          requirement: controlRequirementLabel(record.requirement),
          notApplicableBasis: record.notApplicableBasis ?? '—',
        },
      }));
    case 'AS_OF_POLICIES_LISTED':
      return body.policies.map((record) => ({
        key: `as-of:${record.objectId}@${record.version}:${record.judgmentType}`,
        values: {
          ...baseValues(record),
          judgmentType: judgmentTypeLabel(record.judgmentType),
          semanticsReference: record.semanticsReference,
          policyVersion: record.policyVersion,
        },
      }));
  }
}

// 各政策族独立请求、独立列形；同页切换不把五类对象折成一份“大配置”。
export function CommercialPoliciesPage() {
  const [kind, setKind] = useState<CommercialPolicyKind>('acceptance-control');
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    kind: CommercialPolicyKind;
    answer: ApiResult<CommercialPoliciesResponseBody>;
  } | null>(null);
  const selected = kinds.find((candidate) => candidate.id === kind) ?? kinds[0];

  useEffect(() => {
    let cancelled = false;
    void listCommercialPolicies(kind).then((answer) => {
      if (!cancelled) setLoaded({ kind, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [kind, reloadKey]);

  const answer = loaded?.kind === kind ? loaded.answer : null;
  const rows = answer?.kind === 'outcome' ? rowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<PolicyRow>
      title={info.title}
      description={`${info.owner}——五类政策分别查阅，重叠候选仍是适用冲突而非“同时生效”`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索政策对象、范围或引用',
      }}
      filters={
        <>
          {kinds.map((candidate) => (
            <button
              key={candidate.id}
              type="button"
              className={chipClass(candidate.id === kind)}
              onClick={() => setKind(candidate.id)}
            >
              {commercialPolicyKindLabel(candidate.id)}
            </button>
          ))}
        </>
      }
      filterSummary={`${commercialPolicyKindLabel(kind)} ${rows.length} 条`}
      columns={selected.columns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /commercial-policies?kind=${kind}`,
        emptyTitle: `当前租户尚无${commercialPolicyKindLabel(kind)}`,
        emptyDescription: '读取入口已配置，但该政策登记为空；页面不会生成默认政策。',
      })}
    />
  );
}
