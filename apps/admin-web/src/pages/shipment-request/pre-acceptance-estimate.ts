import type { TechnicalRow } from '../../components/TechnicalDetails';
import { operatorRefusalMessage } from '../../auth/access-refusal';
import { chargeCodeLabel } from '../../vocabulary/status';
import type { ApiResult } from '../catalogue-api';
import type {
  PreAcceptanceEstimateRecord,
  PreAcceptanceEstimatesResponseBody,
  PreAcceptanceOccupationRecord,
  PreAcceptanceParcelEstimateRecord,
} from '../settlement/api';
import { formatMoney, isPositiveMinorAmount } from '../settlement/money';
import { stallReasonOf } from './stall-reasons';
import { answerTechnicalRows } from './words';

// 运单详情「受理前财务控制」一块里估价那一半的判读（ADR-0379 决定四，票 declared-goods-lines/21）。纯函数，node:test 直接钉；
// React 只在 PreAcceptanceFinancialControl.tsx。
//
// 估价读的是结算的读口，结论读的是委托详情：两侧各交各的、页面拼，所以这一半有自己的加载、出错与空态——结算那一口读不到，
// 运单详情与结论照样在。
//
// 金额两种写法分开处理：合计、逐件小计与占用金额是最小币单位整数串，走费用页同一个 formatMoney；费用行是试算当时算出的
// 主单位十进制串，照原样带它自己的币种，不能再过一遍最小币单位的写法。

export interface ChargeLineView {
  code: string;
  label: string;
  effect: string;
  amount: string;
}

export interface ParcelEstimateView {
  parcel: string;
  subtotal: string;
  /** 补记之前的旧行不出这一格；补记之后没记下来答「未记」。 */
  weight: string;
  /** null 是没记下费用行（与「记了、零行」分开）。 */
  charges: ChargeLineView[] | null;
}

export interface OccupationView {
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

export type EstimateBlockState =
  | { kind: 'loading' }
  | { kind: 'failed'; text: string; retryable: boolean; technical: TechnicalRow[] }
  | { kind: 'none'; note: string }
  | { kind: 'formed'; sections: EstimateSectionView[]; gapNote?: string };

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
  // 部分转扣费的占用状态不变（还占着余下那部分），只靠已扣金额看得出来；整笔扣完是 CONVERTED，不再加这一句。
  const stillHeld = occupation.status === 'HELD' || occupation.status === 'RECORDED';
  const baseStatus = statusWords[occupation.status] ?? occupation.status;
  const status = converted && stillHeld ? `${baseStatus}，部分已扣费` : baseStatus;
  const amount = formatMoney(occupation.amount, occupation.currency);
  const times = [{ label: occupationFormedAtLabels[occupation.kind] ?? '占用时刻', at: occupation.formedAt }];
  if (occupation.releasedAt) times.push({ label: '释放时刻', at: occupation.releasedAt });
  if (occupation.convertedAt) times.push({ label: '扣费时刻', at: occupation.convertedAt });
  const summary = [kind, status, amount, ...(converted ? [`已扣 ${converted}`] : [])].join(' · ');
  return { kind, status, amount, ...(converted ? { converted } : {}), times, summary };
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

export function estimateSectionOf(estimate: PreAcceptanceEstimateRecord, currentVersion: string): EstimateSectionView {
  const isCurrent = estimate.submissionVersion === currentVersion;
  const occupations = (estimate.occupations ?? []).map(occupationViewOf);
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
    // 空数组是控制受限、没有占用；不写「0」冒充占了零。
    ...(occupations.length === 0 ? { occupationNote: '没有占用：控制受限，没冻结资金也没占用信用' } : {}),
    technical: sectionTechnicalRows(estimate),
  };
}

/** 逐提交版本一段，次序照读口（按试算时点）。 */
export function estimateSectionsOf(estimates: readonly PreAcceptanceEstimateRecord[], currentVersion: string): EstimateSectionView[] {
  return estimates.map((estimate) => estimateSectionOf(estimate, currentVersion));
}

/** 受理任务最近一次停在财务控制那一步时的中文停因；停在别处或没停过答 undefined。 */
function financialStallOf(lastAttemptReason: string | undefined): string | undefined {
  if (!lastAttemptReason) return undefined;
  const stall = stallReasonOf(lastAttemptReason);
  return stall.financialControl ? stall.text : undefined;
}

function failedStateOf(answer: Exclude<ApiResult<PreAcceptanceEstimatesResponseBody>, { kind: 'outcome' }>): EstimateBlockState {
  const technical = answerTechnicalRows(answer);
  switch (answer.kind) {
    case 'unconfigured':
      return { kind: 'failed', text: '结算读口的接入渠道还没配置，估价显示不了；这不是故障，重试不会改变结果。', retryable: false, technical };
    case 'loginRequired':
    case 'notGranted':
      return { kind: 'failed', text: operatorRefusalMessage(answer.kind, '受理前估价没读到'), retryable: false, technical };
    case 'callerProblem':
      return { kind: 'failed', text: '受理前估价这次没有被受理，问题码见技术详情；运单其余部分不受影响。', retryable: false, technical };
    case 'noAnswer':
      return { kind: 'failed', text: '受理前估价这次没取到，可以重试。', retryable: true, technical };
    case 'transport':
      return { kind: 'failed', text: '连不上服务，受理前估价没取到；请确认网络与服务在运行后重试。', retryable: true, technical };
  }
}

/**
 * 估价那一半显什么。没有留痕时，若受理任务最近停在财务控制那一步，就把停因说出来——估价没形成的原因在受理任务上，结算不另存
 * （ADR-0379 决定三）。有留痕而没有当前版本的那一份（换了版本、新版本还停着），同样说一句，免得把旧版本的估价读成现在的。
 */
export function estimateBlockStateOf(
  answer: ApiResult<PreAcceptanceEstimatesResponseBody> | null,
  currentVersion: string,
  lastAttemptReason: string | undefined,
): EstimateBlockState {
  if (answer === null) return { kind: 'loading' };
  if (answer.kind !== 'outcome') return failedStateOf(answer);
  const stall = financialStallOf(lastAttemptReason);
  const sections = estimateSectionsOf(answer.body.estimates ?? [], currentVersion);
  if (sections.length === 0) {
    return { kind: 'none', note: stall ? `估价没形成，停在：${stall}。` : '这张运单还没有受理前估价。' };
  }
  if (sections.some((section) => section.isCurrent)) return { kind: 'formed', sections };
  const gapNote = stall
    ? `当前版本的估价没形成，停在：${stall}。下面是此前版本的估价。`
    : '当前版本还没有估价，下面是此前版本的估价。';
  return { kind: 'formed', sections, gapNote };
}
