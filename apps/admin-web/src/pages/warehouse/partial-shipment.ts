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
