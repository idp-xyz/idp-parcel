/**
 * 部分发货规则：一张出库请求拣不齐时货主愿意怎么发。词表与后端 `domain.PartialShipment` 同一批。
 *
 * 三种都是货主的意思，页面不替他选：哪一层都没写明就答「未配置」，不摆其中任何一种当默认。
 */

export const partialShipmentOptions = [
  { value: 'SHIP_COMPLETE', label: '整单发：拣不齐就不发' },
  { value: 'SHORT_SHIP', label: '少发：拣到的发走，缺的不再发' },
  { value: 'SPLIT_SHIP', label: '拆单发：拣到的先发，缺的拆成新单' },
] as const;

const ruleLabels: Record<string, string> = {
  SHIP_COMPLETE: '整单发',
  SHORT_SHIP: '少发',
  SPLIT_SHIP: '拆单发',
};

/** 出处的词与后端 `domain.PartialShipmentSource` 同一批：越贴近这一张单、越晚说的越优先。 */
const sourceLabels: Record<string, string> = {
  ORDER_CONSENT: '受理后代录的货主同意',
  ORDER: '这张单提交时写明',
  OWNER: '货主登记的默认',
};

export function partialShipmentLabel(rule: string | undefined): string {
  if (!rule) return '未配置';
  return ruleLabels[rule] ?? rule;
}

export function partialShipmentSourceLabel(source: string | undefined): string {
  if (!source) return '';
  return sourceLabels[source] ?? source;
}

/** 明细读口答的那一格。各层自己写的照实带出，没写的不带。 */
export interface PartialShipmentAnswer {
  configured: boolean;
  rule?: string;
  source?: string;
  order?: string;
  owner?: string;
  consent?: { rule: string; basis: string; operator: string; recordedAt?: string };
}

/**
 * 一张单的部分发货怎么说。`layers` 是被盖过的那几层：适用的是代录的同意时，单上原来写的与货主默认也摆出来，
 * 让人看得出这一张为什么与别的单不一样。读口没答这一格（旧后端）时不猜，回 `null`。
 */
export function partialShipmentSummary(
  answer: PartialShipmentAnswer | undefined,
): { configured: boolean; headline: string; source: string; layers: string[] } | null {
  if (!answer) return null;
  if (!answer.configured || !answer.rule) {
    return {
      configured: false,
      headline: '未配置',
      source: '这张单没写明，货主也没登记默认；拣不齐时不能少发，也不能拆出缺货部分',
      layers: [],
    };
  }
  const layers: string[] = [];
  if (answer.source !== 'ORDER' && answer.order) layers.push(`提交时写明 ${partialShipmentLabel(answer.order)}`);
  if (answer.source !== 'OWNER' && answer.owner) layers.push(`货主默认 ${partialShipmentLabel(answer.owner)}`);
  return {
    configured: true,
    headline: partialShipmentLabel(answer.rule),
    source: partialShipmentSourceLabel(answer.source),
    layers,
  };
}

/** 已经出库、取消、拒收、拆开或交给别处履行的单不再收代录的同意，与后端 `RecordConsent` 拒的那几种同一批。 */
const settledStatuses = new Set(['SHIPPED', 'CANCELLED', 'REJECTED', 'SPLIT', 'HANDED_OFF', 'FORWARDED', 'AWAITING_RETURN']);

export function canRecordPartialShipmentConsent(status: string): boolean {
  return !settledStatuses.has(status);
}

/** 明细读口答的拣不齐处置：照的规则与出处、要发与实发、谁在何时办的。 */
export interface ShortPickDisposalAnswer {
  rule: string;
  source: string;
  wanted: number;
  shipped: number;
  remainderRequestId?: string;
  operator: string;
  recordedAt: string;
}

/**
 * 一张单是不是停在「拣不齐、等处置」：已预占，任务都有了结果（没有一条待执行），并且已有货拣进了待出库。
 * 口径同后端 `AwaitsShortPickDisposal`；`picked` 只数拣满与少拣的任务。
 */
export function awaitsShortPickDisposal(status: string, tasks: ReadonlyArray<{ status: string }>, picked: number): boolean {
  if (status !== 'RESERVED') return false;
  if (tasks.some((task) => task.status === 'OPEN')) return false;
  return picked > 0;
}

/**
 * 停在「拣不齐、等处置」的单此刻能办什么。少发还是拆单发照货主的规则，页面只摆货主许了的那一个动作；
 * 不许或没写明时说清为什么不摆、还能怎么办，不让人去猜。读口没答规则（旧后端）时不摆动作。
 * `picked` 与 `wanted` 都按基本单位：`wanted` 取读口的预占需求合计，不取明细行合计（明细行按下单单位写）。
 */
export function shortPickDisposalChoice(
  answer: PartialShipmentAnswer | undefined,
  picked: number,
  wanted: number,
  prefetched: boolean,
): { action: 'short-shipment' | 'shortage-split' | null; note: string } {
  const gap = `拣到 ${picked}，要发 ${wanted}，少的 ${Math.max(0, wanted - picked)} 换不出别的库位。`;
  // 预取过估算委托的单照办：委托按全量申报了件数，打包时后端按实际装的件数提货物明细修订，面单号不变。
  // 早先预取、没留下申报件数的那几张后端不办，答 `PREFETCH_STANDS`，由答复说清——页面上分不出是哪一种，不先拦。
  const amended = prefetched ? '这张单放波次时预取过面单：打包时会按实际装的件数更正委托上的申报件数，面单号不变。' : '';
  switch (answer?.configured ? answer.rule : undefined) {
    case 'SHORT_SHIP':
      return { action: 'short-shipment', note: `${gap}货主许了少发，可以按已拣数量发货；也可以等有货了再放波次。${amended}` };
    case 'SHIP_COMPLETE':
      return { action: null, note: `${gap}货主要整单发：等有货了再放波次，或截单。` };
    case 'SPLIT_SHIP':
      return {
        action: 'shortage-split',
        note: `${gap}货主许了拆单发，可以拆出缺货部分：拣到的先发，缺的另成一张单继续等货；也可以等有货了再放波次。${amended}`,
      };
    default:
      return {
        action: null,
        note: `${gap}货主没写明拣不齐时怎么发：等有货了再放波次、截单，或问过货主之后代录部分发货同意。`,
      };
  }
}

/** 办过的那一次处置的一句话。 */
export function shortPickDisposalLine(disposal: ShortPickDisposalAnswer): string {
  const base = `要发 ${disposal.wanted}，实发 ${disposal.shipped}，照${partialShipmentSourceLabel(disposal.source)}`;
  return disposal.rule === 'SPLIT_SHIP' ? `已拆出缺货部分：${base}` : `已按已拣数量发货：${base}`;
}

/**
 * 代录拆单只给还没有货拣出来的单：已有货拣进待出库的单拆开会整张作废，拣出来的货就没了归属。
 * 与后端 `Split` 挡的是同一种；那种单拣不齐时走上面两个处置，或截单。
 */
export function canSplitUnallocated(status: string, picked: number): boolean {
  return (status === 'RESERVED' || status === 'PARTIALLY_RESERVED' || status === 'PENDING_POLICY') && picked === 0;
}
