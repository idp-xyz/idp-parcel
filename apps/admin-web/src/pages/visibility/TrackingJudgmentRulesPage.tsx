import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { MultiRegistrationPanel, type RegistrationTarget } from '../../components/registration';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  listVisibilityCatalogues,
  registerVisibilityCatalogue,
  visibilityRegistrationEndpoints,
  type ApiResult,
  type VisibilityCatalogueListResponseBody,
  type VisibilityRegistrableCatalogueKind,
} from './catalogue-api';
import {
  labelOf,
  problemNote,
  registrationOutcomeLabels,
  registrationRefusalReasonLabels,
  registrationSnapshotHints,
  registrationTitles,
  sourceContextLabels,
  triageOutcomeLabels,
  visibilityCatalogueKindLabels,
} from './presentation';

// VE 目录拆三页,本页装**判断规则**那组:里程碑映射与分诊规则(票
// admin-web-page-wiring-frontier/02 的页面裁决),外加冲突信号规则(票
// ve-disclosure-policy-view/03)。三册同为「事实/信号 → 结论」的判断规则——映射把源
// 事实归到标准里程碑,分诊把信号归到处置结论,冲突信号规则裁投影派生里无法按业务时间
// 裁决的替代链分叉形成哪一类异常信号;通知与披露是对外口径、资格与授权是索赔前置,
// 各归各页,不折成多页签巨面。
//
// 行随条目展开(一行一条规则,版本列随行重复):册子是拿来查「这类事实/信号会
// 判成什么」的,按版本折行会把待查的规则埋进折叠单元格。空版本不存在——登记入口
// 要求每版至少一条(application checkEntries),行展开不会藏掉任何版本。冲突信号规则
// 一租户一条、没有版本壳与生效区间(0025:换版是治理动作,不是接续闭合),行上只有
// 库落下的登记时刻。

const info = moduleInfoById['tracking-judgment-rules'];

type JudgmentKind = 'MILESTONE_MAPPING' | 'TRIAGE_RULE' | 'CONFLICT_SIGNAL_RULE';

const judgmentKinds: JudgmentKind[] = ['MILESTONE_MAPPING', 'TRIAGE_RULE', 'CONFLICT_SIGNAL_RULE'];

// 登记签只铺今天有写面的两册。冲突信号规则的写签跟着本页这枚读签走,归票
// ve-disclosure-policy-view/02 步二在 03 进 main 后铺。
const judgmentRegistrableKinds: Extract<JudgmentKind, VisibilityRegistrableCatalogueKind>[] = [
  'MILESTONE_MAPPING',
  'TRIAGE_RULE',
];

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
  // 冲突信号规则没有生效区间列,位置上换成登记时刻:那是库落下的事实,不是生效边界。
  CONFLICT_SIGNAL_RULE: [
    col('signalKind', '信号类型', true),
    col('version', '规则版本', true),
    col('confidence', '可信度依据', true),
    col('approvedBy', '批准人', true),
    col('registeredAt', '登记时刻', true),
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
    case 'CONFLICT_SIGNAL_RULE':
      return body.catalogues.map((record) => ({
        // 一租户一条(0025 键只有租户),行键取信号类型 + 规则版本足以区分并存的历史版。
        key: `conflict:${record.signalKind}:${record.version}`,
        values: {
          signalKind: record.signalKind,
          version: record.version,
          confidence: record.confidence,
          approvedBy: record.approvedBy,
          registeredAt: formatInstant(record.registeredAt),
        },
      }));
  }
}

function TrackingJudgmentRulesTable() {
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
      description={`${info.owner}——里程碑映射、分诊规则与冲突信号规则三册判断规则。映射按源上下文与事实类型一行覆盖此后同类型事实,无法可靠归类时保持未归类;分诊按信号类型与可信度给出封闭四格结论;冲突信号规则(0025)一租户一条,答无法按业务时间裁决的替代链分叉形成哪一类异常信号、依据哪版识别规则、记什么可信度依据。信号类型与可信度是开放引用按原词直示`}
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
        // 冲突信号规则没有版本壳,只报条数。
        body?.kind === 'CONFLICT_SIGNAL_RULE'
          ? `${visibilityCatalogueKindLabels[body.kind]} ${rows.length} 条`
          : body
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

// 登记签装本页读签里有写面的两册,不多铺:多铺一册会让同一本册在两处都能登,而其中一处的
// 页面上根本看不到登进去的结果。冲突信号规则的写签见 judgmentRegistrableKinds 注。
const registrationTargets: RegistrationTarget[] = judgmentRegistrableKinds.map((candidate) => ({
  id: candidate,
  label: visibilityCatalogueKindLabels[candidate],
  title: registrationTitles[candidate],
  endpoint: `POST ${visibilityRegistrationEndpoints[candidate]}`,
  snapshotHint: registrationSnapshotHints[candidate],
  submit: (snapshot) => registerVisibilityCatalogue(candidate, snapshot),
  outcomeLabels: registrationOutcomeLabels,
  refusalReasonLabels: registrationRefusalReasonLabels,
}));

/**
 * 判断规则三册:逐册查阅版本原文,外加两册的登记签(ADR-0085,票 admin-write-faces/02 切片 02d;
 * 冲突信号规则今天只有读签,票 ve-disclosure-policy-view/03)。
 *
 * 登记签不是「新建一版」的表单:目录修订按笔推进,新版翻旧插新、不覆盖行,同版本号再登
 * 一律答版本不可覆盖——所以这里只有登记一个动作,没有行级编辑或删除面。墙降之前它必然
 * 答 403「接入渠道未配置」,那是诚实答案;墙降当天在装配点换真 Intake 即点亮,本页一行
 * 不用改。
 */
export function TrackingJudgmentRulesPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalogue" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalogue">判断规则</TabsTrigger>
          <TabsTrigger value="register">登记判断规则</TabsTrigger>
        </TabsList>
        <TabsContent
          value="catalogue"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <TrackingJudgmentRulesTable />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <MultiRegistrationPanel
            moduleId="tracking-judgment-rules"
            targets={registrationTargets}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
