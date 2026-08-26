import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { catalogueViewState, formatRange } from '../catalogue-view';
import {
  listVisibilityCatalogues,
  type ApiResult,
  type VisibilityCatalogueListResponseBody,
} from './catalogue-api';
import {
  labelOf,
  sourceContextLabels,
  triageOutcomeLabels,
  visibilityCatalogueKindLabels,
} from './presentation';

// VE 六类目录拆三页,本页装**判断规则**那组:里程碑映射与分诊规则(票
// admin-web-page-wiring-frontier/02 的页面裁决)。两册同为「事实/信号 → 结论」的
// 版本化判断规则——映射把源事实归到标准里程碑,分诊把信号归到处置结论;通知与
// 披露两册是对外口径、资格与授权两册是索赔前置,各归各页,不折成六页签巨面。
//
// 行随条目展开(一行一条规则,版本列随行重复):册子是拿来查「这类事实/信号会
// 判成什么」的,按版本折行会把待查的规则埋进折叠单元格。空版本不存在——登记入口
// 要求每版至少一条(application checkEntries),行展开不会藏掉任何版本。

const info = moduleInfoById['tracking-judgment-rules'];

type JudgmentKind = 'MILESTONE_MAPPING' | 'TRIAGE_RULE';

const judgmentKinds: JudgmentKind[] = ['MILESTONE_MAPPING', 'TRIAGE_RULE'];

type JudgmentListBody = Extract<
  VisibilityCatalogueListResponseBody,
  { kind: JudgmentKind }
>;

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

// 两册行形状不同,列向各随其册。映射行没有独立结论词表:里程碑是版本化登记的实例
// 参数,原词直示;分诊行的 outcome 是封闭四格,配词表。
const kindColumns: Record<JudgmentKind, ListColumn<RuleRow>[]> = {
  MILESTONE_MAPPING: [
    col('version', '映射版本', true),
    col('effective', '生效区间', true),
    col('source', '源上下文'),
    col('factKind', '事实类型', true),
    col('milestone', '标准里程碑', true),
    col('approvedBy', '批准人', true),
  ],
  TRIAGE_RULE: [
    col('version', '规则版本', true),
    col('effective', '生效区间', true),
    col('signalKind', '信号类型', true),
    col('confidence', '可信度', true),
    col('outcome', '分诊结果'),
    col('approvedBy', '批准人', true),
  ],
};

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

function rowsOf(body: JudgmentListBody): RuleRow[] {
  switch (body.kind) {
    case 'MILESTONE_MAPPING':
      return body.catalogues.flatMap((catalogue) =>
        catalogue.entries.map((entry) => ({
          // 条目键取登记入口的防撞键(源上下文 + 事实类型,checkEntries keyOf)。
          key: `mapping:${catalogue.version}:${entry.source}:${entry.factKind}`,
          values: {
            version: catalogue.version,
            effective: formatRange(catalogue.effectiveFrom, catalogue.effectiveTo),
            source: labelOf(sourceContextLabels, entry.source),
            factKind: entry.factKind,
            milestone: entry.milestone,
            approvedBy: catalogue.approvedBy,
          },
        })),
      );
    case 'TRIAGE_RULE':
      return body.catalogues.flatMap((catalogue) =>
        catalogue.entries.map((entry) => ({
          key: `triage:${catalogue.version}:${entry.signalKind}:${entry.confidence}`,
          values: {
            version: catalogue.version,
            effective: formatRange(catalogue.effectiveFrom, catalogue.effectiveTo),
            signalKind: entry.signalKind,
            confidence: entry.confidence,
            outcome: labelOf(triageOutcomeLabels, entry.outcome),
            approvedBy: catalogue.approvedBy,
          },
        })),
      );
  }
}

export function TrackingJudgmentRulesPage() {
  const [kind, setKind] = useState<JudgmentKind>('MILESTONE_MAPPING');
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    kind: JudgmentKind;
    answer: ApiResult<JudgmentListBody>;
  } | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listVisibilityCatalogues(kind).then((answer) => {
      if (!cancelled) setLoaded({ kind, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [kind, reloadKey]);

  const answer = loaded?.kind === kind ? loaded.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? rowsOf(body) : [];
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
      description={`${info.owner}——里程碑映射与分诊规则两册版本化判断规则。映射按源上下文与事实类型一行覆盖此后同类型事实,无法可靠归类时保持未归类;分诊按信号类型与可信度给出封闭四格结论,信号类型与可信度是开放引用按原词直示`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索版本、事实类型、里程碑或信号',
      }}
      filters={
        <>
          {judgmentKinds.map((candidate) => (
            <button
              key={candidate}
              type="button"
              className={chipClass(candidate === kind)}
              onClick={() => setKind(candidate)}
            >
              {visibilityCatalogueKindLabels[candidate]}
            </button>
          ))}
        </>
      }
      filterSummary={
        // 计数只在拿到业务答案后显示,未配置态不报「0 条」(与既有目录页同一守卫)。
        body
          ? `${visibilityCatalogueKindLabels[kind]} ${body.catalogues.length} 版 / ${rows.length} 条`
          : undefined
      }
      columns={kindColumns[kind]}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /visibility-catalogues?kind=${kind}`,
        emptyTitle: `当前租户尚无${visibilityCatalogueKindLabels[kind]}`,
        emptyDescription: '读取入口已配置,但该目录为空;页面不会生成默认规则。',
      })}
    />
  );
}
