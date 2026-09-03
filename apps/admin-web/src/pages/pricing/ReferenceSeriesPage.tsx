import { useEffect, useState } from 'react';
import { Button, Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import {
  ListPageTemplate,
  type ListColumn,
  type TemplateViewState,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import {
  evidenceGradeLabels,
  labelOf,
  problemNote,
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
import { CoverageSummaryBar } from './CoverageSummaryBar';

const info = moduleInfoById['reference-series'];

const columns: ListColumn<ReferenceSeriesRecord>[] = [
  {
    id: 'series',
    header: '序列',
    className: 'w-[160px]',
    render: (row) => (
      <div>
        <div className="font-mono text-[12px] text-idpxyz-accent">{row.seriesId}</div>
        <div className="font-mono text-[11px] text-idpxyz-textMuted">{row.seriesVersion}</div>
      </div>
    ),
  },
  {
    id: 'kind',
    header: '种类',
    align: 'center',
    className: 'w-[88px]',
    render: (row) => labelOf(seriesKindLabels, row.kind),
  },
  {
    id: 'source',
    header: '序列来源标识',
    render: (row) => <span className="font-mono text-[12px]">{row.sourceIdentifier}</span>,
  },
  {
    id: 'range',
    header: '生效区间',
    className: 'w-[200px]',
    render: (row) => (
      <span className="font-mono text-[12px]">
        {row.effectiveFrom}
        {row.effectiveTo ? ` → ${row.effectiveTo}` : ' → 开放'}
      </span>
    ),
  },
  {
    id: 'basis',
    header: '汇率口径',
    render: (row) =>
      row.quoteBasisId ? (
        <span className="font-mono text-[12px]">
          {row.quoteBasisId}@{row.quoteBasisVersion}
        </span>
      ) : (
        <span className="text-idpxyz-textMuted">—</span>
      ),
  },
  {
    id: 'grade',
    header: '证据等级',
    align: 'center',
    render: (row) => labelOf(evidenceGradeLabels, row.evidenceGrade),
  },
  {
    id: 'corrects',
    header: '更正关系',
    render: (row) =>
      row.priorVersion ? (
        <div>
          <div className="font-mono text-[12px]">更正自 {row.priorVersion}</div>
          {row.correctionBasis ? (
            <div className="text-[11px] text-idpxyz-textMuted">{row.correctionBasis}</div>
          ) : null}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">无</span>
      ),
  },
  {
    id: 'canon',
    header: '规范化 / 摘要',
    render: (row) => (
      <div>
        <div className="font-mono text-[12px]">{row.canonicalization}</div>
        <div className="font-mono text-[11px] text-idpxyz-textMuted" title={row.contentDigest}>
          {row.contentDigest.length > 16
            ? `${row.contentDigest.slice(0, 16)}…`
            : row.contentDigest}
        </div>
      </div>
    ),
  },
  { id: 'registrant', header: '登记责任方', render: (row) => row.registrant },
  {
    id: 'registered',
    header: '登记时间',
    className: 'w-[180px]',
    render: (row) => <span className="font-mono text-[12px]">{row.registeredAt}</span>,
  },
];

// 行动作列单独拼装：它要 setState，而上面那张表是模块级常量。
function columnsWithReview(
  onReview: (target: SeriesReviewTarget) => void,
): ListColumn<ReferenceSeriesRecord>[] {
  return [
    ...columns,
    {
      id: 'review',
      header: '复核',
      align: 'center',
      className: 'w-[72px]',
      render: (row) => (
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
      ),
    },
  ];
}

function viewStateOf(
  answer: ApiResult<ReferenceSeriesListResponseBody> | null,
  rowCount: number,
  retry: () => void,
): TemplateViewState {
  if (answer === null) return { kind: 'loading' };
  switch (answer.kind) {
    case 'outcome':
      return rowCount === 0
        ? {
            kind: 'empty',
            title: '当前租户内尚无参考序列版本',
            description:
              '空列表是正常业务答案(REFERENCE_SERIES_LISTED),不是故障;经受控登记口登记序列后本页即可见。',
          }
        : { kind: 'ready' };
    case 'unconfigured':
      return {
        kind: 'unconfigured',
        title: '接入渠道未配置',
        description:
          '计价参考序列查阅端点(GET /pricing-reference-series)已建立并装配,但接入渠道认证方式未登记,' +
          '服务端按 ADR-0055 如实答 403 ACCESS_CHANNEL_NOT_CONFIGURED。这是诚实答案不是接线缺陷;' +
          '改请求或重试不会改变结果。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock:
            '登记接入渠道认证参数(PAR-INT-01,实例半边)后由装配侧换上真 Intake 即放行;' +
            '序列数值仍由外部来源产生、经登记口登记(ADR-0013)。在线登记口本身已建立' +
            '(见「登记序列」签,ADR-0085),它挂的是同一堵墙,因此今天同答未配置。',
        },
      };
    case 'callerProblem':
      return {
        kind: 'error',
        title: `调用方式问题(HTTP ${answer.status})`,
        description: problemNote(answer.code),
      };
    case 'noAnswer':
      return {
        kind: 'error',
        title: `服务端未形成答案(HTTP ${answer.status})`,
        description: problemNote(answer.code),
        onRetry: retry,
      };
    case 'transport':
      return {
        kind: 'error',
        title: '请求未到达 parcel-api',
        description: `${answer.message};请确认代理与服务端在运行(约定走 /api 经代理转发)。`,
        onRetry: retry,
      };
  }
}

/**
 * 计价参考序列:已登记序列版本的查阅/复核面,外加登记签(ADR-0085,票
 * admin-write-faces/01 切片 01b)。
 *
 * 登记签只有登记一个动作,没有行级修改或删除面:参考序列只登记不产生数值(ADR-0013),
 * 数值由外部来源产生;登记册本身不可覆盖。
 */
function ReferenceSeriesTable() {
  const [keyword, setKeyword] = useState('');
  const [answer, setAnswer] = useState<ApiResult<ReferenceSeriesListResponseBody> | null>(
    null,
  );
  const [reloadToken, setReloadToken] = useState(0);
  const [reviewing, setReviewing] = useState<SeriesReviewTarget | null>(null);

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

  const retry = () => setReloadToken((token) => token + 1);
  const rows = answer?.kind === 'outcome' ? answer.body.series : [];
  const needle = keyword.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        [row.seriesId, row.sourceIdentifier, row.registrant].some((field) =>
          field.toLowerCase().includes(needle),
        ),
      )
    : rows;

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* 摘要条摆在目录之上而不是另开一页：本票要的是「让缺口在当天被看见」，
          而另开一页等于要人先想起来去看它。 */}
      <CoverageSummaryBar />
      <ListPageTemplate<ReferenceSeriesRecord>
        title={info.title}
        description={`${info.owner}——计价只登记不生产数值(ADR-0013)`}
        search={{
          value: keyword,
          onChange: setKeyword,
          placeholder: '搜索序列标识 / 来源 / 登记责任方',
        }}
        filterSummary={
          answer?.kind === 'outcome' ? `共 ${visibleRows.length} 条` : undefined
        }
        columns={columnsWithReview(setReviewing)}
        rows={visibleRows}
        rowKey={(row) => `${row.seriesId}@${row.seriesVersion}`}
        viewState={viewStateOf(answer, rows.length, retry)}
      />
      {reviewing ? (
        <SeriesReviewPanel
          // 换一行复核时重建面板：结论与依据是上一行的，留着会让人把 A 的依据提给 B。
          key={`${reviewing.seriesId}@${reviewing.seriesVersion}`}
          target={reviewing}
          onClose={() => setReviewing(null)}
        />
      ) : null}
    </div>
  );
}

export function ReferenceSeriesPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalog" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">参考序列</TabsTrigger>
          {/* 「高级」二字是 ADR-0101 决定一的落点，不是措辞偏好：JSON 快照口退为受控批量
              口的在线镜像，**不是运营配置员的主路径**。主路径（逐字段表单 + 提交前预览）
              要一个「先校验、回摘要与证据等级、尚未登记」的后端步骤，本册今天没有——同
              ADR 决定三、四为价卡立的那套草稿/预览机制，参考序列册按决定八另裁另建。
              在那之前这一签仍是唯一在线入口，所以它留着而不是藏起来。 */}
          <TabsTrigger value="register">高级：JSON 登记口</TabsTrigger>
        </TabsList>
        <TabsContent
          value="catalog"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <ReferenceSeriesTable />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <RegistrationPanel
            moduleId="reference-series"
            title="登记参考序列版本（高级：受控批量口的在线镜像）"
            endpoint="POST /pricing-reference-series-registrations"
            snapshotHint="登记快照 JSON 的形状与受控登记口 parcel-pricing-register -kind reference-series -file 吃的同一份。这是受控批量口的在线镜像，供 API 集成方与批量登记用；运营配置员的主路径是逐字段表单加提交前预览（证据等级、内容摘要、与上一版逐期差异），那一路需要一个先校验、回摘要而尚未登记的后端步骤，本册今天还没有——ADR-0101 决定三、四已为价卡立了同构的草稿与预览机制，本册按决定八另裁另建。在那之前这一口仍是唯一在线入口。"
            submit={registerReferenceSeries}
            outcomeLabels={registrationOutcomeLabels}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
