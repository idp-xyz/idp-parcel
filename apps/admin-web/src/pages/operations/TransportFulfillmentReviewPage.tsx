import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn, type TemplateViewState } from '../../templates';
import { moduleInfoById } from '../../navigation';
import {
  StatusBadgeFor,
  domainStatusTones,
  type DomainStatus,
} from '../../domain/status';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listTransportFulfillmentRecords,
  type ApiResult,
  type TransportFulfillmentListResponseBody,
  type TransportFulfillmentRegistry,
} from './records-api';
import { handoverVerdictLabels, labelOf } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['transport-fulfillment-review'];

/**
 * 运输履约的治理查阅面。五区栏目取 transport-fulfillment CONTEXT.md 原词。本页只
 * 查阅履约判断与事实，不建班次、不订舱、不改交接结果、不更正交付。
 *
 * 四区接真（GET /transport-fulfillment-records，票 admin-skeleton-closure-batch/05）：
 * 班次、容量池、权威交接结果、交付证明。承运总单与运输舱单一区在存储上还没有
 * 登记册——服务端的册名封闭集刻意不含那一格（没有表就没有读法，票 05 Comments），
 * 该区不发请求、如实呈现「无登记册」，不把「无处可登」演成「登记册为空」。
 */
interface ReviewRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

// 词表词（已交接/已拒收/待确认）按共享词表着色；词表没收录的词原样示文,
// 不猜色调。断言只桥接两个模块的类型边界,词本身同源于 CONTEXT 原词。
function toneWordOrText(value: string) {
  return value in domainStatusTones ? (
    <StatusBadgeFor status={value as DomainStatus} />
  ) : (
    value
  );
}

function col(
  id: string,
  header: string,
  options?: {
    mono?: boolean;
    align?: 'left' | 'center' | 'right';
    className?: string;
    toneWord?: boolean;
  },
): ListColumn<ReviewRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) => {
      const value = row.values[id] ?? '—';
      if (options?.toneWord) return toneWordOrText(value);
      if (options?.mono) return <span className="font-mono text-[12px]">{value}</span>;
      return value;
    },
  };
}

type SectionId =
  | 'transport-legs'
  | 'capacity-pools'
  | 'handover-results'
  | 'delivery-proofs'
  | 'transport-documents';

interface ReviewSection {
  id: SectionId;
  /** CONTEXT.md 原词。 */
  word: string;
  /** 对应册名；无册区缺席（该区在存储上没有登记册，不发请求）。 */
  registry?: TransportFulfillmentRegistry;
  columns: ListColumn<ReviewRow>[];
  /** 无册区的如实呈现；与「登记册为空」严格分词。 */
  absence?: { title: string; description: string; unlock: string };
}

const sections: ReviewSection[] = [
  {
    id: 'transport-legs',
    word: '班次',
    registry: 'transport-schedule',
    // 每个明确日期、时间范围和运行方向的运输机会具有独立、稳定的班次身份
    // (CONTEXT)。执行准备判断(开放/关闭订舱、暂停、延误)与实际事实(出发、移动、
    // 到达…)在班次行上没有登记格——缺席说的是「无处可登」,本区不代填「未出发」
    // 一类派生状态列(票 05 Comments)。
    columns: [
      col('scheduleId', '班次标识', { mono: true }),
      col('direction', '运行方向', { mono: true }),
      col('departsAt', '出发时刻', { mono: true }),
    ],
  },
  {
    id: 'capacity-pools',
    word: '容量池',
    registry: 'capacity-pool',
    // 容量池只启用适用维度,不强制换算成一个数值(CONTEXT);有效容量、已预占、
    // 已释放、实际使用分别维护,各占一列,页面不互相抵扣算「可用量」——那是领域
    // 按时点算的判断。
    columns: [
      col('poolId', '容量池', { mono: true }),
      col('schedule', '关联班次', { mono: true }),
      col('unit', '容量单位', { mono: true, className: 'w-[104px]' }),
      col('capacity', '有效容量', { align: 'right', mono: true }),
      col('reserved', '已预占', { align: 'right', mono: true }),
      col('released', '已释放', { align: 'right', mono: true }),
      col('consumed', '实际使用', { align: 'right', mono: true }),
    ],
  },
  {
    id: 'handover-results',
    word: '权威交接结果',
    registry: 'transport-handover',
    // 权威运输交接结果由本上下文唯一形成,逐载运对象判断并允许部分成立(CONTEXT);
    // 结果词(已交接/已拒收/待确认)在共享词表内,按词表着色。一行一判断版本,更正
    // 是新行指回前版(更正指回列),原判断在册面上继续可见;整批、整车结论只能由
    // 对象级结果派生,本区行粒度就是载运对象。
    columns: [
      col('object', '载运对象', { mono: true }),
      col('boundary', '交接边界', { mono: true }),
      col('verdict', '结果', { toneWord: true, className: 'w-[104px]' }),
      col('basis', '依据', { mono: true }),
      col('judgedAt', '判断时间', { mono: true }),
      col('version', '判断版本', { mono: true }),
      col('corrects', '更正指回', { mono: true }),
    ],
  },
  {
    id: 'delivery-proofs',
    word: '交付证明',
    registry: 'effective-delivery',
    // 有效交付结果必须关联载运对象、履约尝试、业务时间、地点、交付方式、接收对象
    // 和符合当时规则的交付证明(CONTEXT);「已签收」不是可直接修改的状态,证明列
    // 是引用不是证据内容,更正走新判断版本(更正指回列),本册只列当前版。
    columns: [
      col('object', '载运对象', { mono: true }),
      col('attempt', '履约尝试', { mono: true }),
      col('method', '交付方式', { mono: true }),
      col('recipient', '接收对象', { mono: true }),
      col('place', '交付地点', { mono: true }),
      col('occurredAt', '业务时间', { mono: true }),
      col('proof', '交付证明（引用）', { mono: true }),
      col('corrects', '更正指回', { mono: true }),
    ],
  },
  {
    id: 'transport-documents',
    word: '承运总单与运输舱单',
    // 本上下文拥有承运总单、运输舱单及外部承运凭证的身份和版本(CONTEXT-MAP)。
    // 与监管舱单分界:首发出口/进口监管舱单由承运商在外部形成并提交,关务经
    // UC-CC-012 只接受引用——监管舱单不在本区,本区是运输侧单证。
    columns: [
      col('documentKind', '单证类别'),
      col('documentId', '单证标识', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('relatedScope', '关联班次 / 实际履约段', { mono: true }),
      col('businessTime', '业务时间', { mono: true }),
    ],
    absence: {
      title: '承运总单与运输舱单在存储上还没有登记册',
      description:
        'transport-fulfillment 目前没有运输侧单证登记表，查询端点的册名封闭集刻意不含本区——没有表就没有读法，答一份恒空的册子会把「无处可登」演成「登记册为空」（票 admin-skeleton-closure-batch/05 Comments）。',
      unlock: '运输侧单证登记表落地后随查询契约扩册接线；本区不发请求、不含合成数据。',
    },
  },
];

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

function rowsOf(body: TransportFulfillmentListResponseBody): ReviewRow[] {
  switch (body.outcome) {
    case 'TRANSPORT_SCHEDULES_LISTED':
      return body.schedules.map((record) => ({
        key: `schedule:${record.scheduleId}`,
        values: {
          scheduleId: record.scheduleId,
          direction: record.direction,
          departsAt: formatInstant(record.departsAt),
        },
      }));
    case 'CAPACITY_POOLS_LISTED':
      return body.pools.map((record) => ({
        key: `pool:${record.poolId}`,
        values: {
          poolId: record.poolId,
          schedule: record.schedule,
          unit: record.unit,
          capacity: record.capacity,
          reserved: record.reserved,
          released: record.released,
          consumed: record.consumed,
        },
      }));
    case 'TRANSPORT_HANDOVERS_LISTED':
      return body.handovers.map((record) => ({
        key: `handover:${record.object}:${record.version}`,
        values: {
          object: record.object,
          // 交接边界由交出方与接收方两个参与方引用表达,组合成「A → B」是呈现的事。
          boundary: `${record.releasedBy} → ${record.receivedBy}`,
          verdict: labelOf(handoverVerdictLabels, record.verdict),
          basis: record.basis ?? '',
          judgedAt: formatInstant(record.judgedAt),
          version: record.version,
          corrects: record.correctsVersion
            ? `${record.correctsVersion}${record.correctedAt ? ` · ${formatInstant(record.correctedAt)}` : ''}`
            : '',
        },
      }));
    case 'EFFECTIVE_DELIVERIES_LISTED':
      return body.deliveries.map((record) => ({
        key: `delivery:${record.object}:${record.attempt}:${record.version}`,
        values: {
          object: record.object,
          attempt: record.attempt,
          method: record.method,
          recipient: record.recipient,
          place: record.place,
          occurredAt: formatInstant(record.occurredAt),
          proof: record.proof,
          corrects: record.correctsVersion
            ? `${record.correctsVersion}${record.correctedAt ? ` · ${formatInstant(record.correctedAt)}` : ''}`
            : '',
        },
      }));
  }
}

export function TransportFulfillmentReviewPage() {
  const [sectionId, setSectionId] = useState<SectionId>(sections[0].id);
  const [keyword, setKeyword] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    registry: TransportFulfillmentRegistry;
    answer: ApiResult<TransportFulfillmentListResponseBody>;
  } | null>(null);
  const section = sections.find((candidate) => candidate.id === sectionId) ?? sections[0];
  const registry = section.registry;

  useEffect(() => {
    // 无册区不发请求：服务端册名封闭集没有那一格，发了只会收 400。
    if (!registry) return undefined;
    let cancelled = false;
    void listTransportFulfillmentRecords(registry).then((answer) => {
      if (!cancelled) setLoaded({ registry, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [registry, reloadKey]);

  const answer = registry && loaded?.registry === registry ? loaded.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? rowsOf(body) : [];
  const needle = keyword.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  const viewState: TemplateViewState = section.absence
    ? {
        kind: 'unconfigured',
        title: section.absence.title,
        description: section.absence.description,
        facts: { owner: info.owner, source: info.source, unlock: section.absence.unlock },
      }
    : catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /transport-fulfillment-records?registry=${registry ?? ''}`,
        emptyTitle: `当前租户尚无${section.word}登记`,
        emptyDescription:
          '读取入口已配置，登记册为空——履约判断与事实的写入方在接入渠道墙后面，空册是预期不是缺陷；页面不含合成数据。',
      });

  return (
    <ListPageTemplate<ReviewRow>
      title={info.title}
      description={`${info.owner} · 治理查阅面，只读履约判断与事实；实时现场作业属一线作业端（ADR-0021）。`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按载运对象或班次标识检索',
      }}
      filters={
        <>
          {sections.map((candidate) => (
            <button
              key={candidate.id}
              type="button"
              className={chipClass(candidate.id === sectionId)}
              onClick={() => setSectionId(candidate.id)}
            >
              {candidate.word}
            </button>
          ))}
        </>
      }
      filterSummary={
        // 计数只在拿到业务答案后显示,未配置/无册态不报「0 行」。
        body ? `${section.word} ${visibleRows.length} 行` : undefined
      }
      columns={section.columns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={viewState}
    />
  );
}
