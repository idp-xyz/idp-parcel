import { useEffect, useState } from 'react';
import {
  ListPageTemplate,
  type ListColumn,
  type TemplateViewState,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import {
  evidenceGradeLabels,
  labelOf,
  problemNote,
  seriesKindLabels,
} from './presentation';
import {
  listReferenceSeries,
  type ApiResult,
  type ReferenceSeriesListResponseBody,
  type ReferenceSeriesRecord,
} from './api';

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
            '序列数值仍由外部来源产生、经登记口登记(ADR-0013)。',
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
 * 计价参考序列:已登记序列版本的查阅/复核面。
 * 页面刻意没有「登记 / 修改」动作——序列登记走受控登记口,且数值由外部产生。
 */
export function ReferenceSeriesPage() {
  const [keyword, setKeyword] = useState('');
  const [answer, setAnswer] = useState<ApiResult<ReferenceSeriesListResponseBody> | null>(
    null,
  );
  const [reloadToken, setReloadToken] = useState(0);

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
      columns={columns}
      rows={visibleRows}
      rowKey={(row) => `${row.seriesId}@${row.seriesVersion}`}
      viewState={viewStateOf(answer, rows.length, retry)}
    />
  );
}
