import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { MultiRegistrationPanel, chipClass, type RegistrationTarget } from '../../components/registration';
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

// —— 登记签(ADR-0085,票 admin-write-faces/02 切片 02b)——
//
// 两册的顺序与上面读签的 `registries` 一致,让同一本册在读签与写签里排在同一位。
//
// 选册按钮取读面已有的 `registryLabels`,不为写签另造说法;但键要转一道——读口的册词是
// `interpretation`,写口与 CLI 的册词是 `interpretation-rule`,两处各随自己的端点,谁也
// 不迁就谁。这一行转换就是那道差异的全部落点,写在这里而不是在词表里改名。
const registrationTargets: RegistrationTarget[] = (
  [
    ['case-requirement', 'case-requirement'],
    ['interpretation-rule', 'interpretation'],
  ] as const
).map(([kind, readRegistry]) => ({
  id: kind,
  label: registryLabels[readRegistry],
  title: registrationTitles[kind],
  endpoint: `POST ${customsRegistrationEndpoints[kind]}`,
  snapshotHint: registrationSnapshotHints[kind],
  submit: (snapshot: unknown) => registerCustomsConfiguration(kind, snapshot),
  outcomeLabels: registrationOutcomeLabels,
}));

/**
 * 合规规则库(customs-compliance):两本册子查阅一签,两本册子登记一签(ADR-0085,
 * 票 admin-write-faces/02 切片 02b)。
 *
 * **建案要求规则的登记口比另一本晚一步到。** 票 02 的关务片把十二个用例分成「配置四类」
 * 与「案件事实七类」,四加七只有十一个,漏掉的第十二个正是它;它按判据明明白白是配置——
 * 登的是「某辖区+方向+程序要不要建案」加依据,答案代数与另四类同为 `CaseConfigurationOutcome`,
 * 受控 CLI 里也有 `case-requirement` 一命令。端点补建后本签随之从一册变两册,签名也从
 * 「登记解释规则」改回「登记合规规则」——**在它到位之前签名是写死单册的**,为的是不让
 * 一个不存在的端点在页面上长出入口(那会答 404,而 404 与今天必然的 403「接入渠道未配置」
 * 长得像却是两件事:后者是诚实答案,前者是页面自己编出来的路)。
 *
 * 登记签只有登记一个动作,两册的理由却不同形:解释规则不可覆盖,更正是登一个更晚法定
 * 起点的新版、开放前版终点随之落定(ADR-0070);建案要求规则没有版本维,是键上的当前
 * 判断,同键重登异内容由登记册答冲突。两者都没有行级编辑或删除面。
 */
export function ComplianceRulesPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="rules" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="rules">合规规则库</TabsTrigger>
          <TabsTrigger value="register">登记合规规则</TabsTrigger>
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
          <MultiRegistrationPanel
            moduleId="compliance-rules"
            targets={registrationTargets}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
