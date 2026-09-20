import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn, type TemplateViewState } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listNodeOperationsRecords,
  type ApiResult,
  type NodeOperationsListResponseBody,
  type NodeOperationsRegistry,
} from './records-api';
import {
  consolidationPhaseLabels,
  controlKindLabels,
  labelOf,
  receptionKindLabels,
} from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['node-operations-review'];

/**
 * 节点作业的治理查阅面。五区栏目取 node-operations CONTEXT.md 原词。实时现场作业
 * （扫描/点验/装卸）属一线作业端（ADR-0021），本页只查阅已发生的作业事实，不下发
 * 任务、不录事实、不确认身份。
 *
 * 三区接真（GET /node-operations-records，票 admin-skeleton-closure-batch/05）：
 * 节点收寄、集运单元、待识别实物。实际测量与节点侧交接证据两区在存储上还没有
 * 登记册——服务端的册名封闭集刻意不含那两格（没有表就没有读法，票 05 Comments），
 * 两区不发请求、如实呈现「无登记册」，不显示会被 400 拒掉的查询结果，也不把
 * 「无处可登」演成「登记册为空」。
 */
interface ReviewRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

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
        <span className="font-mono text-[12px]">{row.values[id] ?? '—'}</span>
      ) : (
        (row.values[id] ?? '—')
      ),
  };
}

type SectionId =
  | 'node-intake'
  | 'consolidation-units'
  | 'measurements'
  | 'handover-evidence'
  | 'unidentified-items';

interface ReviewSection {
  id: SectionId;
  /** CONTEXT.md 原词。 */
  word: string;
  /** 对应册名；无册区缺席（该区在存储上没有登记册，不发请求）。 */
  registry?: NodeOperationsRegistry;
  columns: ListColumn<ReviewRow>[];
  /** 无册区的如实呈现；与「登记册为空」严格分词。 */
  absence?: { title: string; description: string; unlock: string };
}

const sections: ReviewSection[] = [
  {
    id: 'node-intake',
    word: '节点收寄',
    registry: 'reception',
    // 节点收寄=客户或其授权交付方在节点直接交付作业实物,节点完成明确接收并取得
    // 控制(CONTEXT);到站扫描、卸载或发现实物本身都不等于节点收寄——本册由服务端
    // 只列带收寄判断与控制在场的登记行,这些事实混不进来。
    columns: [
      col('unit', '作业实物', { mono: true }),
      col('kind', '收寄判断', { className: 'w-[104px]' }),
      col('node', '物流节点', { mono: true }),
      col('deliveredBy', '交付方', { mono: true }),
      col('receivedAt', '收寄时间', { mono: true }),
      col('control', '实物控制'),
      col('serviceMarkers', '服务标记'),
    ],
  },
  {
    id: 'consolidation-units',
    word: '集运单元',
    registry: 'consolidation-unit',
    // 集运单元实例贯穿一次使用周期,同一实体载具的下一次使用必须创建新实例
    // (CONTEXT);实例阶段封闭三相(开放装入/已封装/已关闭)。集运成员列的是当前
    // 成员数不是清单;封签取最近一次快照,封装次数把「从未封装」与「重新封装过」
    // 分开。没有形成时间列——单元行上只有库面簿记时刻,不是业务事实。
    //
    // 开启作业与最近封装各列一格「来源 · 执行方」(UC-NO-003 结果契约要保存的那两
    // 件)。两格都把来源身份摆在执行方前面:一线过渡期由内勤汇总导入的封签事实与
    // 现场设备扫描的封签事实,执行方可以是同一个人,分得开两者的只有来源身份。
    columns: [
      col('unitId', '实例标识', { mono: true }),
      col('asset', '可复用载具', { mono: true }),
      col('phase', '实例阶段', { className: 'w-[96px]' }),
      col('memberCount', '集运成员', { align: 'right', className: 'w-[88px]', mono: true }),
      col('sealCount', '封装次数', { align: 'right', className: 'w-[88px]', mono: true }),
      col('openedSource', '开启来源', { mono: true }),
      col('seal', '最近封签', { mono: true }),
      col('sealSource', '封装来源', { mono: true }),
      col('closedAt', '关闭时间', { mono: true }),
    ],
  },
  {
    id: 'measurements',
    word: '实际测量',
    // 每次实际测量必须保留对象、测量项、结果、单位、发生时间、位置、来源和作业
    // 依据(CONTEXT),不可覆盖。存储上还没有实测登记表,本区无处可读——列向留在
    // 这里,登记表落地随查询契约扩册时按 CONTEXT 词条重谈。
    columns: [
      col('operationalItem', '作业实物', { mono: true }),
      col('measuredItem', '测量项'),
      col('result', '结果', { align: 'right', className: 'w-[88px]', mono: true }),
      col('unit', '单位', { className: 'w-[64px]' }),
      col('occurredAt', '发生时间', { mono: true }),
    ],
    absence: {
      title: '实际测量在存储上还没有登记册',
      description:
        'node-operations 目前没有实测登记表，查询端点的册名封闭集刻意不含本区——没有表就没有读法，答一份恒空的册子会把「无处可登」演成「登记册为空」（票 admin-skeleton-closure-batch/05 Comments）。',
      unlock: '实测登记表落地后随查询契约扩册接线；本区不发请求、不含合成数据。',
    },
  },
  {
    id: 'handover-evidence',
    word: '交接证据',
    // 节点侧交接证据=备货、点验、扫描、装载、卸载、交出或接收观察(CONTEXT);它支持
    // transport-fulfillment 形成权威运输交接结果但自身不等于该结果。存储上现有的
    // execution_fact 与 collaboration_acceptance 属关务协同执行,不是本区的节点交接
    // 证据,不得混入(票 05 Comments)。
    columns: [
      col('operationalItem', '作业实物', { mono: true }),
      col('evidenceKind', '证据类别'),
      col('handoverScope', '交接范围', { mono: true }),
      col('occurredAt', '发生时间', { mono: true }),
      col('authoritativeResultRef', '权威交接结果（引用）', { mono: true }),
    ],
    absence: {
      title: '节点侧交接证据在存储上还没有登记册',
      description:
        '现有登记表里与「交接」相近的是关务协同执行事实，不是本区的节点侧交接证据——混入会把关务协同演成通用交接观察（票 admin-skeleton-closure-batch/05 Comments）。没有表就没有读法。',
      unlock: '节点侧交接证据登记表落地后随查询契约扩册接线；本区不发请求、不含合成数据。',
    },
  },
  {
    id: 'unidentified-items',
    word: '待识别实物',
    registry: 'unidentified-item',
    // 待识别实物与实物身份候选、冲突是本上下文拥有的对象(CONTEXT)。本区无任何
    // 「确认身份/选一个为准」动作——身份确认由 parcel-shipment 以版本化关联形成,
    // 末列只放登记时已有的关联引用;识别成功不回写本行(行是永久作业记录),缺席
    // 说的是「登记那一刻还没有」。
    columns: [
      col('unit', '内部作业标签', { mono: true }),
      col('node', '登记节点', { mono: true }),
      col('candidates', '身份候选'),
      col('identityConflict', '身份冲突', { className: 'w-[88px]' }),
      col('receivedAt', '收到时间', { mono: true }),
      col('association', '正式包裹关联（引用）', { mono: true }),
    ],
  },
];

function rowsOf(body: NodeOperationsListResponseBody): ReviewRow[] {
  switch (body.outcome) {
    case 'RECEPTIONS_LISTED':
      return body.receptions.map((record) => ({
        key: `reception:${record.sourceId}`,
        values: {
          unit: record.unit,
          kind: labelOf(receptionKindLabels, record.kind),
          node: record.node,
          deliveredBy: record.deliveredBy,
          receivedAt: formatInstant(record.receivedAt),
          // released 两键成对缺席即实物仍在节点控制中（传输层在场规则）；转出行
          // 照登记示引用与时刻，不代判「在库/出库」。
          control: record.controlReleasedBy
            ? `${labelOf(controlKindLabels, record.controlKind)} · 已转出 ${record.controlReleasedBy}${
                record.controlReleasedAt ? ` · ${formatInstant(record.controlReleasedAt)}` : ''
              }`
            : `${labelOf(controlKindLabels, record.controlKind)} · 控制中`,
          serviceMarkers: record.serviceMarkers.join('、'),
        },
      }));
    case 'UNIDENTIFIED_ITEMS_LISTED':
      return body.items.map((record) => ({
        key: `unidentified:${record.sourceId}`,
        values: {
          unit: record.unit,
          node: record.node,
          candidates: record.candidates.join('、'),
          identityConflict: record.identityConflict ? '有冲突' : '无冲突',
          receivedAt: formatInstant(record.receivedAt),
          association: record.association ?? '',
        },
      }));
    case 'CONSOLIDATION_UNITS_LISTED':
      return body.units.map((record) => ({
        key: `unit:${record.unitId}`,
        values: {
          unitId: record.unitId,
          asset: record.asset,
          phase: labelOf(consolidationPhaseLabels, record.phase),
          memberCount: String(record.memberCount),
          sealCount: String(record.sealCount),
          openedSource: `${record.openedSourceId} · ${record.openedBy}`,
          seal: record.latestSeal
            ? `${record.latestSeal}${record.latestSealedAt ? ` · ${formatInstant(record.latestSealedAt)}` : ''}`
            : '',
          sealSource: record.latestSealSourceId
            ? `${record.latestSealSourceId} · ${record.latestSealPerformedBy ?? ''}`
            : '',
          closedAt: record.closedAt ? formatInstant(record.closedAt) : '',
        },
      }));
  }
}

export function NodeOperationsReviewPage() {
  const [sectionId, setSectionId] = useState<SectionId>(sections[0].id);
  const [keyword, setKeyword] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    registry: NodeOperationsRegistry;
    answer: ApiResult<NodeOperationsListResponseBody>;
  } | null>(null);
  const section = sections.find((candidate) => candidate.id === sectionId) ?? sections[0];
  const registry = section.registry;

  useEffect(() => {
    // 无册区不发请求：服务端册名封闭集没有那两格，发了只会收 400。
    if (!registry) return undefined;
    let cancelled = false;
    void listNodeOperationsRecords(registry).then((answer) => {
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
        endpoint: `GET /node-operations-records?registry=${registry ?? ''}`,
        emptyTitle: `当前租户尚无${section.word}登记`,
        emptyDescription:
          '读取入口已配置，登记册为空——作业事实的写入方在接入渠道墙后面，空册是预期不是缺陷；页面不含合成数据。',
      });

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
