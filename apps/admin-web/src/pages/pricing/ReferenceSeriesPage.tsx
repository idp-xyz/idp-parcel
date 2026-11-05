import { useEffect, useState } from 'react';
import { Button, Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import type { StatusTone } from '../../vocabulary/status';
import { catalogueViewState } from '../catalogue-view';
import { technicalRows } from '../settlement/ops-ui';
import { useMayWriteRegistry } from '../use-operator-grants';
import {
  evidenceGradeLabels,
  labelOf,
  problemNote,
  reviewDecisionLabels,
  seriesKindLabels,
} from './presentation';
import {
  listReferenceSeries,
  registerReferenceSeries,
  registrationOutcomeLabels,
  type ApiResult,
  type ReferenceSeriesListResponseBody,
  type ReferenceSeriesRecord,
} from './api';
import { SeriesReviewPanel, type SeriesReviewTarget } from './SeriesReviewPanel';
import { SeriesRegistrationForm } from './SeriesRegistrationForm';
import { CoverageSummaryBar } from './CoverageSummaryBar';
import { visibleSeriesRows, type SeriesKey } from './catalogue-filter';
import { correctionDraftOf, emptySeriesDraft } from './series-form';
import { formatInstant } from '../moment';

const info = moduleInfoById['reference-series'];

// 页名「汇率与燃油费率」，领域里叫「计价参考序列」：计价要用的汇率、燃油费率、按期公布的金额，每一项是一条序列、每次
// 登记是一个版本。列只留运营认得出一版数值所需的几样：哪一项、什么种类、数据从哪来、管哪段时间、复核到哪一步、何时登记。
// 证据等级、汇率口径、更正回指、规范化版本、两条摘要、登记人进「技术详情」——对账、查争议时展开可复制。
//
// 「在用」不上列：在用是相对某一刻派生的结论，列表没有那个时刻，它归页顶的「还能用多久」摘要条（那条带截至时刻）。

/** 复核那一格的词与色：没人复核过是要人去做的事（琥珀），最近一次退回要登记人更正（红），最近一次通过（绿）。 */
function reviewState(row: ReferenceSeriesRecord): { label: string; tone: StatusTone } {
  if (row.reviewCount === 0) return { label: '待复核', tone: 'warn' };
  if (row.lastReviewDecision === 'RETURNED') return { label: '已退回', tone: 'bad' };
  if (row.lastReviewDecision === 'APPROVED') return { label: '已通过', tone: 'good' };
  // 集外结论原样显示，不代折成已知格。
  return { label: labelOf(reviewDecisionLabels, row.lastReviewDecision ?? ''), tone: 'muted' };
}

function seriesTechnicalRows(row: ReferenceSeriesRecord) {
  return technicalRows([
    { label: '序列', value: `${row.seriesId}@${row.seriesVersion}` },
    { label: '种类码', value: row.kind },
    { label: '凭证', value: labelOf(evidenceGradeLabels, row.evidenceGrade) },
    {
      label: '汇率口径',
      value: row.quoteBasisId ? `${row.quoteBasisId}@${row.quoteBasisVersion ?? ''}` : undefined,
    },
    { label: '更正自', value: row.priorVersion },
    { label: '更正理由', value: row.correctionBasis },
    { label: '规范化版本', value: row.canonicalization },
    { label: '内容摘要', value: row.contentDigest },
    { label: '引用摘要', value: row.referenceDigest },
    { label: '登记人', value: row.registrant },
  ]);
}

const columns: ListColumn<ReferenceSeriesRecord>[] = [
  {
    id: 'series',
    header: '名称',
    render: (row) => (
      <div className="min-w-40">
        <div className="break-all font-mono type-body text-idpxyz-textBright">{row.seriesId}</div>
        <div className="type-caption text-idpxyz-textMuted">
          版本 {row.seriesVersion}
          {row.priorVersion ? ` · 更正 ${row.priorVersion}` : ''}
        </div>
      </div>
    ),
  },
  {
    id: 'kind',
    header: '种类',
    className: 'whitespace-nowrap',
    render: (row) => labelOf(seriesKindLabels, row.kind),
  },
  {
    id: 'source',
    header: '数据来源',
    render: (row) => <span className="break-all font-mono type-body">{row.sourceIdentifier}</span>,
  },
  {
    id: 'range',
    header: '管哪段时间',
    className: 'whitespace-nowrap',
    render: (row) => (
      <span className="font-mono type-body">
        {formatInstant(row.effectiveFrom)}
        {row.effectiveTo ? ` → ${formatInstant(row.effectiveTo)}` : ' → 长期'}
      </span>
    ),
  },
  {
    id: 'review',
    header: '复核',
    render: (row) => {
      const state = reviewState(row);
      return (
        <div className="whitespace-nowrap">
          <StatusPill label={state.label} tone={state.tone} />
          {row.reviewCount > 0 ? (
            <div className="mt-0.5 type-caption text-idpxyz-textMuted">
              {row.approvedReviewCount}/{row.reviewCount} 次通过
              {row.lastReviewedAt ? ` · ${formatInstant(row.lastReviewedAt)}` : ''}
            </div>
          ) : null}
        </div>
      );
    },
  },
  {
    id: 'registered',
    header: '登记时间',
    className: 'whitespace-nowrap',
    render: (row) => <span className="font-mono type-body">{formatInstant(row.registeredAt)}</span>,
  },
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={seriesTechnicalRows(row)} className="max-w-md open:min-w-[22rem]" />,
  },
];

// 行动作列单独拼装：它要 setState，而上面那张表是模块级常量。
// 两个动作都不改原行：复核是追加在版本之旁的独立事实，更正是登记一个回指它的新版本。
// 页面上因此没有「编辑」——登记册不可覆盖（ADR-0013）。没有写授予时整列收起（ADR-0315）。
function columnsWithActions(
  onReview: (target: SeriesReviewTarget) => void,
  onCorrect: (row: ReferenceSeriesRecord) => void,
): ListColumn<ReferenceSeriesRecord>[] {
  return [
    ...columns,
    {
      id: 'actions',
      header: '操作',
      align: 'center',
      className: 'w-[180px]',
      render: (row) => (
        <div className="flex items-center justify-center gap-2">
          <Button
            variant="outline"
            onClick={() =>
              onReview({
                seriesId: row.seriesId,
                seriesVersion: row.seriesVersion,
                registrant: row.registrant,
              })
            }
          >
            复核
          </Button>
          <Button variant="outline" onClick={() => onCorrect(row)}>
            更正
          </Button>
        </div>
      ),
    },
  ];
}

/**
 * 计价参考序列:已登记序列版本的查阅/复核面,外加登记签(ADR-0085,票
 * admin-write-faces/01 切片 01b;逐字段表单与预览见票 pricing-reference-series-operations/08)。
 *
 * 行级动作只有「复核」与「更正此版本」,没有修改或删除面:参考序列只登记不产生数值
 * (ADR-0013),数值由外部来源产生;登记册本身不可覆盖,更正是登记一个回指原版的新版本。
 *
 * `reloadToken` 由页面持有而不在本组件内:登记签与更正面板登记成功后目录要跟着刷新,而
 * 登记签是本组件的兄弟不是子孙。
 */
function ReferenceSeriesTable({
  reloadToken,
  requestReload,
  mayWrite,
}: {
  reloadToken: number;
  requestReload: () => void;
  mayWrite: boolean;
}) {
  const [keyword, setKeyword] = useState('');
  const [answer, setAnswer] = useState<ApiResult<ReferenceSeriesListResponseBody> | null>(
    null,
  );
  // 复核面板与更正面板只开一个：两者都挂在目录之下，同时开着会让人把 A 的依据写到 B 里。
  const [reviewing, setReviewing] = useState<SeriesReviewTarget | null>(null);
  const [correcting, setCorrecting] = useState<ReferenceSeriesRecord | null>(null);
  // 「只看这条序列」与搜索词是两种收窄，分开持有；判据在 catalogue-filter.ts。
  const [focus, setFocus] = useState<SeriesKey | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listReferenceSeries().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  const retry = requestReload;
  const rows = answer?.kind === 'outcome' ? answer.body.series : [];
  const visibleRows = visibleSeriesRows(rows, { focus, keyword });
  const openReview = (target: SeriesReviewTarget) => {
    setCorrecting(null);
    setReviewing(target);
  };
  const openCorrection = (row: ReferenceSeriesRecord) => {
    setReviewing(null);
    setCorrecting(row);
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* 摘要条摆在目录之上而不是另开一页：本票要的是「让缺口在当天被看见」，
          而另开一页等于要人先想起来去看它。 */}
      <CoverageSummaryBar onSelectSeries={setFocus} focusedSeries={focus} />
      <ListPageTemplate<ReferenceSeriesRecord>
        title={info.title}
        description="计价用到的汇率、燃油费率等每期数值：外部来源公布新一期后在这里登记，复核通过才参与计价"
        search={{
          value: keyword,
          onChange: setKeyword,
          placeholder: '搜名称、数据来源、登记人',
        }}
        filterSummary={
          answer?.kind === 'outcome'
            ? focus
              ? `${visibleRows.length} 个版本 · 只看 ${focus.seriesId}（${labelOf(seriesKindLabels, focus.kind)}）`
              : `${visibleRows.length} 个版本`
            : undefined
        }
        columns={mayWrite ? columnsWithActions(openReview, openCorrection) : columns}
        rows={visibleRows}
        rowKey={(row) => `${row.seriesId}@${row.seriesVersion}`}
        emptyRowsNote="没有匹配的版本"
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /pricing-reference-series',
          emptyTitle: '还没有登记任何汇率或燃油费率',
          emptyDescription: mayWrite
            ? '在「登记数值」签里登记第一条；价格表里用到汇率或燃油费率的费用项要等它登记、复核通过后才算得出来。'
            : '请有登记权限的同事登记；价格表里用到汇率或燃油费率的费用项要等它登记、复核通过后才算得出来。',
        })}
      />
      {reviewing ? (
        <SeriesReviewPanel
          // 换一行复核时重建面板：结论与依据是上一行的，留着会让人把 A 的依据提给 B。
          key={`${reviewing.seriesId}@${reviewing.seriesVersion}`}
          target={reviewing}
          onClose={() => setReviewing(null)}
        />
      ) : null}
      {correcting ? (
        <SeriesRegistrationForm
          // 同一条理由：换一行更正时重建表单，预填与预览都是上一行的。
          key={`${correcting.seriesId}@${correcting.seriesVersion}`}
          initialDraft={correctionDraftOf(correcting)}
          onRecorded={requestReload}
          onClose={() => setCorrecting(null)}
        />
      ) : null}
    </div>
  );
}

export function ReferenceSeriesPage() {
  const [reloadToken, setReloadToken] = useState(0);
  const requestReload = () => setReloadToken((token) => token + 1);
  // 没有登记册写授予的操作者不摆登记签与行上的复核 / 更正：写口照旧自己判授予，这里只是呈现（ADR-0315）。
  const mayWrite = useMayWriteRegistry();
  const [tab, setTab] = useState('catalog');
  useEffect(() => {
    if (!mayWrite && tab !== 'catalog') setTab('catalog');
  }, [mayWrite, tab]);

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">全部数值</TabsTrigger>
          {/* 运营配置员的主路径：逐字段表单 + 提交前预览（ADR-0101 决定一；本册按决定八自裁，理由见
              票 pricing-reference-series-operations/08）。 */}
          {mayWrite ? <TabsTrigger value="form">登记数值</TabsTrigger> : null}
          {/* 「高级」二字是 ADR-0101 决定一的落点：JSON 快照口退为受控批量口的在线镜像，不是运营配置员的主路径；
              API 集成方与批量登记仍走这一形，两条路打同一个端点、同一段登记用例。 */}
          {mayWrite ? <TabsTrigger value="register">高级：批量导入（JSON）</TabsTrigger> : null}
        </TabsList>
        <TabsContent
          value="catalog"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <ReferenceSeriesTable reloadToken={reloadToken} requestReload={requestReload} mayWrite={mayWrite} />
        </TabsContent>
        {mayWrite ? (
          <TabsContent
            value="form"
            className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
          >
            <SeriesRegistrationForm initialDraft={emptySeriesDraft()} onRecorded={requestReload} />
          </TabsContent>
        ) : null}
        {mayWrite ? (
          <TabsContent
            value="register"
            className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
          >
            <RegistrationPanel
              moduleId="reference-series"
              title="批量导入汇率与费率（JSON）"
              endpoint="POST /pricing-reference-series-registrations"
              snapshotHint="给系统对接或一次导入多期数值用：贴入整份 JSON 直接登记，格式与命令行导入工具相同。日常登记一期新数值，用「登记数值」签逐项填写、先预览再登记更稳妥。"
              submit={registerReferenceSeries}
              outcomeLabels={registrationOutcomeLabels}
              problemNote={problemNote}
            />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
