import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { RegistrationPanel } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatRange } from '../catalogue-view';
import {
  customsRegistrationEndpoints,
  listComplianceRules,
  registerCustomsConfiguration,
  registrationOutcomeLabels,
  type ComplianceRegistry,
  type ComplianceRulesListResponseBody,
} from './api';
import {
  directionLabels,
  labelOf,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
  registryLabels,
  resultLayerLabels,
} from './presentation';

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
function ComplianceRulesTable() {
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
      filterSummary={
        // 计数只在拿到业务答案后显示,未配置态不报「0 条」(与 pricing 两页同一守卫)。
        answer?.kind === 'outcome'
          ? `${registryLabels[registry]} ${rows.length} 条`
          : undefined
      }
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

/**
 * 合规规则库(customs-compliance):两本册子查阅一签,解释规则登记一签(ADR-0085,
 * 票 admin-write-faces/02 切片 02b)。
 *
 * **登记签只装解释规则,不装案件要求规则。** 两本册在读签上并列,写签却只有一本,不是
 * 漏了一半:案件要求规则按票 02 的判据同属租户配置、受控 CLI 里也有 case-requirement
 * 一命令,但服务端至今没有它的在线登记端点(端点表里关务只有解释规则、门禁目录、候选
 * 口岸、申报路径四个登记口)。页面不为一个不存在的端点造入口——造了就会答 404,而 404
 * 与本签今天必然的 403「接入渠道未配置」长得像却是两件事:后者是诚实答案,前者是页面
 * 自己编出来的路。签名写死「登记解释规则」而不是「登记合规规则」,为的就是让这一半的
 * 缺席在签上看得见。
 *
 * 登记签只有登记一个动作:解释规则不可覆盖,更正是登一个更晚法定起点的新版、开放前版
 * 终点随之落定(ADR-0070),历史区间不接受追改,所以没有行级编辑或删除面。
 */
export function ComplianceRulesPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="rules" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="rules">合规规则库</TabsTrigger>
          <TabsTrigger value="register">登记解释规则</TabsTrigger>
        </TabsList>
        <TabsContent
          value="rules"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <ComplianceRulesTable />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <RegistrationPanel
            moduleId="compliance-rules"
            title={registrationTitles['interpretation-rule']}
            endpoint={`POST ${customsRegistrationEndpoints['interpretation-rule']}`}
            snapshotHint={registrationSnapshotHints['interpretation-rule']}
            submit={(snapshot) => registerCustomsConfiguration('interpretation-rule', snapshot)}
            outcomeLabels={registrationOutcomeLabels}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
