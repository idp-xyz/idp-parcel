import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import {
  StatusBadgeFor,
  domainStatusTones,
  type DomainStatus,
} from '../../domain/status';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['transport-fulfillment-review'];

/**
 * 运输履约的治理查阅面。四区栏目取 transport-fulfillment CONTEXT.md 原词：
 * 具体班次、容量池、权威运输交接结果、交付证明。本页只查阅履约判断与事实，
 * 不建班次、不订舱、不改交接结果。
 *
 * 查询端点未建，数据区如实呈现「未配置」态，不发请求、不含合成数据。行形状
 * 用列 id 索引的字符串占位（各区的权威字段见 CONTEXT.md 对应词条），查询契约
 * 落地时按其重谈。
 */
type ReviewRow = Record<string, string>;

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
      if (options?.toneWord) return toneWordOrText(row[id]);
      if (options?.mono) return <span className="font-mono text-[12px]">{row[id]}</span>;
      return row[id];
    },
  };
}

interface ReviewSection {
  id: string;
  /** CONTEXT.md 原词。 */
  word: string;
  columns: ListColumn<ReviewRow>[];
}

const sections: ReviewSection[] = [
  {
    id: 'transport-legs',
    word: '班次',
    // 每个明确日期、时间范围和运行方向的运输机会具有独立、稳定的班次身份
    // (CONTEXT)。执行准备判断(开放/关闭订舱、暂停、延误)与实际事实(出发、
    // 移动、到达、中断…)不共用一个可覆盖状态,故各占一列。
    columns: [
      col('serviceRunId', '班次标识', { mono: true }),
      col('date', '日期', { mono: true, className: 'w-[104px]' }),
      col('timeRange', '时间范围', { mono: true }),
      col('direction', '运行方向'),
      col('preparation', '执行准备'),
      col('actualExecution', '实际执行'),
    ],
  },
  {
    id: 'capacity-pools',
    word: '容量池',
    // 容量池只启用适用维度(重量/体积/件数/托盘位/袋位/舱位),不强制换算成一个
    // 数值(CONTEXT);有效容量、已预占、已释放、实际使用分别维护,各占一列。
    columns: [
      col('poolId', '容量池', { mono: true }),
      col('scopeAndPeriod', '适用范围与期间'),
      col('dimension', '容量维度'),
      col('effectiveCapacity', '有效容量', { align: 'right', mono: true }),
      col('reserved', '已预占', { align: 'right', mono: true }),
      col('released', '已释放', { align: 'right', mono: true }),
      col('actuallyUsed', '实际使用', { align: 'right', mono: true }),
    ],
  },
  {
    id: 'handover-results',
    word: '权威交接结果',
    // 权威运输交接结果由本上下文唯一形成,逐载运对象判断并允许部分成立
    // (CONTEXT);结果词(已交接/已拒收/待确认)在共享词表内,按词表着色。
    // 整批、整车结论只能由对象级结果派生,本区行粒度就是载运对象。
    columns: [
      col('carriedObject', '载运对象', { mono: true }),
      col('boundary', '交接边界'),
      col('result', '结果', { toneWord: true, className: 'w-[104px]' }),
      col('businessTime', '业务时间', { mono: true }),
      col('judgmentVersion', '判断版本', { mono: true }),
    ],
  },
  {
    id: 'delivery-proofs',
    word: '交付证明',
    // 有效交付结果必须关联载运对象、履约尝试、业务时间、地点、交付方式、接收
    // 对象和符合当时规则的交付证明(CONTEXT);「已签收」不是可直接修改的状态,
    // 本区呈现证据构成,更正走新判断版本。
    columns: [
      col('carriedObject', '载运对象', { mono: true }),
      col('attempt', '履约尝试', { mono: true }),
      col('deliveryMethod', '交付方式'),
      col('recipient', '接收对象'),
      col('businessTime', '业务时间', { mono: true }),
      col('proofComposition', '证据构成'),
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
  },
];

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

export function TransportFulfillmentReviewPage() {
  const [sectionId, setSectionId] = useState(sections[0].id);
  const [keyword, setKeyword] = useState('');
  const section = sections.find((candidate) => candidate.id === sectionId) ?? sections[0];

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
      columns={section.columns}
      rows={[]}
      rowKey={(row) =>
        `${row.carriedObject ?? row.serviceRunId ?? row.poolId}#${row.businessTime ?? row.date ?? ''}`
      }
      viewState={{
        kind: 'unconfigured',
        title: '运输履约查询端点尚未建立',
        description: `查询契约待建；本页不发请求、不含合成数据。场景出处：${info.source}`,
      }}
    />
  );
}
