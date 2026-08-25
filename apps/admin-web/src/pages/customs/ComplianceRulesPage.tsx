import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatRange } from '../catalogue-view';
import {
  listComplianceRules,
  type ComplianceRegistry,
  type ComplianceRulesListResponseBody,
} from './api';
import { directionLabels, labelOf, registryLabels, resultLayerLabels } from './presentation';

const info = moduleInfoById['compliance-rules'];

interface RuleRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(id: string, header: string, mono = false): ListColumn<RuleRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

// 两本册子两套列(MCP-3 裁决⑤):案件要求规则答「是否要求建案」,解释规则答
// 「外部结果如何按层解释」,列向各随其登记册行形,不折成一套。
const registries: ReadonlyArray<{
  id: ComplianceRegistry;
  columns: ListColumn<RuleRow>[];
}> = [
  {
    id: 'case-requirement',
    columns: [
      col('jurisdiction', '适用辖区', true),
      col('direction', '申报方向'),
      col('procedure', '关务程序', true),
      col('required', '是否要求案件'),
      col('basis', '依据引用', true),
    ],
  },
  {
    id: 'interpretation',
    columns: [
      col('layer', '外部结果层'),
      col('jurisdiction', '适用辖区', true),
      col('rule', '解释规则引用', true),
      col('applies', '法定适用区间', true),
    ],
  },
];

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

function rowsOf(body: ComplianceRulesListResponseBody): RuleRow[] {
  switch (body.outcome) {
    case 'CASE_REQUIREMENT_RULES_LISTED':
      return body.rules.map((record) => ({
        key: `case:${record.jurisdiction}:${record.direction}:${record.procedure}`,
        values: {
          jurisdiction: record.jurisdiction,
          direction: labelOf(directionLabels, record.direction),
          procedure: record.procedure,
          required: record.required ? '要求' : '不要求',
          basis: record.basis,
        },
      }));
    case 'INTERPRETATION_RULES_LISTED':
      return body.rules.map((record) => ({
        key: `interpretation:${record.layer}:${record.jurisdiction}:${record.rule}:${record.appliesFrom}`,
        values: {
          layer: labelOf(resultLayerLabels, record.layer),
          jurisdiction: record.jurisdiction,
          rule: record.rule,
          applies: formatRange(record.appliesFrom, record.appliesUntil),
        },
      }));
  }
}

// 两本登记册分别呈现,避免把「是否建案」与「如何解释外部结果」折成一套规则。
export function ComplianceRulesPage() {
  const [registry, setRegistry] = useState<ComplianceRegistry>('case-requirement');
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    registry: ComplianceRegistry;
    answer: ApiResult<ComplianceRulesListResponseBody>;
  } | null>(null);
  const selected = registries.find((candidate) => candidate.id === registry) ?? registries[0];

  useEffect(() => {
    let cancelled = false;
    void listComplianceRules(registry).then((answer) => {
      if (!cancelled) setLoaded({ registry, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [registry, reloadKey]);

  const answer = loaded?.registry === registry ? loaded.answer : null;
  const rows = answer?.kind === 'outcome' ? rowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<RuleRow>
      title={info.title}
      description={`${info.owner}——只读展示登记规则,新规则不默认追溯既有判断`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索辖区、程序或规则引用',
      }}
      filters={
        <>
          {registries.map((candidate) => (
            <button
              key={candidate.id}
              type="button"
              className={chipClass(candidate.id === registry)}
              onClick={() => setRegistry(candidate.id)}
            >
              {registryLabels[candidate.id]}
            </button>
          ))}
        </>
      }
      filterSummary={`${registryLabels[registry]} ${rows.length} 条`}
      columns={selected.columns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /customs-compliance-rules?registry=${registry}`,
        emptyTitle: `当前租户尚无${registryLabels[registry]}`,
        emptyDescription: '读取入口已配置,但该登记册为空;页面不会预置关务规则。',
      })}
    />
  );
}
