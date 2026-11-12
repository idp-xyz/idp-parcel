import type { TechnicalRow } from '../../components/TechnicalDetails';
import type { OperatorRefusalKind } from '../../auth/access-refusal';
import { chargeCodeLabel } from '../../vocabulary/status';
import type { ApiResult } from '../catalogue-api';
import type {
  PreAcceptanceEstimateRecord,
  PreAcceptanceEstimatesResponseBody,
  PreAcceptanceOccupationRecord,
  PreAcceptanceParcelEstimateRecord,
} from '../settlement/api';
import { formatMoney, isPositiveMinorAmount } from '../settlement/money';
import type { LastAttemptView, ResumeNote } from './last-attempt';
import { answerTechnicalRows } from './words';

// 运单详情「受理前财务控制」一块里估价那一半的判读（ADR-0379 决定四，票 declared-goods-lines/21）。纯函数，node:test 直接钉；
// React 只在 PreAcceptanceFinancialControl.tsx。
//
// 估价读的是结算的读口，结论读的是委托详情：两侧各交各的、页面拼，所以这一半有自己的加载、出错与空态——结算那一口读不到，
// 运单详情与结论照样在。
//
// 金额两种写法分开处理：合计、逐件小计与占用金额是最小币单位整数串，走费用页同一个 formatMoney；费用行是试算当时算出的
// 主单位十进制串，照原样带它自己的币种，不能再过一遍最小币单位的写法。
//
// 只说数据说得出的话：留痕在而账上没有占用，原因不止受限一种，所以只有当前版本、且结论确是受限时才说「控制受限」。

export interface ChargeLineView {
  code: string;
  label: string;
  effect: string;
  amount: string;
}

export interface ParcelEstimateView {
  parcel: string;
  subtotal: string;
  /** 计价重量；没记下（补记之前的旧行，或补记之后这一件缺这一格）答「未记」。旧行整列不出由页面按 detailRecorded 定。 */
  weight: string;
  /** null 是没记下费用行（与「记了、零行」分开）。 */
  charges: ChargeLineView[] | null;
}

export interface OccupationView {
  /** 占用在结算账本上的标识，一份估价里不重。 */
  id: string;
  kind: string;
  status: string;
  amount: string;
  /** 已扣费的那部分；没扣过（"0" 或缺席）不出。 */
  converted?: string;
  times: { label: string; at: string }[];
  /** 一行写完：种类 · 状态 · 金额（· 已扣多少）。 */
  summary: string;
}

export interface EstimateSectionView {
  submissionVersion: string;
  isCurrent: boolean;
  versionLabel: string;
  total: string;
  form: string;
  priceCard: { plan: string; version: string; text: string };
  serviceProduct?: string;
  trialAt: string;
  recordedAt: string;
  detailRecorded: boolean;
  /** detailRecorded 为 false 时那一句。 */
  detailNote?: string;
  parcels: ParcelEstimateView[];
  occupations: OccupationView[];
  /** 没有占用时那一句。 */
  occupationNote?: string;
  technical: TechnicalRow[];
}

/** 一段估价要知道的上下文：哪一版是当前的，委托详情记下的结论是什么（没判过缺席）。 */
export interface EstimateContext {
  currentVersion: string;
  controlOutcome?: string;
}

export type EstimateBlockState =
  | { kind: 'loading' }
  | { kind: 'unconfigured'; text: string }
  | { kind: 'refused'; refusal: OperatorRefusalKind; what: string }
  | { kind: 'problem'; text: string; technical: TechnicalRow[] }
  | { kind: 'unreachable'; title: string; description: string; technical: TechnicalRow[] }
  | { kind: 'none'; note: string; resume?: ResumeNote }
  | { kind: 'formed'; sections: EstimateSectionView[]; gapNote?: string; resume?: ResumeNote };

export const NOT_RECORDED_DETAIL_NOTE = '这份估价形成于补记之前，没有逐件明细';

const formWords: Record<string, string> = {
  PER_PARCEL_SELL_TRIAL_SUM: '逐件售价试算合计',
};

const effectWords: Record<string, string> = {
  ADD: '加项',
  DEDUCT: '减项',
};

const occupationKindWords: Record<string, string> = {
  FUNDS_FREEZE: '资金冻结',
  CREDIT_EXPOSURE: '信用占用',
};

// 状态码按种类各一套（冻结册 HELD，信用册 RECORDED），说法取结算的叫法：冻结中或占用中 → 已释放或已扣费。
const occupationStatusWords: Record<string, Record<string, string>> = {
  FUNDS_FREEZE: { HELD: '资金冻结中', RELEASED: '已释放', CONVERTED: '已扣费' },
  CREDIT_EXPOSURE: { RECORDED: '信用占用中', RELEASED: '已释放', CONVERTED: '已扣费' },
};

const occupationFormedAtLabels: Record<string, string> = {
  FUNDS_FREEZE: '冻结时刻',
  CREDIT_EXPOSURE: '占用时刻',
};

export function occupationViewOf(occupation: PreAcceptanceOccupationRecord): OccupationView {
  const kind = occupationKindWords[occupation.kind] ?? occupation.kind;
  const statusWords = occupationStatusWords[occupation.kind] ?? {};
  const converted = isPositiveMinorAmount(occupation.convertedAmount)
    ? formatMoney(occupation.convertedAmount, occupation.currency)
    : undefined;
  // 部分转扣费的占用状态不变（还占着余下那部分），只靠已扣金额看得出来；整笔扣完是 CONVERTED、扣过一部分再放掉余下的是
  // RELEASED，这两种状态词已说清，只另列已扣多少。
  const stillHeld = occupation.status === 'HELD' || occupation.status === 'RECORDED';
  const baseStatus = statusWords[occupation.status] ?? occupation.status;
  const status = converted && stillHeld ? `${baseStatus}，部分已扣费` : baseStatus;
  const amount = formatMoney(occupation.amount, occupation.currency);
  const times = [{ label: occupationFormedAtLabels[occupation.kind] ?? '占用时刻', at: occupation.formedAt }];
  if (occupation.releasedAt) times.push({ label: '释放时刻', at: occupation.releasedAt });
  if (occupation.convertedAt) times.push({ label: '扣费时刻', at: occupation.convertedAt });
  const summary = [kind, status, amount, ...(converted ? [`已扣 ${converted}`] : [])].join(' · ');
  return { id: occupation.id, kind, status, amount, ...(converted ? { converted } : {}), times, summary };
}

function parcelViewOf(parcel: PreAcceptanceParcelEstimateRecord): ParcelEstimateView {
  return {
    parcel: parcel.parcel,
    subtotal: formatMoney(parcel.totalAmount, parcel.currency),
    weight: parcel.pricingWeight ? `${parcel.pricingWeight.value} ${parcel.pricingWeight.unit}` : '未记',
    charges: parcel.chargeLines
      ? parcel.chargeLines.map((line) => ({
          code: line.code,
          label: chargeCodeLabel(line.code),
          effect: effectWords[line.effect] ?? line.effect,
          amount: `${line.amount} ${line.currency}`,
        }))
      : null,
  };
}

function sectionTechnicalRows(estimate: PreAcceptanceEstimateRecord): TechnicalRow[] {
  const rows: TechnicalRow[] = [
    { label: '提交版本', value: estimate.submissionVersion },
    { label: '估价形态', value: estimate.form },
    { label: '计价范围', value: estimate.pricingScope },
    { label: '价卡内容指纹', value: estimate.priceCard.fingerprint },
  ];
  for (const parcel of estimate.parcels) rows.push({ label: `试算评价 · ${parcel.parcel}`, value: parcel.evaluation });
  for (const occupation of estimate.occupations) {
    rows.push({ label: `${occupationKindWords[occupation.kind] ?? occupation.kind}标识`, value: occupation.id });
  }
  return rows;
}

/**
 * Go 的 nil 切片编成 null 而不是 []：几个数组格读进来先收成数组，后面的组装才不必处处设防——一处漏了，渲染抛错就会把整张
 * 运单详情换成出错页。费用行收成缺席，与「没记下」同读，因为 null 分不出是没记还是零行。
 */
export function normalisedEstimateOf(raw: PreAcceptanceEstimateRecord): PreAcceptanceEstimateRecord {
  return {
    ...raw,
    parcels: (raw.parcels ?? []).map((parcel) => ({
      ...parcel,
      pricingWeight: parcel.pricingWeight ?? undefined,
      chargeLines: parcel.chargeLines ?? undefined,
    })),
    occupations: raw.occupations ?? [],
  };
}

/** 一段估价。入参须已过 normalisedEstimateOf。 */
export function estimateSectionOf(estimate: PreAcceptanceEstimateRecord, context: EstimateContext): EstimateSectionView {
  const isCurrent = estimate.submissionVersion === context.currentVersion;
  const occupations = estimate.occupations.map(occupationViewOf);
  const restrictedNow = isCurrent && context.controlOutcome === 'RESTRICTED';
  return {
    submissionVersion: estimate.submissionVersion,
    isCurrent,
    versionLabel: isCurrent ? '当前版本' : '此前版本',
    total: formatMoney(estimate.totalAmount, estimate.currency),
    form: formWords[estimate.form] ?? estimate.form,
    priceCard: {
      plan: estimate.priceCard.plan,
      version: estimate.priceCard.version,
      // 与合同引用同一种写法（方案@版本），链接另按方案标识去价格表。
      text: `${estimate.priceCard.plan}@${estimate.priceCard.version}`,
    },
    ...(estimate.serviceProduct ? { serviceProduct: estimate.serviceProduct } : {}),
    trialAt: estimate.trialAt,
    recordedAt: estimate.recordedAt,
    detailRecorded: estimate.detailRecorded,
    ...(estimate.detailRecorded ? {} : { detailNote: NOT_RECORDED_DETAIL_NOTE }),
    parcels: estimate.parcels.map(parcelViewOf),
    occupations,
    ...(occupations.length === 0 ? { occupationNote: restrictedNow ? '没有占用：控制受限' : '没有占用' } : {}),
    technical: sectionTechnicalRows(estimate),
  };
}

/**
 * 逐提交版本一段，按试算时点新的在前：当前版本多半是最后试算的那一份，排在最前，人不用往下翻就看到现在适用的那一份。
 * 时点读不出时退回读口次序的倒序（读口按试算时点正序交）。入参须已过 normalisedEstimateOf。
 */
export function estimateSectionsOf(estimates: readonly PreAcceptanceEstimateRecord[], context: EstimateContext): EstimateSectionView[] {
  return estimates
    .map((estimate, index) => ({ estimate, index, at: Date.parse(estimate.trialAt) }))
    .sort((left, right) => {
      const byTime = Number.isFinite(left.at) && Number.isFinite(right.at) ? right.at - left.at : 0;
      return byTime !== 0 ? byTime : right.index - left.index;
    })
    .map(({ estimate }) => estimateSectionOf(estimate, context));
}

const NO_CONTROL_NOTE = '合同约定无财务控制，不做估价';

/**
 * 估价那一半显什么。没有留痕时按能说的说：结论是不适用就说合同约定不做；受理任务最近停在财务控制那一步就把停因与谁来续办说
 * 出来——估价没形成的原因在受理任务的处理尝试上，结算不另存（ADR-0379 决定三、ADR-0383 决定三）。有留痕而没有当前版本的那一份
 * （换了版本、新版本还停着），同样说一句，免得把旧版本的估价读成现在的。
 */
export function estimateBlockStateOf(
  answer: ApiResult<PreAcceptanceEstimatesResponseBody> | null,
  context: EstimateContext,
  lastAttempt: LastAttemptView | null,
): EstimateBlockState {
  if (answer === null) return { kind: 'loading' };
  switch (answer.kind) {
    case 'outcome':
      break;
    case 'unconfigured':
      return { kind: 'unconfigured', text: '暂时读不到受理前估价：接入渠道还没有配置。这不代表没有估价；配置好之后重新打开即可。' };
    case 'loginRequired':
    case 'notGranted':
      return { kind: 'refused', refusal: answer.kind, what: '受理前估价' };
    case 'callerProblem':
      return { kind: 'problem', text: '受理前估价这次没有被受理，问题码见技术详情；运单其余部分不受影响。', technical: answerTechnicalRows(answer) };
    case 'noAnswer':
      return { kind: 'unreachable', title: '受理前估价这次没取到', description: '可以重试；运单其余部分不受影响。', technical: answerTechnicalRows(answer) };
    case 'transport':
      return {
        kind: 'unreachable',
        title: '连不上服务，受理前估价没取到',
        description: '请确认网络与服务在运行后重试。',
        technical: answerTechnicalRows(answer),
      };
  }
  const estimates = (answer.body.estimates ?? []).map(normalisedEstimateOf);
  const sections = estimateSectionsOf(estimates, context);
  const stall = lastAttempt?.financialStall;
  // 续办只跟着停因说：停因不显（不适用、停在别处），续办那句也就无所指。
  const resume = stall && lastAttempt?.resume ? { resume: lastAttempt.resume } : {};
  const notApplicable = context.controlOutcome === 'NOT_APPLICABLE';
  if (sections.length === 0) {
    if (notApplicable) return { kind: 'none', note: `${NO_CONTROL_NOTE}。` };
    return stall ? { kind: 'none', note: `估价没形成，停在：${stall}。`, ...resume } : { kind: 'none', note: '这张运单还没有受理前估价。' };
  }
  if (sections.some((section) => section.isCurrent)) return { kind: 'formed', sections };
  if (notApplicable) return { kind: 'formed', sections, gapNote: `当前版本${NO_CONTROL_NOTE}；下面是此前版本的估价。` };
  return stall
    ? { kind: 'formed', sections, gapNote: `当前版本的估价没形成，停在：${stall}。下面是此前版本的估价。`, ...resume }
    : { kind: 'formed', sections, gapNote: '当前版本还没有估价，下面是此前版本的估价。' };
}
