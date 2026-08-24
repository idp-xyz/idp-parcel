import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['node-operations-review'];

/**
 * 节点作业的治理查阅面。四区栏目取 node-operations CONTEXT.md 原词：节点收寄、
 * 集运单元实例、实际测量、节点侧交接证据。实时现场作业（扫描/点验/装卸）属
 * 一线作业端（ADR-0021），本页只查阅已发生的作业事实，不下发任务、不录事实。
 *
 * 查询端点未建，数据区如实呈现「未配置」态，不发请求、不含合成数据。行形状
 * 用列 id 索引的字符串占位（各区事实的权威字段见 CONTEXT.md 对应词条），查询
 * 契约落地时按其重谈。
 */
type ReviewRow = Record<string, string>;

function col(
  id: string,
  header: string,
  options?: { mono?: boolean; align?: 'left' | 'center' | 'right'; className?: string },
): ListColumn<ReviewRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) =>
      options?.mono ? (
        <span className="font-mono text-[12px]">{row[id]}</span>
      ) : (
        row[id]
      ),
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
    id: 'node-intake',
    word: '节点收寄',
    // 节点收寄=客户或其授权交付方在节点直接交付作业实物,节点完成明确接收并取得
    // 控制(CONTEXT);到站扫描、卸载或发现实物本身都不等于节点收寄,接线时不得
    // 把这些事实混入本区。
    columns: [
      col('operationalItem', '作业实物', { mono: true }),
      col('node', '物流节点', { mono: true }),
      col('deliveringParty', '交付方'),
      col('receivedAt', '收寄时间', { mono: true }),
      col('control', '实物控制'),
    ],
  },
  {
    id: 'consolidation-units',
    word: '集运单元',
    // 集运单元实例贯穿一次使用周期,同一实体载具的下一次使用必须创建新实例
    // (CONTEXT);实例阶段词取其生命周期:开放装入/已封装/授权开封/重新封装/
    // 显式终局关闭——尚未收入共享词表,先以原词文本呈现。
    columns: [
      col('instanceId', '实例标识', { mono: true }),
      col('carrierAsset', '可复用载具', { mono: true }),
      col('stage', '实例阶段'),
      col('memberCount', '集运成员', { align: 'right', className: 'w-[88px]' }),
      col('sealRecord', '封签记录', { mono: true }),
      col('formedAt', '形成时间', { mono: true }),
    ],
  },
  {
    id: 'measurements',
    word: '实际测量',
    // 每次实际测量必须保留对象、测量项、结果、单位、发生时间、位置、来源和作业
    // 依据(CONTEXT),不可覆盖;当前有效实测按业务规则派生,本区两者都要呈现。
    columns: [
      col('operationalItem', '作业实物', { mono: true }),
      col('measuredItem', '测量项'),
      col('result', '结果', { align: 'right', className: 'w-[88px]', mono: true }),
      col('unit', '单位', { className: 'w-[64px]' }),
      col('occurredAt', '发生时间', { mono: true }),
      col('location', '作业位置', { mono: true }),
      col('source', '来源'),
      col('currentlyEffective', '当前有效实测', { className: 'w-[104px]' }),
    ],
  },
  {
    id: 'handover-evidence',
    word: '交接证据',
    // 节点侧交接证据=备货、点验、扫描、装载、卸载、交出或接收观察(CONTEXT);
    // 它支持 transport-fulfillment 形成权威运输交接结果但自身不等于该结果,
    // 「权威交接结果」列只放引用,不得在本区改写或代答结果。
    columns: [
      col('operationalItem', '作业实物', { mono: true }),
      col('evidenceKind', '证据类别'),
      col('handoverScope', '交接范围', { mono: true }),
      col('occurredAt', '发生时间', { mono: true }),
      col('authoritativeResultRef', '权威交接结果（引用）', { mono: true }),
    ],
  },
  {
    id: 'unidentified-items',
    word: '待识别实物',
    // 待识别实物与实物身份候选、冲突是本上下文拥有的对象(CONTEXT)。本区无任何
    // 「确认身份/选一个为准」动作——现场人员不得以覆盖、合并或「最后一次扫描
    // 为准」决定正式包裹身份;身份确认由 parcel-shipment 以版本化关联形成,
    // 本区末列只放该关联的引用。识别成功不删除原实物记录,行是永久作业记录。
    columns: [
      col('internalLabel', '内部作业标签', { mono: true }),
      col('foundLocation', '发现位置', { mono: true }),
      col('conditionObservation', '状况观察'),
      col('identityCandidates', '身份候选与冲突'),
      col('registeredAt', '登记时间', { mono: true }),
      col('formalIdentityRef', '正式包裹关联（引用）', { mono: true }),
    ],
  },
];

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

export function NodeOperationsReviewPage() {
  const [sectionId, setSectionId] = useState(sections[0].id);
  const [keyword, setKeyword] = useState('');
  const section = sections.find((candidate) => candidate.id === sectionId) ?? sections[0];

  return (
    <ListPageTemplate<ReviewRow>
      title={info.title}
      description={`${info.owner} · 实时现场作业属一线作业端（ADR-0021），本页为治理查阅面，只读已发生的作业事实。`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按作业实物或节点检索',
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
        `${row.operationalItem ?? row.instanceId ?? row.internalLabel}#${
          row.occurredAt ?? row.formedAt ?? row.registeredAt
        }`
      }
      viewState={{
        kind: 'unconfigured',
        title: '作业与履约模块尚未接线',
        description: '节点作业的查询契约待建；本页不发请求、不含合成数据。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '节点作业查阅的查询契约建成并经 ADR-0017 准入闸门放行后接线；一线作业端另行其道（ADR-0021），不经本页',
        },
      }}
    />
  );
}
