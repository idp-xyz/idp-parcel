import { useState, type ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import type { TemplateViewState } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { RouteWorkbench, SectionTabs, notConnectedOverride } from '../network/route-workbench';
import { usePlaceNames } from '../network/use-place-names';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { PageOnlyNote, PageOnlySearch } from '../visibility/page-only';
import { ReviewDetail, reviewStatusOf, withPlaceNames, workbenchColumnsOf, type ReviewColumn, type ReviewStatusSpec } from './review-workbench';
import { useMayMakeDecision } from '../use-operator-grants';
import { hiddenDecisionHint, pendingIdentityRecheckDecision } from '../operator-grants';
import { labelFor } from '../../vocabulary/status';
import { OperatorRefusalNote } from '../../components/states';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { catalogueConditionsNarrow, type CatalogueConditions } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import {
  listIdentityReviewUnits,
  listNodeOperationsRecords,
  nodeOperationsConditions,
  type ApiResult,
  type NodeOperationsConditions,
  type NodeOperationsListResponseBody,
  type NodeOperationsRegistry,
} from './records-api';
import {
  allStatuses,
  nodePushedStatus,
  pageStatusChips,
  pushedStatusChips,
  reviewListConditions,
  rowsOnPage,
} from './list-conditions';
import { consolidationPhaseLabels, labelOf } from './presentation';
import { receptionRowOf } from './reception-rows';
import { recheckPendingIdentity, type IdentityRecheckResponseBody } from './identity-recheck-api';
import {
  describeRecheckAnswer,
  recheckProblemNote,
  unidentifiedRowOf,
  type RecheckAvailability,
} from './identity-recheck';
import { IdentityDispositionPanel } from './IdentityDispositionPanel';
import { DeliveryPreAdvicesSection } from './DeliveryPreAdvicesSection';
import { DeliveryHandoverNotesSection } from './DeliveryHandoverNotesSection';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['node-operations-review'];

/**
 * 节点作业的治理查阅面。五区栏目取 node-operations CONTEXT.md 原词。实时现场作业
 * （扫描/点验/装卸）属一线作业端（ADR-0021），本页只查阅已发生的作业事实，不下发
 * 任务、不录事实、不确认身份。写动作只在待识别实物区，两样都是运营决定：「重核」（POST
 * /node-operations/identity-rechecks，ADR-0199 决定三）叫服务端按存下的号再问 parcel-shipment 一次，身份照旧由那边答；
 * 「处置」（POST /node-operations/identity-dispositions，UC-NO-004）在一件判身份冲突的实物的处置候选里指定一件或确认都不是，
 * 候选都出自 parcel-shipment 的身份核对答复，不改正式包裹身份。
 *
 * 三区接真（GET /node-operations-records，票 admin-skeleton-closure-batch/05）：
 * 节点收寄、集运单元、待识别实物。实际测量与节点侧交接证据两区在存储上还没有
 * 登记册——服务端的册名封闭集刻意不含那两格（没有表就没有读法，票 05 Comments），
 * 两区不发请求、如实呈现「无登记册」，不显示会被 400 拒掉的查询结果，也不把
 * 「无处可登」演成「登记册为空」。
 *
 * 送站预报与送站交接单两区各走自己的读口（GET /node-operations-delivery-pre-advices、/node-operations-delivery-handover-notes，
 * 票 ps-external-mark-relations/13），筛选与详情各在自己的组件里；交接单详情上的「确认」是又一样运营决定（送站交接确认，UC-NO-006
 * 步骤 5B），依授权交付关系确认一个账户的绑定，不确认身份。
 *
 * 各区接读口分页（ADR-0144，票 catalogue-read-pagination-rollout/14）：检索框下推成 `q`，收货记录与集包的状态胶囊下推成筛选维；
 * 「为什么认不出」由页面按几格派生、不是筛选维，胶囊只筛本页。读口检索只认标识与编码，网点名称、中文词这些在「只筛本页」的框里
 * 再筛。按实物列的处置面板是另一本册（registry=identity-review-unit），自己翻页。
 */
interface ReviewRow {
  key: string;
  values: Readonly<Record<string, string>>;
  /** 只在待识别实物区的行上在场：「重核」动作能不能发起。 */
  recheck?: RecheckAvailability;
}

function col(
  id: string,
  header: string,
  options?: { mono?: boolean; align?: 'left' | 'center' | 'right'; className?: string },
): ReviewColumn<ReviewRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) =>
      options?.mono ? (
        <span className="font-mono type-body">{row.values[id] ?? '—'}</span>
      ) : (
        (row.values[id] ?? '—')
      ),
  };
}

/** 状态胶囊格：词取行上已译好的那一格，色调按码（codeKey 那一格）查词典。 */
function pillCol(id: string, header: string, codeKey: string, scope?: string): ReviewColumn<ReviewRow> {
  return {
    id,
    header,
    align: 'center',
    className: 'w-[112px]',
    render: (row) => {
      const word = row.values[id];
      if (!word) return '—';
      const code = row.values[codeKey];
      return code ? <StatusPill code={code} scope={scope} label={word} /> : <StatusPill label={word} />;
    },
  };
}

/** 每行末尾的「技术详情」：来源标识、判断版本、关联引用这类排查时才用的字，空的不占格。 */
function techCol(pairs: [label: string, key: string][]): ReviewColumn<ReviewRow> {
  // 不上表：工作台把这一格挪进右栏的「技术详情」（review-workbench 的 ReviewDetail）。
  return { id: 'technical', header: '技术详情', render: () => null, techPairs: pairs };
}

type SectionId =
  | 'node-intake'
  | 'consolidation-units'
  | 'measurements'
  | 'handover-evidence'
  | 'unidentified-items'
  | DeliverySectionId;

/** 不走 /node-operations-records 的两区：各有自己的读口、筛选与详情，区内容交给各自的组件。 */
type DeliverySectionId = 'delivery-pre-advices' | 'delivery-handover-notes';

const deliverySections: { id: DeliverySectionId; word: string }[] = [
  { id: 'delivery-pre-advices', word: '客户送货预报' },
  { id: 'delivery-handover-notes', word: '送货交接单' },
];

interface ReviewSection {
  id: SectionId;
  /** CONTEXT.md 原词。 */
  word: string;
  /** 对应册名；无册区缺席（该区在存储上没有登记册，不发请求）。 */
  registry?: NodeOperationsRegistry;
  /** 检索框的提示：只写读口 `q` 覆盖的那几列（internal/nodeoperations/adapters/postgres 各册的 Keyword）。 */
  keywordHint?: string;
  columns: ReviewColumn<ReviewRow>[];
  /** 无册区的如实呈现；与「登记册为空」严格分词。 */
  absence?: { title: string; description: string; unlock: string };
  /** 行状态胶囊取哪两格；给了即指标即筛。 */
  status?: ReviewStatusSpec;
}

const sections: ReviewSection[] = [
  {
    id: 'node-intake',
    word: '收货记录',
    registry: 'reception',
    keywordHint: '搜货物标签、网点编码、交货人、作业来源',
    status: { labelKey: 'kind', codeKey: 'kindCode' },
    // 节点收寄=客户或其授权交付方在节点直接交付作业实物,节点完成明确接收并取得
    // 控制(CONTEXT);到站扫描、卸载或发现实物本身都不等于节点收寄——本册由服务端
    // 只列带收寄判断与控制在场的登记行,这些事实混不进来。
    columns: [
      col('unit', '货物标签', { mono: true }),
      pillCol('kind', '收寄', 'kindCode'),
      col('node', '网点'),
      col('deliveredBy', '交货人', { mono: true }),
      col('receivedAt', '收货时间', { mono: true }),
      col('control', '现在'),
      col('serviceMarkers', '取消或终局'),
      techCol([
        ['网点编码', 'nodeCode'],
        ['作业来源', 'sourceId'],
        ['收寄结论码', 'kindCode'],
        ['控制来源码', 'controlKind'],
        ['转出给', 'releasedTo'],
        ['登记时间', 'recordedAt'],
      ]),
    ],
  },
  {
    id: 'consolidation-units',
    word: '集包',
    registry: 'consolidation-unit',
    keywordHint: '搜袋 / 箱、封签、集包标识',
    status: { labelKey: 'phase', codeKey: 'phaseCode', scope: 'consolidation' },
    // 集运单元实例贯穿一次使用周期,同一实体载具的下一次使用必须创建新实例
    // (CONTEXT);实例阶段封闭三相(开放装入/已封装/已关闭)。集运成员列的是当前
    // 成员数不是清单;封签取最近一次快照,封装次数把「从未封装」与「重新封装过」
    // 分开。没有形成时间列——单元行上只有库面簿记时刻,不是业务事实。
    //
    // 开启作业与最近封装各列一格「来源 · 执行方」(UC-NO-003 结果契约要保存的那两
    // 件)。两格都把来源身份摆在执行方前面:一线过渡期由内勤汇总导入的封签事实与
    // 现场设备扫描的封签事实,执行方可以是同一个人,分得开两者的只有来源身份。
    columns: [
      col('asset', '袋 / 箱', { mono: true }),
      pillCol('phase', '状态', 'phaseCode', 'consolidation'),
      col('memberCount', '件数', { align: 'right', className: 'w-[72px]', mono: true }),
      col('seal', '封签', { mono: true }),
      col('sealCount', '封了几次', { align: 'right', className: 'w-[88px]', mono: true }),
      col('closedAt', '关闭时间', { mono: true }),
      techCol([
        ['集包标识', 'unitId'],
        ['开启来源 · 人', 'openedSource'],
        ['封装来源 · 人', 'sealSource'],
        ['状态码', 'phaseCode'],
      ]),
    ],
  },
  {
    id: 'measurements',
    word: '称重量方',
    // 每次实际测量必须保留对象、测量项、结果、单位、发生时间、位置、来源和作业
    // 依据(CONTEXT),不可覆盖。存储上还没有实测登记表,本区无处可读——列向留在
    // 这里,登记表落地随查询契约扩册时按 CONTEXT 词条重谈。
    columns: [
      col('operationalItem', '货物标签', { mono: true }),
      col('measuredItem', '测量项'),
      col('result', '结果', { align: 'right', className: 'w-[88px]', mono: true }),
      col('unit', '单位', { className: 'w-[64px]' }),
      col('occurredAt', '发生时间', { mono: true }),
    ],
    // 存储上没有实测登记表，查询端点的册名封闭集刻意不含本区（票 admin-skeleton-closure-batch/05 Comments）：没有表就没有读法，
    // 答一份恒空的册子会把「无处可登」演成「登记册为空」。
    absence: {
      title: '称重量方记录还不能在这里查看',
      description: '网点的称重、量方结果还没有接入管理台，这不是「没有记录」。这一签不显示任何示意数据。',
      unlock: '称重量方的记录接入后，这一签会自动显示',
    },
  },
  {
    id: 'handover-evidence',
    word: '交接记录',
    // 节点侧交接证据=备货、点验、扫描、装载、卸载、交出或接收观察(CONTEXT);它支持
    // transport-fulfillment 形成权威运输交接结果但自身不等于该结果。存储上现有的
    // execution_fact 与 collaboration_acceptance 属关务协同执行,不是本区的节点交接
    // 证据,不得混入(票 05 Comments)。
    columns: [
      col('operationalItem', '货物标签', { mono: true }),
      col('evidenceKind', '记录类别'),
      col('handoverScope', '交接批次', { mono: true }),
      col('occurredAt', '发生时间', { mono: true }),
    ],
    // 现有登记表里与「交接」相近的是关务协同执行事实，不是节点侧交接证据——混入会把关务协同演成通用交接观察（票
    // admin-skeleton-closure-batch/05 Comments）。没有表就没有读法。交接的结论在「运输班次」页的交接结果签。
    absence: {
      title: '网点的交接记录还不能在这里查看',
      description: '网点备货、点验、装卸时的扫描记录还没有接入管理台，这不是「没有记录」。交接成没成功，到「运输班次」页的「交接结果」查看。',
      unlock: '网点交接记录接入后，这一签会自动显示',
    },
  },
  {
    id: 'unidentified-items',
    word: '待识别货物',
    registry: 'unidentified-item',
    keywordHint: '搜货物标签、网点编码、扫到的号、作业来源',
    status: { labelKey: 'pendingCause' },
    // 待识别实物与实物身份候选、冲突是本上下文拥有的对象(CONTEXT)。正式包裹及其身份由
    // parcel-shipment 给出,「正式包裹关联」一列只放登记时已有的关联引用;识别成功不回写本行
    // (行是永久作业记录),缺席说的是「登记那一刻还没有」。
    //
    // 本区下方按实物列的那一块有「处置」(UC-NO-004):只在一件判身份冲突的实物的处置候选里
    // 指定一件或确认都不是——候选都出自 parcel-shipment 的身份核对答复,页面不收候选之外的
    // 包裹;它定的是实物与哪一件包裹关联(ADR-0210 越权风险点「正式身份由 parcel-shipment 确认」的读法),不创建或替换正式包裹身份。
    //
    // 「重核」也不确认身份:它只叫服务端拿来源里存下的号再问一次 parcel-shipment
    // (ADR-0199 决定三),结果唯一才追加一版关联收寄,原行照留。外部标识观察一列是重核
    // 拿去问的那一格,原样示出;判断版本一列是重核指名的那一版。记身份冲突的行只有来由是
    // 「只因歧义信号」的给「重核」(ADR-0211 决定二:声明同号的委托被拒绝或撤回后再问就是唯一命中),
    // 其余来由与来由未记的只给下方「处置」;冲突来由一列说的就是这一分别。
    //
    // 「为何待识别」把缺范围、范围成立但冲突、零候选与未核对分开写（ADR-0212 决定五）：四者候选列与冲突列可以一样，出路却各不相同。
    // 交付范围核对一列是这一版判断时记下的推导，附恢复动作；依据一列是来源里上报的依据与此刻最后归入的交接单。
    columns: [
      col('unit', '货物标签', { mono: true }),
      col('node', '网点'),
      col('pendingCause', '为什么认不出'),
      col('externalMark', '扫到的号（原样）', { mono: true }),
      col('candidates', '可能是哪件'),
      // 身份核对一列紧挨候选：未核对那一行候选是空的，这一列说它是没问到、不是查无此号（ADR-0199 决定五）。
      col('identityCheck', '核对过没有'),
      col('identityConflict', '身份冲突', { className: 'w-[88px]' }),
      col('identityConflictCause', '冲突原因'),
      col('deliveryScope', '送货范围核对'),
      col('receivedAt', '收到时间', { mono: true }),
      techCol([
        ['网点编码', 'nodeCode'],
        ['范围依据 · 此刻归入', 'deliveryScopeBasis'],
        ['已关联包裹', 'association'],
        ['判断版本', 'judgmentVersion'],
      ]),
    ],
  },
];

function rowsOf(body: NodeOperationsListResponseBody): ReviewRow[] {
  switch (body.outcome) {
    case 'RECEPTIONS_LISTED':
      return body.receptions.map(receptionRowOf);
    case 'UNIDENTIFIED_ITEMS_LISTED':
      return body.items.map(unidentifiedRowOf);
    case 'CONSOLIDATION_UNITS_LISTED':
      return body.units.map((record) => ({
        key: `unit:${record.unitId}`,
        values: {
          unitId: record.unitId,
          asset: record.asset,
          phase: labelOf(consolidationPhaseLabels, record.phase),
          phaseCode: record.phase,
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

type RecheckState =
  | { kind: 'submitting'; judgmentVersion: string }
  | { kind: 'answered'; judgmentVersion: string; answer: ApiResult<IdentityRecheckResponseBody> };

const subtitle =
  '网点收了哪些货、货现在还在不在网点、哪些货认不出是谁的。扫描与装卸在网点的作业终端上完成，这里查看结果，并处理认不出身份的货。';

// useCatalogueCursor 要稳定引用；按 registry 收窄的泛型读口在这里定成各册答复的并集，rowsOf 按 outcome 择形状。
const loadNodeRecords = (conditions: NodeOperationsConditions<NodeOperationsRegistry>, after: string | null) =>
  listNodeOperationsRecords(conditions, after);

// 「为什么认不出」由页面按核对态、冲突与候选几格派生（delivery-scope-check 的 pendingCauseOf），读口没有这一维：胶囊只筛本页。
const pendingCauseOfRow = (row: ReviewRow) => row.values.pendingCause;

export function NodeOperationsReviewPage() {
  const [sectionId, setSectionId] = useState<SectionId>(sections[0].id);
  const delivery = deliverySections.find((candidate) => candidate.id === sectionId);
  const section = sections.find((candidate) => candidate.id === sectionId) ?? sections[0];

  const tabs = (
    <SectionTabs
      sections={[...sections, ...deliverySections].map((candidate) => ({ id: candidate.id, label: candidate.word }))}
      active={sectionId}
      onChange={setSectionId}
    />
  );
  // 送站两区不读 /node-operations-records：落空的 section 回退到第一区，不能让它顺带发一次收寄册的请求。
  if (delivery?.id === 'delivery-pre-advices') return <DeliveryPreAdvicesSection key={delivery.id} sections={tabs} />;
  if (delivery?.id === 'delivery-handover-notes') return <DeliveryHandoverNotesSection key={delivery.id} sections={tabs} />;
  // 换区重挂：上一区的检索、胶囊、翻页位置与选中行不带进下一区。
  return section.registry ? (
    <RegistrySection key={section.id} section={section} registry={section.registry} tabs={tabs} />
  ) : (
    <AbsentSection key={section.id} section={section} tabs={tabs} />
  );
}

/** 有册的一区：按读口分页取数，检索与能下推的胶囊下推，「为什么认不出」与「只筛本页」的框在到手的这一页上筛。 */
function RegistrySection({
  section,
  registry,
  tabs,
}: {
  section: ReviewSection;
  registry: NodeOperationsRegistry;
  tabs: ReactNode;
}) {
  const pushed = nodePushedStatus[registry];
  const pageStatus = registry === 'unidentified-item' ? pendingCauseOfRow : undefined;
  const [keyword, setKeyword] = useState('');
  // 下推的胶囊存读口词表里的词，只筛本页的胶囊存页上的状态词；一区只有其中一种。
  const [status, setStatus] = useState(allStatuses);
  const [refine, setRefine] = useState('');
  const [recheck, setRecheck] = useState<RecheckState | null>(null);
  const mayRecheck = useMayMakeDecision(pendingIdentityRecheckDecision);
  // 处置面板是另一本册、自己翻页；作业落地后随上方列表一起重读，重挂即回到它的第一页。
  const [unitsKey, setUnitsKey] = useState(0);
  const conditions = nodeOperationsConditions(registry, reviewListConditions(useSettledKeyword(keyword), pushed, status));
  const { answer, pagination, retry } = useCatalogueCursor(conditions, loadNodeRecords);
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const places = usePlaceNames();
  // 网点一格按此刻生效那一版显名称，编码进技术详情（review-workbench 的 withPlaceNames）。
  const pageRows = body ? withPlaceNames(rowsOf(body), ['node'], places) : [];
  const rows = rowsOnPage(pageRows, refine, status, pageStatus);
  const refresh = () => {
    retry();
    setUnitsKey((value) => value + 1);
  };
  const startRecheck = (judgmentVersion: string) => {
    setRecheck({ kind: 'submitting', judgmentVersion });
    void recheckPendingIdentity({ judgmentVersion }).then((answer) => {
      setRecheck({ kind: 'answered', judgmentVersion, answer });
      // 追加了一版（关联收寄或冲突版）才重读：册上多了行、被重核那一行有了接续。
      if (answer.kind === 'outcome' && describeRecheckAnswer(answer.body).landed) refresh();
    });
  };
  const statusChips = pushed
    ? pushedStatusChips(pushed, status, setStatus)
    : pageStatus && body
      ? pageStatusChips(pageRows.map(pageStatus), status, setStatus)
      : undefined;
  const viewState = catalogueViewState(answer, pageRows.length, retry, {
    module: info,
    endpoint: `GET /node-operations-records?registry=${registry}`,
    emptyTitle: `还没有${section.word}`,
    emptyDescription: '网点现场扫描、收货后，记录会出现在这里。',
    narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
  });

  /**
   * 待识别货物的右栏动作：「再核对一次」（重核）。不能发起的行照说为什么，不给一个必然答「不重核」的按钮；明确没有「待识别复核」
   * 授予时不摆按钮（ADR-0315）。答复挂在按钮下面，只对被重核的那一行显示。
   */
  const recheckActions = (row: ReviewRow) => {
    const availability = row.recheck;
    if (!availability) return undefined;
    if (availability.kind === 'unavailable') return <p className="type-body text-idpxyz-breadcrumb">{availability.reason}</p>;
    if (!mayRecheck) return <p className="type-body text-idpxyz-breadcrumb">{hiddenDecisionHint('再核对', labelFor(pendingIdentityRecheckDecision))}</p>;
    const mine = recheck?.judgmentVersion === availability.judgmentVersion;
    return (
      <div className="space-y-2">
        <Button variant="outline" disabled={recheck?.kind === 'submitting'} onClick={() => startRecheck(availability.judgmentVersion)}>
          {recheck?.kind === 'submitting' && mine ? '核对中…' : '再核对一次'}
        </Button>
        {mine && recheck?.kind === 'answered' ? <RecheckAnswerNote judgmentVersion={recheck.judgmentVersion} answer={recheck.answer} /> : null}
      </div>
    );
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <RouteWorkbench<ReviewRow>
        title={info.title}
        subtitle={subtitle}
        sections={tabs}
        extraFilters={
          <>
            {pageStatus && body ? <PageOnlyNote>状态只筛本页</PageOnlyNote> : null}
            <PageOnlySearch value={refine} onChange={setRefine} subject="名称等其余各格" />
          </>
        }
        answered={body !== null}
        rows={rows}
        rowKey={(row) => row.key}
        statusChips={statusChips}
        onClearFilters={() => {
          setStatus(allStatuses);
          setRefine('');
        }}
        rowAttention={(row) => row.values.identityConflict === '有冲突'}
        searchCells={() => []}
        search={{ value: keyword, onChange: setKeyword, pushedDown: true }}
        searchPlaceholder={section.keywordHint ?? `搜${section.word}`}
        columns={workbenchColumnsOf(section.columns, section.status)}
        renderDetail={(row, close) => (
          <ReviewDetail
            row={row}
            columns={section.columns}
            status={reviewStatusOf(row, section.status)}
            subtitle={section.word}
            actions={section.id === 'unidentified-items' ? recheckActions(row) : undefined}
            onClose={close}
          />
        )}
        onRefresh={refresh}
        toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
        viewState={viewState}
      />
      {registry === 'unidentified-item' ? <IdentityReviewUnits key={unitsKey} onLanded={refresh} /> : null}
    </div>
  );
}

const noConditions: CatalogueConditions = {};
const loadIdentityReviewUnits = (_conditions: CatalogueConditions, after: string | null) => listIdentityReviewUnits(after);

/**
 * 身份冲突处置按实物看各链当前有效版本（一件实物可能横跨几行），不是按行，所以留在列表下方而不进右栏。它读按实物那一本册
 * （registry=identity-review-unit）：待识别册答复里的 units 只是这本册不带条件的第一页，往后翻、看总数都得走这一口。上方的检索与
 * 胶囊筛的是逐行那本册，不套到这里。
 */
function IdentityReviewUnits({ onLanded }: { onLanded: () => void }) {
  const { answer, pagination, retry } = useCatalogueCursor(noConditions, loadIdentityReviewUnits);
  if (answer === null) return null;
  if (answer.kind !== 'outcome') {
    const reason =
      answer.kind === 'transport'
        ? `连不上服务：${answer.message}`
        : answer.kind === 'callerProblem' || answer.kind === 'noAnswer'
          ? `错误码 ${answer.code}`
          : '没有查看权限或入口未配置，同上方列表';
    return (
      <div className="flex shrink-0 items-center gap-2 border-t border-idpxyz-border px-4 py-2 type-body text-idpxyz-danger">
        <span>处理身份冲突的货：按实物列的清单这次没有读出来（{reason}）。</span>
        <Button variant="ghost" size="sm" onClick={retry}>
          重试
        </Button>
      </div>
    );
  }
  if (answer.body.units.length === 0 && !pastFirstPage(pagination)) return null;
  return (
    <div className="max-h-[45%] shrink-0 overflow-y-auto border-t border-idpxyz-border p-4">
      <IdentityDispositionPanel units={answer.body.units} onLanded={onLanded} />
      {pagination ? (
        <div className="mt-2">
          <CursorPager pagination={pagination} />
        </div>
      ) : null}
    </div>
  );
}

/** 存储上还没有登记册的两区：不发请求，如实说「还不能在这里查看」。 */
function AbsentSection({ section, tabs }: { section: ReviewSection; tabs: ReactNode }) {
  const viewState: TemplateViewState = { kind: 'unconfigured' };
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <RouteWorkbench<ReviewRow>
        title={info.title}
        subtitle={subtitle}
        sections={tabs}
        answered={false}
        rows={[]}
        rowKey={(row) => row.key}
        searchCells={(row) => Object.values(row.values)}
        searchPlaceholder={`搜${section.word}`}
        columns={workbenchColumnsOf(section.columns, section.status)}
        renderDetail={(row, close) => <ReviewDetail row={row} columns={section.columns} subtitle={section.word} onClose={close} />}
        viewState={viewState}
        // 还没接进管理台的册：说「还不能在这里查看」，不露接入渠道的错误码（那个码说的是口没接渠道，不是册没建）。
        stateOverride={section.absence ? notConnectedOverride(section.absence.title, section.absence.description) : undefined}
      />
    </div>
  );
}

function RecheckAnswerNote({
  judgmentVersion,
  answer,
}: {
  judgmentVersion: string;
  answer: ApiResult<IdentityRecheckResponseBody>;
}) {
  const frame = 'type-body leading-4';
  switch (answer.kind) {
    case 'outcome': {
      const note = describeRecheckAnswer(answer.body);
      const tone = note.tone === 'answered' ? 'text-idpxyz-textMuted' : 'text-idpxyz-danger';
      return (
        <div className={`${frame} ${tone}`}>
          <p>再核对一次：{note.headline}</p>
          {note.details.length > 0 ? (
            <ul className="mt-0.5 ml-4 list-disc">
              {note.details.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          ) : null}
        </div>
      );
    }
    case 'unconfigured':
      return (
        <p className={`${frame} text-idpxyz-textMuted`}>
          再核对一次：操作入口还没有配置好，这次什么都没有改；重试也不会改变结果，请联系系统管理员完成配置。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="再核对" className={`${frame} text-idpxyz-textMuted`} />;
    case 'callerProblem':
      return (
        <p className={`${frame} text-idpxyz-danger`}>
          再核对一次没有完成：{recheckProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className={`${frame} text-idpxyz-danger`}>
          再核对一次，结果还不确定：{recheckProblemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className={`${frame} text-idpxyz-danger`}>连不上服务，这次没有核对；请检查网络后重试（{answer.message}）</p>;
  }
}
