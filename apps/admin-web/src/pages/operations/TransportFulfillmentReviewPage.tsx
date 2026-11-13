import { useEffect, useState, type ReactNode } from 'react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { RegistrationPanel } from '../../components/registration';
import {
  bookingAnswerEntryEndpoint,
  bookingAnswerEntryHint,
  bookingAnswerEntryOutcomeLabels,
  bookingAnswerEntryProblemNote,
  bookingAnswerEntryTitle,
  submitBookingAnswerEntry,
} from './booking-answer-entry';
import type { TemplateViewState } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { parcelNoOf } from '../shipment-request/ref-no';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { RouteWorkbench, SectionTabs, notConnectedOverride } from '../network/route-workbench';
import { usePartyNames } from '../settlement/party-names';
import { usePlaceNames } from '../network/use-place-names';
import { PageOnlyNote, PageOnlySearch } from '../visibility/page-only';
import {
  ReviewDetail,
  reviewStatusOf,
  workbenchColumnsOf,
  type ReviewColumn,
  type ReviewRow,
  type ReviewStatusSpec,
} from './review-workbench';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import {
  listTransportFulfillmentRecords,
  summarizeHandoverScope,
  transportFulfillmentConditions,
  type ApiResult,
  type HandoverScopeSummaryResponseBody,
  type TransportFulfillmentConditions,
  type TransportFulfillmentListResponseBody,
  type TransportFulfillmentRegistry,
} from './records-api';
import { scopeSummaryRowsOf, scopeSummaryViewState } from './handover-scope-summary';
import { handoverVerdictLabels, labelOf, masterDocumentStandingLabels } from './presentation';
import { labelFor } from '../../vocabulary/status';
import { documentTypeWords } from '../../vocabulary/document-numbering';
import { masterDocumentReferencesOf } from './transport-document-numbers';
import { bookingRowOf, bounceFilterDimensions, bounceRowOf, unitWord, type BounceFilter } from './booking-registers';
import {
  allStatuses,
  pageStatusChips,
  pushedStatusChips,
  reviewListConditions,
  rowsOnPage,
  transportPushedStatus,
} from './list-conditions';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['transport-fulfillment-review'];

// 委托、订舱的号名只取单据编号词典（ADR-0355 决定十一），不在本页另写。
const commissionNumberName = documentTypeWords.TRANSPORT_COMMISSION.numberName;
const bookingNumberName = documentTypeWords.BOOKING.numberName;

/**
 * 运输履约的治理查阅面。各区栏目取 transport-fulfillment CONTEXT.md 原词。本页只
 * 查阅履约判断与事实，不建班次、不订舱、不改交接结果、不更正交付。
 *
 * 五区接册面（GET /transport-fulfillment-records，票 admin-skeleton-closure-batch/05；
 * 总单一格由 ADR-0113 立册后补入）：班次、容量池、权威交接结果、交付证明、总单。
 * 运输舱单一区在存储上仍没有登记册——服务端的册名封闭集刻意不含那一格（没有表就
 * 没有读法，票 05 Comments），该区不发请求、如实呈现「无登记册」，不把「无处可登」
 * 演成「登记册为空」。总单立册前它与舱单同属一区，立册后拆开：一个有册一个没有，
 * 共用一份空态措辞会把两种空混成一种。
 *
 * 交接范围汇总一区走另一条读路（GET /transport-fulfillment-handover-scope-summary，
 * 票 admin-web-audit-followups/06）：它不整册上列，是按范围问一次的派生答案，因此有
 * 自己的入参、自己的四格结果代数与自己的空态措辞（判读在 handover-scope-summary.ts）。
 *
 * 有册的各区接读口分页（ADR-0144，票 catalogue-read-pagination-rollout/14）：检索框下推成 `q`，交接结果与总单的状态胶囊下推成
 * 筛选维，挡回册两个筛选框照旧是筛选维；订舱的状态词由读口派生、不是筛选维，胶囊只筛本页。读口检索只认标识与编码，网点与承运商
 * 名称、委托号、中文词这些在「只筛本页」的框里再筛。
 */

// 词表词（已交接/已拒收/待确认）按共享词表着色；词表没收录的词原样示文,
// 不猜色调。断言只桥接两个模块的类型边界,词本身同源于 CONTEXT 原词。
/**
 * 状态格：词取行上已译好的那一格；codeKey 给了就按码查词典色调（同码异义的册经 scope 分开），没给按词反查。
 * 原先这里走 domain/status 的旧徽章，只认得它自己那几个词；改走全站的状态胶囊，词典改一个词全站一起变。
 */
function pillCell(row: ReviewRow, id: string, codeKey?: string, scope?: string) {
  const word = row.values[id];
  if (!word) return '—';
  const code = codeKey ? row.values[codeKey] : undefined;
  return code ? <StatusPill code={code} scope={scope} label={word} /> : <StatusPill label={word} />;
}

/** 包裹号格：去掉线格式前缀的短写法上屏，整串在悬停与技术详情里。 */
function parcelCol(id: string, header: string): ReviewColumn<ReviewRow> {
  return {
    id,
    header,
    render: (row) => (
      <span className="font-mono type-body" title={row.values[id]}>
        {parcelNoOf(row.values[id] ?? '')}
      </span>
    ),
  };
}

/** 每行末尾的「技术详情」：判断版本、引用、更正指回这类排查时才用的字，空的不占格。 */
function techCol(pairs: [label: string, key: string][]): ReviewColumn<ReviewRow> {
  // 不上表：工作台把这一格挪进右栏的「技术详情」（review-workbench 的 ReviewDetail）。
  return { id: 'technical', header: '技术详情', render: () => null, techPairs: pairs };
}

function col(
  id: string,
  header: string,
  options?: {
    mono?: boolean;
    align?: 'left' | 'center' | 'right';
    className?: string;
    pill?: { codeKey?: string; scope?: string };
  },
): ReviewColumn<ReviewRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) => {
      const value = row.values[id] ?? '—';
      if (options?.pill) return pillCell(row, id, options.pill.codeKey, options.pill.scope);
      if (options?.mono) return <span className="font-mono type-body">{value}</span>;
      return value;
    },
  };
}

type SectionId =
  | 'transport-legs'
  | 'capacity-pools'
  | 'handover-results'
  | 'handover-scope-summary'
  | 'delivery-proofs'
  | 'master-documents'
  | 'booking-requests'
  | 'booking-answer-bounces'
  | 'transport-manifests';

interface ReviewSection {
  id: SectionId;
  /** CONTEXT.md 原词。 */
  word: string;
  /** 对应册名；无册区缺席（该区在存储上没有登记册，不发请求）。 */
  registry?: TransportFulfillmentRegistry;
  /** 检索框的提示：只写读口 `q` 覆盖的那几列（internal/transportfulfillment/adapters/postgres 各册的 Keyword）。 */
  keywordHint?: string;
  columns: ReviewColumn<ReviewRow>[];
  /** 无册区的如实呈现；与「登记册为空」严格分词。 */
  absence?: { title: string; description: string; unlock: string };
  /** 行状态胶囊取哪两格；给了即指标即筛。 */
  status?: ReviewStatusSpec;
  /** 汇总区：不整册上列，按范围逐次问一个派生答案（另一条读路，见下方 useEffect）。 */
  scopeSummary?: true;
}

const sections: ReviewSection[] = [
  {
    id: 'transport-legs',
    word: '班次',
    registry: 'transport-schedule',
    keywordHint: '搜班次、运输连接、两端网点编码、班次模板',
    // 每个明确日期、时间范围和运行方向的运输机会具有独立、稳定的班次身份
    // (CONTEXT)。执行准备判断(开放/关闭订舱、暂停、延误)与实际事实(出发、移动、
    // 到达…)在班次行上没有登记格——缺席说的是「无处可登」,本区不代填「未出发」
    // 一类派生状态列(票 05 Comments)。
    columns: [
      col('scheduleId', '班次', { mono: true }),
      col('leg', '从 → 到'),
      col('departsAt', '出发时间', { mono: true }),
      col('arrivesAt', '到达时间', { mono: true }),
      col('runningDate', '运行日', { mono: true }),
      techCol([
        ['两端网点编码', 'legCodes'],
        ['运输连接', 'connection'],
        ['服务范围', 'serviceScope'],
        ['班次模板', 'template'],
        ['运行方向', 'direction'],
        ['登记时间', 'recordedAt'],
      ]),
    ],
  },
  {
    id: 'capacity-pools',
    word: '舱位',
    registry: 'capacity-pool',
    keywordHint: '搜舱位池、班次',
    // 容量池只启用适用维度,不强制换算成一个数值(CONTEXT);有效容量、已预占、
    // 已释放、实际使用分别维护,各占一列,页面不互相抵扣算「可用量」——那是领域
    // 按时点算的判断。
    // 四个量各自照实列出，不互相抵扣成「剩余」：可用量是领域按时点算的判断，读面不代算（records-api 的 CapacityPoolRecord）。
    columns: [
      col('schedule', '班次', { mono: true }),
      col('unit', '按什么计', { mono: true, className: 'w-[96px]' }),
      col('capacity', '总舱位', { align: 'right', mono: true }),
      col('reserved', '已预订', { align: 'right', mono: true }),
      col('released', '已释放', { align: 'right', mono: true }),
      col('consumed', '已装载', { align: 'right', mono: true }),
      techCol([['舱位池', 'poolId']]),
    ],
  },
  {
    id: 'handover-results',
    word: '交接结果',
    registry: 'transport-handover',
    keywordHint: '搜包裹、交接批次、交出方或接收方编码',
    status: { labelKey: 'verdict', codeKey: 'verdictCode', scope: 'handover' },
    // 权威运输交接结果由本上下文唯一形成,逐载运对象判断并允许部分成立(CONTEXT);
    // 结果词(已交接/已拒收/待确认)在共享词表内,按词表着色。一行一判断版本,更正
    // 是新行指回前版(更正指回列),原判断在册面上继续可见;整批、整车结论只能由
    // 对象级结果派生,本区行粒度就是载运对象。
    columns: [
      parcelCol('object', '包裹'),
      col('boundary', '从谁交给谁'),
      col('verdict', '结果', { pill: { codeKey: 'verdictCode', scope: 'handover' }, className: 'w-[104px]' }),
      col('judgedAt', '交接时间', { mono: true }),
      techCol([
        ['包裹标识', 'object'],
        ['交出方 → 接收方编码', 'boundaryCodes'],
        ['交接批次', 'scope'],
        ['判断版本', 'version'],
        ['依据', 'basis'],
        ['更正了', 'corrects'],
      ]),
    ],
  },
  {
    id: 'handover-scope-summary',
    word: '交接批次汇总',
    scopeSummary: true,
    // 整批/整车/整袋结论只能由对象级结果派生(CONTEXT);本区因此不是第六本册,是上一区
    // 那本册的派生问答——一个范围问一次,答的是「各裁决各有多少」。三格计数与`全部已
    // 交接`都取服务端派生的那一份,页面不相加也不比对:自己算就是为同一条规则立第二个
    // 口径,领域改一次判据、页面这份会悄悄漂移。
    columns: [
      col('scope', '交接批次', { mono: true }),
      col('handedOver', '已交接', { align: 'right', mono: true }),
      col('refused', '已拒收', { align: 'right', mono: true }),
      col('unconfirmed', '待确认', { align: 'right', mono: true }),
      col('total', '合计', { align: 'right', mono: true }),
      col('allHandedOver', '全部交接了没有', { align: 'center', className: 'w-[120px]' }),
    ],
  },
  {
    id: 'delivery-proofs',
    word: '签收记录',
    registry: 'effective-delivery',
    keywordHint: '搜包裹、派送尝试、签收地点、签收人、签收凭证',
    // 有效交付结果必须关联载运对象、履约尝试、业务时间、地点、交付方式、接收对象
    // 和符合当时规则的交付证明(CONTEXT);「已签收」不是可直接修改的状态,证明列
    // 是引用不是证据内容,更正走新判断版本(更正指回列),本册只列当前版。
    columns: [
      parcelCol('object', '包裹'),
      col('method', '怎么签收的'),
      col('recipient', '签收人', { mono: true }),
      col('place', '签收地点', { mono: true }),
      col('occurredAt', '签收时间', { mono: true }),
      techCol([
        ['包裹标识', 'object'],
        ['派送尝试', 'attempt'],
        ['签收凭证', 'proof'],
        ['判断版本', 'version'],
        ['签收方式码', 'methodCode'],
        ['更正了', 'corrects'],
      ]),
    ],
  },
  {
    id: 'master-documents',
    word: '总单',
    registry: 'carrier-master-document',
    keywordHint: '搜总单号、签发方、凭证范围、运输委托或订舱标识',
    status: { labelKey: 'standing', codeKey: 'standingCode' },
    // 总单是运营企业与外部运输服务提供方之间针对明确运输范围形成的主运输凭证
    // (CONTEXT)；跨上下文文本里的「承运总单」是它的别名。一行一版本：撤销、替代、
    // 关联重述都是新行回指前版(回指前版列)，原版本在册面上继续可见；关联数是关联
    // 子表的计数，关联本体(哪些集运单元、包裹、段)不在列面展开——列入总单不证明
    // 装载、交接或运输，本区不派生任何控制状态词。与监管舱单分界:监管舱单由承运商
    // 在外部形成并提交，关务经 UC-CC-012 只接受引用，不在本页。
    // 所引委托、订舱的号列有号显号、没号显标识，标识与无号原因进技术详情（ADR-0355 决定十一）。读口检索认的是所引委托、订舱的
    // 标识，不认它们的号；按号找引用它的那几版只在「只筛本页」的框里。
    columns: [
      col('document', '总单号', { mono: true }),
      col('issuer', '签发方', { mono: true }),
      col('standing', '状态', { pill: { codeKey: 'standingCode' }, className: 'w-[104px]' }),
      col('commissionNumber', commissionNumberName, { mono: true }),
      col('bookingNumber', bookingNumberName, { mono: true }),
      col('associationCount', '关联件数', { align: 'right', mono: true, className: 'w-[88px]' }),
      col('changedAt', '变更时间', { mono: true }),
      techCol([
        ['版本', 'version'],
        ['凭证范围', 'scope'],
        ['运输委托标识', 'commissionId'],
        ['委托无号原因', 'commissionNote'],
        ['订舱标识', 'bookingId'],
        ['订舱无号原因', 'bookingNote'],
        ['接续', 'supersedes'],
        ['被替代为', 'replacedBy'],
      ]),
    ],
  },
  {
    id: 'booking-requests',
    word: '订舱',
    registry: 'booking-request',
    keywordHint: `搜${bookingNumberName}、订舱标识、运输委托标识、班次`,
    status: { labelKey: 'standing' },
    // 一次订舱一行。状态词由读口按名下当前那一份承运答复读出，页面原词示文、不另算；取消是运营的决定
    // 不是承运答复，取消时刻与取消人单列，不并进状态（CONTEXT「订舱」）。提交人、取消人没记下时留空。
    // 主列是订舱的单据号，没有号的照旧显示订舱标识；订舱标识与无号原因进技术详情（ADR-0355 决定十一，booking-registers）。
    columns: [
      col('booking', bookingNumberName, { mono: true }),
      col('schedule', '班次', { mono: true }),
      col('requested', '订了多少', { align: 'right', mono: true }),
      col('requestedAt', '订舱时间', { mono: true }),
      col('standing', '状态', { pill: {}, className: 'w-[96px]' }),
      col('currentAnswer', '承运商答复', { className: 'w-[96px]' }),
      col('acceptedQuantity', '接受了多少', { align: 'right', mono: true }),
      col('cancelledAt', '取消时间', { mono: true }),
      techCol([
        ['订舱标识', 'bookingId'],
        ['单据号', 'numberNote'],
        [commissionNumberName, 'commission'],
        ['运输委托标识', 'commissionId'],
        ['答复时间', 'currentAnsweredAt'],
        ['提交人', 'submittedBy'],
        ['取消人', 'cancelledBy'],
      ]),
    ],
  },
  {
    id: 'booking-answer-bounces',
    word: '被挡回的答复',
    registry: 'booking-answer-bounce',
    keywordHint: '搜答复方、订舱标识、答复标识',
    // 挡回册是台账不是事实来源：挡回不形成承运接受，订舱的状态词不读它。依据两词（订舱不在册 /
    // 不是服务提供方）对答复方是同一格答复，只有这里分得开（CONTEXT「承运答复挡回」）。
    // 订舱一列是承运方答复里点名的订舱标识（承运答复照旧按它认单），不是订舱号——没在册的标识本来就没有号。
    columns: [
      col('source', '谁答复的', { mono: true }),
      col('booking', '订舱标识', { mono: true }),
      col('basis', '为什么挡回'),
      col('firstBouncedAt', '第一次', { mono: true }),
      col('lastBouncedAt', '最近一次', { mono: true }),
      col('count', '次数', { align: 'right', mono: true, className: 'w-[72px]' }),
      techCol([['答复标识', 'answer']]),
    ],
  },
  {
    id: 'transport-manifests',
    word: '运输舱单',
    // 运输舱单表达某次运输计划或申报的载运内容，与总单具有不同业务身份(CONTEXT)。
    // 它在存储上仍没有登记册；总单立册(ADR-0113)后本区单独成格，不再与总单共用
    // 一份「无登记册」文案。
    columns: [
      col('manifestId', '舱单号', { mono: true }),
      col('relatedScope', '班次 / 运段', { mono: true }),
      col('businessTime', '时间', { mono: true }),
    ],
    // 存储上没有运输舱单登记表，查询端点的册名封闭集刻意不含本区（票 admin-skeleton-closure-batch/05 Comments）：没有表就没有
    // 读法，答一份恒空的册子会把「无处可登」演成「登记册为空」。总单已另立登记册（ADR-0113）。
    absence: {
      title: '运输舱单还不能在这里查看',
      description: '运输舱单还没有接入管理台，这不是「没有舱单」。承运商总单在「总单」签里查看。',
      unlock: '运输舱单接入后，这一签会自动显示',
    },
  },
];

/**
 * 册行转写。班次两端与交接的交出方、接收方按 label 换成屏上写法：网点按此刻生效那一版的名称（use-place-names），承运商等参与方
 * 按往来单位册的名称（settlement/party-names），场外地点没有任何册登名称、照显编码。两端原编码各进技术详情。
 */
function rowsOf(body: TransportFulfillmentListResponseBody, label: (code: string) => string): ReviewRow[] {
  const pair = (from: string | undefined, to: string | undefined) => `${from ? label(from) : '—'} → ${to ? label(to) : '—'}`;
  switch (body.outcome) {
    case 'TRANSPORT_SCHEDULES_LISTED':
      return body.schedules.map((record) => ({
        key: `schedule:${record.scheduleId}`,
        values: {
          scheduleId: record.scheduleId,
          leg: record.fromNode || record.toNode ? pair(record.fromNode, record.toNode) : '',
          legCodes: record.fromNode || record.toNode ? `${record.fromNode ?? '—'} → ${record.toNode ?? '—'}` : '',
          departsAt: formatInstant(record.departsAt),
          arrivesAt: record.arrivesAt ? formatInstant(record.arrivesAt) : '',
          runningDate: record.runningDate ?? '',
          connection: record.connection ?? '',
          serviceScope: record.serviceScope ?? '',
          template: record.template ? `${record.template}${record.templateVersion ? ` · 第 ${record.templateVersion} 版` : ''}` : '',
          direction: record.direction ?? '',
          recordedAt: formatInstant(record.recordedAt),
        },
      }));
    case 'CAPACITY_POOLS_LISTED':
      return body.pools.map((record) => ({
        key: `pool:${record.poolId}`,
        values: {
          poolId: record.poolId,
          schedule: record.schedule,
          unit: unitWord(record.unit),
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
          boundary: pair(record.releasedBy, record.receivedBy),
          boundaryCodes: `${record.releasedBy} → ${record.receivedBy}`,
          verdict: labelOf(handoverVerdictLabels, record.verdict),
          verdictCode: record.verdict,
          scope: record.scope,
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
          method: labelFor(record.method),
          methodCode: record.method,
          version: record.version,
          recipient: record.recipient,
          place: record.place,
          occurredAt: formatInstant(record.occurredAt),
          proof: record.proof,
          corrects: record.correctsVersion
            ? `${record.correctsVersion}${record.correctedAt ? ` · ${formatInstant(record.correctedAt)}` : ''}`
            : '',
        },
      }));
    case 'CARRIER_MASTER_DOCUMENTS_LISTED':
      return body.masterDocuments.map((record) => ({
        key: `master-document:${record.document}:${record.version}`,
        values: {
          document: record.document,
          version: record.version,
          issuer: record.issuer,
          scope: record.scope,
          standing: labelOf(masterDocumentStandingLabels, record.standing),
          standingCode: record.standing,
          associationCount: record.associationCount,
          ...masterDocumentReferencesOf(record),
          supersedes: record.supersedes ?? '',
          replacedBy: record.replacedBy ?? '',
          changedAt: record.changedAt ? formatInstant(record.changedAt) : '',
        },
      }));
    case 'BOOKING_REQUESTS_LISTED':
      return body.bookings.map(bookingRowOf);
    case 'BOOKING_ANSWER_BOUNCES_LISTED':
      return body.bounces.map(bounceRowOf);
  }
}

const subtitle =
  '班次、舱位、订舱与承运商答复，以及货在各段交接成没成功、最后怎么签收的。这里只查看；排班、订舱与交接在运输作业端办理。';

// useCatalogueCursor 要稳定引用；按 registry 收窄的泛型读口在这里定成各册答复的并集，rowsOf 按 outcome 择形状。
const loadTransportRecords = (
  conditions: TransportFulfillmentConditions<TransportFulfillmentRegistry>,
  after: string | null,
) => listTransportFulfillmentRecords(conditions, after);

// 订舱的状态词由读口按名下当前那一份承运答复派生（ports.BookingStanding*），不是筛选维：胶囊只筛本页。
const bookingStandingOf = (row: ReviewRow) => row.values.standing;

/**
 * 运输班次页。「运输班次」签是原来的查阅面；「录入订舱答复」签给没有对接承运商系统的租户：承运方用电话或邮件答复订舱
 * 之后，照着录进来。
 */
export function TransportFulfillmentReviewPage() {
  return (
    <Tabs defaultValue="review" className="flex-1 flex flex-col overflow-hidden gap-0 bg-idpxyz-editor">
      <TabsList className="px-4 shrink-0">
        <TabsTrigger value="review">运输班次</TabsTrigger>
        <TabsTrigger value="booking-answer-entry">录入订舱答复</TabsTrigger>
      </TabsList>
      <TabsContent value="review" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
        <TransportFulfillmentReview />
      </TabsContent>
      <TabsContent value="booking-answer-entry" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
        <RegistrationPanel
          moduleId="transport-fulfillment-review"
          title={bookingAnswerEntryTitle}
          endpoint={`POST ${bookingAnswerEntryEndpoint}`}
          snapshotHint={bookingAnswerEntryHint}
          submit={submitBookingAnswerEntry}
          outcomeLabels={bookingAnswerEntryOutcomeLabels}
          problemNote={bookingAnswerEntryProblemNote}
        />
      </TabsContent>
    </Tabs>
  );
}

function TransportFulfillmentReview() {
  const [sectionId, setSectionId] = useState<SectionId>(sections[0].id);
  // 订舱区一行跳到挡回区时带过去的那一单；挡回区按它起筛，换签即作废。
  const [bounceSeed, setBounceSeed] = useState<BounceFilter | null>(null);
  const places = usePlaceNames();
  const parties = usePartyNames();
  const section = sections.find((candidate) => candidate.id === sectionId) ?? sections[0];
  const tabs = (
    <SectionTabs
      sections={sections.map((candidate) => ({ id: candidate.id, label: candidate.word }))}
      active={sectionId}
      onChange={(id) => {
        setBounceSeed(null);
        setSectionId(id);
      }}
    />
  );
  // 订舱区的一行链到挡回区按那一单筛出的结果；答复方筛选清空，只见这一单。挡回册按订舱标识记，所以带标识、不带订舱号。
  const jumpToBounces = (booking: string) => {
    setBounceSeed({ booking, source: '' });
    setSectionId('booking-answer-bounces');
  };
  // 换区重挂：上一区的检索、胶囊、翻页位置与选中行不带进下一区。
  return section.registry ? (
    <RegistrySection
      key={section.id}
      section={section}
      registry={section.registry}
      tabs={tabs}
      label={(code) => places.get(code) ?? parties.party(code) ?? code}
      bounceSeed={bounceSeed}
      onJumpToBounces={jumpToBounces}
    />
  ) : (
    <UnlistedSection key={section.id} section={section} tabs={tabs} />
  );
}

/** 有册的一区：按读口分页取数，检索与能下推的胶囊下推，订舱状态与「只筛本页」的框在到手的这一页上筛。 */
function RegistrySection({
  section,
  registry,
  tabs,
  label,
  bounceSeed,
  onJumpToBounces,
}: {
  section: ReviewSection;
  registry: TransportFulfillmentRegistry;
  tabs: ReactNode;
  label: (code: string) => string;
  bounceSeed: BounceFilter | null;
  onJumpToBounces: (booking: string) => void;
}) {
  const isBounces = registry === 'booking-answer-bounce';
  const pushed = transportPushedStatus[registry];
  const pageStatus = registry === 'booking-request' ? bookingStandingOf : undefined;
  const [keyword, setKeyword] = useState('');
  // 下推的胶囊存读口词表里的词，只筛本页的胶囊存页上的状态词；一区只有其中一种。
  const [status, setStatus] = useState(allStatuses);
  const [refine, setRefine] = useState('');
  // 挡回册的两个筛选分草稿与已提交两格：每次筛选是一次真请求。
  const [bounceDraft, setBounceDraft] = useState<BounceFilter>(bounceSeed ?? { booking: '', source: '' });
  const [bounceFilter, setBounceFilter] = useState<BounceFilter>(bounceSeed ?? {});
  const conditions = transportFulfillmentConditions(
    registry,
    reviewListConditions(useSettledKeyword(keyword), pushed, status, isBounces ? bounceFilterDimensions(bounceFilter) : {}),
  );
  const { answer, pagination, retry } = useCatalogueCursor(conditions, loadTransportRecords);
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const pageRows = body ? rowsOf(body, label) : [];
  const rows = rowsOnPage(pageRows, refine, status, pageStatus);
  const applyBounceFilter = (next: BounceFilter) => {
    setBounceDraft({ booking: next.booking ?? '', source: next.source ?? '' });
    setBounceFilter({ booking: next.booking?.trim() ?? '', source: next.source?.trim() ?? '' });
  };
  const statusChips = pushed
    ? pushedStatusChips(pushed, status, setStatus)
    : pageStatus && body
      ? pageStatusChips(pageRows.map(pageStatus), status, setStatus)
      : undefined;
  const viewState = catalogueViewState(answer, pageRows.length, retry, {
    module: info,
    endpoint: `GET /transport-fulfillment-records?registry=${registry}`,
    emptyTitle: isBounces ? '没有被挡回的承运答复' : `还没有${section.word}`,
    emptyDescription: isBounces
      ? '承运商的答复对不上订舱、或不是该班次的承运商发来时会被挡回，记在这里供核对。'
      : '运输作业端排班、订舱、交接、签收后，记录会出现在这里。',
    narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
  });

  const filters = (
    <>
      {isBounces ? (
        <span className="inline-flex items-center gap-2">
          <input
            aria-label="按订舱标识筛"
            className="h-8 w-[160px] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body"
            placeholder="按订舱标识筛"
            value={bounceDraft.booking ?? ''}
            onChange={(event) => setBounceDraft({ ...bounceDraft, booking: event.target.value })}
          />
          <input
            aria-label="按答复的承运商筛"
            className="h-8 w-[160px] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body"
            placeholder="按答复的承运商筛"
            value={bounceDraft.source ?? ''}
            onChange={(event) => setBounceDraft({ ...bounceDraft, source: event.target.value })}
          />
          <Button variant="outline" size="sm" onClick={() => applyBounceFilter(bounceDraft)}>
            筛选
          </Button>
          <Button variant="ghost" size="sm" onClick={() => applyBounceFilter({})}>
            清除筛选
          </Button>
        </span>
      ) : null}
      {pageStatus && body ? <PageOnlyNote>状态只筛本页</PageOnlyNote> : null}
      <PageOnlySearch value={refine} onChange={setRefine} subject="名称等其余各格" />
    </>
  );

  return (
    <RouteWorkbench<ReviewRow>
      title={info.title}
      subtitle={subtitle}
      sections={tabs}
      extraFilters={filters}
      answered={body !== null}
      rows={rows}
      rowKey={(row) => row.key}
      statusChips={statusChips}
      onClearFilters={() => {
        setStatus(allStatuses);
        setRefine('');
        if (isBounces) applyBounceFilter({});
      }}
      rowAttention={(row) => ['REFUSED', 'PENDING_CONFIRMATION'].includes(row.values.verdictCode ?? '')}
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
          onClose={close}
          actions={
            section.id === 'booking-requests' ? (
              <Button variant="outline" size="sm" onClick={() => onJumpToBounces(row.values.bookingId)}>
                看这笔订舱被挡回的答复
              </Button>
            ) : undefined
          }
        >
          {section.id === 'booking-requests' && row.values.answerCount !== '0' ? (
            <>
              <p>承运商的全部答复（{row.values.answerCount} 份，含候补与后继）：</p>
              <p className="whitespace-pre-line font-mono">{row.values.answers}</p>
            </>
          ) : null}
        </ReviewDetail>
      )}
      onRefresh={retry}
      toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
      viewState={viewState}
    />
  );
}

/** 没有整册上列的两区：交接批次汇总按批次问一次派生答案；运输舱单还没有登记册，不发请求。 */
function UnlistedSection({ section, tabs }: { section: ReviewSection; tabs: ReactNode }) {
  const isScopeSummary = section.scopeSummary === true;
  const [reloadKey, setReloadKey] = useState(0);
  // 汇总区的范围入参分两格：输入框里的草稿与已提交的那一个。合成一格就会逐键触发查询,
  // 而这一区每问一次都是一次真请求。
  const [scopeDraft, setScopeDraft] = useState('');
  const [scope, setScope] = useState('');
  const [summaryLoaded, setSummaryLoaded] = useState<{
    scope: string;
    answer: ApiResult<HandoverScopeSummaryResponseBody>;
  } | null>(null);

  useEffect(() => {
    // 范围空着不发请求：范围是这个读面的必备维，缺席在服务端是 400，而那条 400 说的是
    // 「你问错了」，与「还没问」不是一件事。
    if (!isScopeSummary || scope.trim() === '') return undefined;
    let cancelled = false;
    void summarizeHandoverScope(scope).then((answer) => {
      if (!cancelled) setSummaryLoaded({ scope, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [isScopeSummary, scope, reloadKey]);

  const summaryAnswer = isScopeSummary && summaryLoaded?.scope === scope ? summaryLoaded.answer : null;
  const summaryBody = summaryAnswer?.kind === 'outcome' ? summaryAnswer.body : null;
  const rows = summaryBody ? scopeSummaryRowsOf(summaryBody) : [];
  const retry = () => setReloadKey((value) => value + 1);
  const viewState: TemplateViewState = section.absence
    ? { kind: 'unconfigured' }
    : scopeSummaryViewState(summaryAnswer, scope, retry, {
        module: info,
        endpoint: 'GET /transport-fulfillment-handover-scope-summary?scope=…',
      });

  const filters = isScopeSummary ? (
    // 汇总按批次问一次，不是整册上列：批次号走自己的输入框与「查询汇总」，不借工具条的检索（那是本地过滤）。
    <span className="inline-flex items-center gap-2">
      <input
        aria-label="交接批次号"
        className="h-8 w-[220px] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 font-mono type-body"
        placeholder="交接批次号"
        value={scopeDraft}
        onChange={(event) => setScopeDraft(event.target.value)}
        onKeyDown={(event) => event.key === 'Enter' && setScope(scopeDraft.trim())}
      />
      <Button variant="outline" size="sm" disabled={scopeDraft.trim() === ''} onClick={() => setScope(scopeDraft.trim())}>
        查询汇总
      </Button>
    </span>
  ) : undefined;

  return (
    <RouteWorkbench<ReviewRow>
      title={info.title}
      subtitle={subtitle}
      sections={tabs}
      extraFilters={filters}
      answered={summaryBody !== null}
      rows={rows}
      rowKey={(row) => row.key}
      // 汇总区至多一行，检索框在那一区只在这一行里找；批次号是上方的入参，不是过滤词。
      searchCells={(row) => Object.values(row.values)}
      searchPlaceholder={isScopeSummary ? '在汇总里搜…' : `搜${section.word}`}
      columns={workbenchColumnsOf(section.columns, section.status)}
      renderDetail={(row, close) => (
        <ReviewDetail row={row} columns={section.columns} status={reviewStatusOf(row, section.status)} subtitle={section.word} onClose={close} />
      )}
      onRefresh={section.absence ? undefined : retry}
      viewState={viewState}
      // 还没接进管理台的册：说「还不能在这里查看」，不露接入渠道的错误码（那个码说的是口没接渠道，不是册没建）。
      stateOverride={section.absence ? notConnectedOverride(section.absence.title, section.absence.description) : undefined}
    />
  );
}
