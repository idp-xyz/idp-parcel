import { useEffect, useState } from 'react';
import {
  ListPageTemplate,
  type ListColumn,
  type TemplateViewState,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { sourceContextLabels, problemNote } from './presentation';
import {
  listTrackingProjections,
  type ApiResult,
  type ProjectionEntryRecord,
  type ProjectionListResponseBody,
  type TrackingProjectionRecord,
} from './api';

// 页面标题的唯一来源是 navigation 的 moduleInfoById,只读引用,不抄第二份。
const info = moduleInfoById['tracking-projection'];

// 全程追踪(visibility-exception),对 GET /tracking-projections 真实端点取数。
//
// 作用域裁决已落:运营查阅走独立读口消费投影库,不复用客户隔离读口(ADR-0076
// 「运营追踪查阅走独立读口消费投影库」;为什么复用等于结构性漏报,四层取证见该
// 记录 Context)。作用域是租户级、无客户维,由服务端接入面从认证结果铸造,本页
// 不送任何自报身份参数;「投影未形成」对租户内已授权查阅是如实的业务答案,本页
// 不背客户面 VIEW_NOT_FOUND 的探针合并义务。
//
// 栏目跟真实读模型走(与委托查阅页同一条先例):行是投影版本,格全部从端点体
// 派生。五个源上下文各占一列、里程碑另占一列,不折成统一状态机——GLOSSARY
// 「追踪摘要」禁统一状态机那条规则仍是本表形状的出处。两类此前上过骨架的列
// 这里刻意没有:
//   - 物流进展/位置或控制范围/交付或退运进展等高阶维度词:传输层把维度派生交给
//     读侧「按条目的来源与类型」,而事实类型(kind)是源上下文拥有的开放词表,
//     kind→维度的对照与标准里程碑映射同族(版本化登记、实例半边),未登记前本页
//     按源上下文分列——源是传输层封闭五元,零发明;
//   - 追踪摘要、当前 ETA、可见性缺口、异常影响:是本上下文拥有的独立对象,投影
//     读模型未携带,等各自读面落地再上列,不虚构空列。

/**
 * 当前有效条目 = 未被同投影内任何条目指名为前身的条目。替代关系只在同一源上下文、
 * 同一事实引用的版本之间成立(CONTEXT「来源事实替代关系」),判据据此取
 * (source, fact, 版本) 三段;被替代条目继续在场是为可追溯,各维度与里程碑只由
 * 当前有效条目派生(CONTEXT「追踪投影与里程碑」)。
 */
function currentEffectiveEntries(entries: ProjectionEntryRecord[]): ProjectionEntryRecord[] {
  const superseded = new Set(
    entries
      .filter((entry) => entry.supersedes)
      .map((entry) => `${entry.source}\u0000${entry.fact}\u0000${entry.supersedes}`),
  );
  return entries.filter(
    (entry) => !superseded.has(`${entry.source}\u0000${entry.fact}\u0000${entry.factVersion}`),
  );
}

/** 行 = 一份当前投影版本;各格全部从端点体派生,不引入端点体之外的字段。 */
interface TrackingProjectionRow {
  parcelId: string;
  projectionVersion: string;
  priorVersion?: string;
  derivedAt: string;
  /** 当前有效已归类条目的最后一条的里程碑;一条都归不上时留空,由列如实示「未归类」。 */
  latestMilestone: string;
  /** 被替代条目数。运营查阅面向替代关系(CONTEXT「运营追踪查阅」),列表级先给计数。 */
  supersededCount: number;
  /** 每源最后一条当前有效条目。条目按业务发生时间自然排序由派生侧保证,本页不另立排序判据。 */
  latestBySource: Partial<Record<string, ProjectionEntryRecord>>;
}

function rowOf(record: TrackingProjectionRecord): TrackingProjectionRow {
  const current = currentEffectiveEntries(record.entries);
  const latestBySource: Partial<Record<string, ProjectionEntryRecord>> = {};
  for (const entry of current) {
    latestBySource[entry.source] = entry;
  }
  const classified = current.filter((entry) => entry.milestone);
  return {
    parcelId: record.parcel,
    projectionVersion: record.version,
    priorVersion: record.priorVersion,
    derivedAt: record.derivedAt,
    latestMilestone: classified.length
      ? (classified[classified.length - 1].milestone as string)
      : '',
    supersededCount: record.entries.length - current.length,
    latestBySource,
  };
}

// 列顺序沿服务链方向(托运→路由→节点→运输→关务)只是阅读便利,不含语义主张。
const sourceColumnOrder = [
  'PARCEL_SHIPMENT',
  'NETWORK_ROUTING',
  'NODE_OPERATIONS',
  'TRANSPORT_FULFILLMENT',
  'CUSTOMS_COMPLIANCE',
];

const sourceColumns: ListColumn<TrackingProjectionRow>[] = sourceColumnOrder.map((source) => ({
  id: `source-${source.toLowerCase()}`,
  // 词表没收录的源原样示码,不猜词。
  header: sourceContextLabels[source] ?? source,
  render: (row) => {
    const entry = row.latestBySource[source];
    // 该源尚无当前有效条目:没有就是没有,不补占位事实。
    if (!entry) return <span className="text-idpxyz-textMuted">—</span>;
    return <span className="font-mono text-[12px]">{entry.kind}</span>;
  },
}));

const columns: ListColumn<TrackingProjectionRow>[] = [
  {
    id: 'parcel',
    header: '包裹标识',
    className: 'w-[160px]',
    render: (row) => (
      <span className="font-mono text-[12px] text-idpxyz-accent">{row.parcelId}</span>
    ),
  },
  ...sourceColumns,
  {
    id: 'milestone',
    header: '最新标准里程碑',
    render: (row) =>
      row.latestMilestone ? (
        <span className="font-mono text-[12px]">{row.latestMilestone}</span>
      ) : (
        // 无法可靠映射时保持未归类是 CONTEXT 硬句;未归类是真话,不是待修的缺陷。
        <span className="text-idpxyz-textMuted">未归类</span>
      ),
  },
  {
    id: 'supersession',
    header: '替代关系',
    className: 'w-[104px]',
    render: (row) =>
      row.supersededCount > 0 ? (
        `${row.supersededCount} 条被替代`
      ) : (
        <span className="text-idpxyz-textMuted">无</span>
      ),
  },
  {
    id: 'derived-at',
    header: '派生时间',
    className: 'w-[210px]',
    render: (row) => <span className="font-mono text-[12px]">{row.derivedAt}</span>,
  },
  {
    id: 'version',
    header: '投影版本',
    className: 'w-[150px]',
    render: (row) => (
      <div>
        <div className="font-mono text-[12px]">{row.projectionVersion}</div>
        {row.priorVersion ? (
          <div className="text-[11px] text-idpxyz-textMuted">
            前身 <span className="font-mono">{row.priorVersion}</span>
          </div>
        ) : null}
      </div>
    ),
  },
];

// 取数答案 → 模板四态。空列表走空态而不是就绪态的空表格:模板明言不从 rows.length
// 推断,「租户内尚无投影」这一业务事实要由本页说出来。
function viewStateOf(
  answer: ApiResult<ProjectionListResponseBody> | null,
  rowCount: number,
  retry: () => void,
): TemplateViewState {
  if (answer === null) return { kind: 'loading' };
  switch (answer.kind) {
    case 'outcome':
      return rowCount === 0
        ? {
            kind: 'empty',
            title: '当前租户内尚无投影',
            description:
              '空列表是正常业务答案(PROJECTIONS_LISTED),不是故障;源上下文接受事实并派生投影后本页即可见。',
          }
        : { kind: 'ready' };
    case 'unconfigured':
      return {
        kind: 'unconfigured',
        title: '接入渠道未配置',
        description:
          '运营追踪查阅端点(GET /tracking-projections)已建立并装配,但接入渠道认证方式未登记,' +
          '服务端按 ADR-0055 如实答 403 ACCESS_CHANNEL_NOT_CONFIGURED。这是诚实答案不是接线缺陷;' +
          '改请求或重试不会改变结果。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock:
            '登记接入渠道认证参数(PAR-INT-01,实例半边)后由装配侧换上真 Intake 即放行;' +
            '运营作用域按 ADR-0076 由认证结果铸造,租户级、无客户维,本页不送自报身份。',
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

export function TrackingProjectionPage() {
  const [keyword, setKeyword] = useState('');
  // null 表示取数中;答案(含各种未形成)一律进 answer,页面不吞任何一格。
  const [answer, setAnswer] = useState<ApiResult<ProjectionListResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listTrackingProjections().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  const retry = () => setReloadToken((token) => token + 1);
  const rows =
    answer?.kind === 'outcome' ? answer.body.projections.map(rowOf) : [];
  // 检索是页面侧对已取回行的便利过滤,不属于查询协议;外部标识不在投影读模型上,
  // 故只按包裹标识与投影版本检索,不承诺读模型没有的定位维。
  const needle = keyword.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        [row.parcelId, row.projectionVersion].some((field) =>
          field.toLowerCase().includes(needle),
        ),
      )
    : rows;

  return (
    <ListPageTemplate<TrackingProjectionRow>
      title={info.title}
      // 页头携带 GLOSSARY「追踪摘要」的硬句原词,与并列分列的表形互为呼应。
      description={`${info.owner}——运营查阅按租户作用域读投影,各源各列,不折成覆盖各源状态的统一状态机`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按包裹标识或投影版本检索',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `共 ${visibleRows.length} 条` : undefined
      }
      columns={columns}
      rows={visibleRows}
      rowKey={(row) => row.projectionVersion}
      viewState={viewStateOf(answer, rows.length, retry)}
    />
  );
}
