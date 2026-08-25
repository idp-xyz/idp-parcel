import { useEffect, useState } from 'react';
import {
  ListPageTemplate,
  type ListColumn,
  type TemplateViewState,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { directionLabels, purposeLabels, labelOf, problemNote } from './presentation';
import {
  listPriceCards,
  type ApiResult,
  type PriceCardListResponseBody,
  type PriceCardRecord,
} from './api';

const info = moduleInfoById['price-card-catalog'];

const columns: ListColumn<PriceCardRecord>[] = [
  {
    id: 'plan',
    header: '定价方案',
    className: 'w-[140px]',
    render: (row) => (
      <div>
        <div className="font-mono text-[12px] text-idpxyz-accent">{row.planId}</div>
        <div className="font-mono text-[11px] text-idpxyz-textMuted">{row.planVersion}</div>
      </div>
    ),
  },
  {
    id: 'direction',
    header: '价格方向',
    align: 'center',
    className: 'w-[72px]',
    render: (row) => labelOf(directionLabels, row.direction),
  },
  {
    id: 'purpose',
    header: '计算目的',
    render: (row) => (
      <span className="font-mono text-[12px]">{labelOf(purposeLabels, row.purpose)}</span>
    ),
  },
  { id: 'scope', header: '适用范围', render: (row) => row.scope },
  {
    id: 'rate-table',
    header: '价表',
    render: (row) => (
      <span className="font-mono text-[12px]">
        {row.rateTableId}@{row.rateTableVersion}
      </span>
    ),
  },
  {
    id: 'period',
    header: '适用期',
    className: 'w-[200px]',
    render: (row) => (
      <span className="font-mono text-[12px]">
        {row.effectiveFrom}
        {row.effectiveTo ? ` → ${row.effectiveTo}` : ' → 开放'}
      </span>
    ),
  },
  {
    id: 'canon',
    header: '规范化版本',
    align: 'center',
    render: (row) => <span className="font-mono text-[12px]">{row.canonicalization}</span>,
  },
  {
    id: 'digest',
    header: '版本内容摘要',
    render: (row) => (
      <span className="font-mono text-[11px]" title={row.contentDigest}>
        {row.contentDigest.length > 16
          ? `${row.contentDigest.slice(0, 16)}…`
          : row.contentDigest}
      </span>
    ),
  },
  {
    id: 'source',
    header: '源文件身份',
    render: (row) => (
      <div>
        <div>{row.sourceFileName}</div>
        <div className="font-mono text-[11px] text-idpxyz-textMuted" title={row.sourceFileSha256}>
          {row.sourceFileSha256.slice(0, 12)}…
        </div>
      </div>
    ),
  },
  {
    id: 'auth',
    header: '发布授权',
    render: (row) => (
      <span className="font-mono text-[12px]">
        {row.authorizationId}@{row.authorizationVersion}
      </span>
    ),
  },
  { id: 'approver', header: '发布批准责任方', render: (row) => row.publicationApprover },
  {
    id: 'registered',
    header: '登记时间',
    className: 'w-[180px]',
    render: (row) => <span className="font-mono text-[12px]">{row.registeredAt}</span>,
  },
];

function viewStateOf(
  answer: ApiResult<PriceCardListResponseBody> | null,
  rowCount: number,
  retry: () => void,
): TemplateViewState {
  if (answer === null) return { kind: 'loading' };
  switch (answer.kind) {
    case 'outcome':
      return rowCount === 0
        ? {
            kind: 'empty',
            title: '当前租户内尚无价卡版本',
            description:
              '空列表是正常业务答案(PRICE_CARDS_LISTED),不是故障;经受控登记口登记价卡后本页即可见。',
          }
        : { kind: 'ready' };
    case 'unconfigured':
      return {
        kind: 'unconfigured',
        title: '接入渠道未配置',
        description:
          '价卡目录查阅端点(GET /pricing-price-cards)已建立并装配,但接入渠道认证方式未登记,' +
          '服务端按 ADR-0055 如实答 403 ACCESS_CHANNEL_NOT_CONFIGURED。这是诚实答案不是接线缺陷;' +
          '改请求或重试不会改变结果。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock:
            '登记接入渠道认证参数(PAR-INT-01,实例半边)后由装配侧换上真 Intake 即放行;' +
            '价卡登记仍走受控登记口(parcel-pricing-register),不进在线面。',
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
 * 价卡目录:已登记定价方案版本的查阅/复核面。
 * 页面刻意没有「新建 / 登记」动作——价卡登记是治理动作,走受控登记口。
 */
export function PriceCardCatalogPage() {
  const [keyword, setKeyword] = useState('');
  const [answer, setAnswer] = useState<ApiResult<PriceCardListResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listPriceCards().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  const retry = () => setReloadToken((token) => token + 1);
  const rows = answer?.kind === 'outcome' ? answer.body.cards : [];
  const needle = keyword.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        [row.planId, row.sourceFileName, row.publicationApprover].some((field) =>
          field.toLowerCase().includes(needle),
        ),
      )
    : rows;

  return (
    <ListPageTemplate<PriceCardRecord>
      title={info.title}
      description={`${info.owner}——价卡登记走受控登记口,本页只查阅`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '搜索定价方案 / 源文件身份 / 批准责任方',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `共 ${visibleRows.length} 条` : undefined
      }
      columns={columns}
      rows={visibleRows}
      rowKey={(row) => `${row.planId}@${row.planVersion}`}
      viewState={viewStateOf(answer, rows.length, retry)}
    />
  );
}
