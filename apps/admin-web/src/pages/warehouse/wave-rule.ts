// 成波规则在管理台上的话（纯 TypeScript，node:test 直接测）：一条规则圈什么、此刻圈得到几张、按它成波之后放出了什么。
//
// 规则是租户按仓登记的取值，圈单的三个条件（货主、发运方式、件型）不带就是不限。按规则成波由人点一下触发：
// 后端照规则圈出已预占的出库单，受理得早的在前，每满规则登的张数放成一个波次；圈到而分配不出的跳过、留在已预占。
// 「此刻圈得到几张」用出库单列表读口照同样的条件问一次总数——它只是点之前的估计，放出多少以后端的答复为准。

import type { CatalogueConditions } from '../catalogue-query';

export interface WaveRuleRecord {
  warehouseCode: string;
  ruleCode: string;
  revision: number;
  name: string;
  /** 只圈这位货主的；不带是不限。 */
  ownerAccount?: string;
  /** 只圈这种发运方式的；不带是不限。 */
  dispatch?: string;
  /** ANY 不限，SINGLE_UNIT 只圈单品单件，MULTI_UNIT 只圈不是单品单件的。 */
  profile: string;
  /** 一个波次最多几张出库单。 */
  maxRequests: number;
  batchPick: boolean;
  deactivated: boolean;
  operator: string;
  registeredAt: string;
}

export const waveRuleProfileOptions = [
  { value: 'ANY', label: '不限件型' },
  { value: 'SINGLE_UNIT', label: '只圈单品单件' },
  { value: 'MULTI_UNIT', label: '只圈不是单品单件的' },
] as const;

const dispatchWords: Record<string, string> = { PRODUCT_NETWORK: '本产品网络', OWNER_LABEL: '货主面单', PICKUP: '到仓自提' };

/** 一条规则圈什么、怎么放，一句话。 */
export function waveRuleSummary(rule: WaveRuleRecord): string {
  const parts: string[] = [];
  if (rule.ownerAccount) parts.push(`货主 ${rule.ownerAccount}`);
  if (rule.dispatch) parts.push(dispatchWords[rule.dispatch] ?? rule.dispatch);
  if (rule.profile === 'SINGLE_UNIT') parts.push('只圈单品单件');
  else if (rule.profile === 'MULTI_UNIT') parts.push('只圈不是单品单件的');
  if (parts.length === 0) parts.push('这个仓已预占的出库单都圈');
  parts.push(`一个波次最多 ${rule.maxRequests} 张`);
  parts.push(rule.batchPick ? '合并拣货' : '逐单拣');
  return parts.join('，');
}

/** 一个仓此刻能用的规则：最新一版没停用的，按规则码排。 */
export function usableWaveRules(rules: readonly WaveRuleRecord[], warehouse: string): WaveRuleRecord[] {
  return rules
    .filter((rule) => rule.warehouseCode === warehouse && !rule.deactivated)
    .sort((left, right) => (left.ruleCode < right.ruleCode ? -1 : left.ruleCode > right.ruleCode ? 1 : 0));
}

/** 有可用规则的仓，按仓编码排；没登记规则的仓不在里面。 */
export function warehousesWithWaveRules(rules: readonly WaveRuleRecord[]): string[] {
  return [...new Set(rules.filter((rule) => !rule.deactivated).map((rule) => rule.warehouseCode))].sort();
}

/** 用出库单列表读口问「这条规则此刻圈得到几张」的条件：与后端圈单的条件逐条对应。 */
export function waveRulePreviewConditions(rule: WaveRuleRecord): CatalogueConditions {
  const filters: Record<string, string[]> = { status: ['RESERVED'], warehouseCode: [rule.warehouseCode] };
  if (rule.ownerAccount) filters.ownerAccount = [rule.ownerAccount];
  if (rule.dispatch) filters.dispatch = [rule.dispatch];
  if (rule.profile === 'SINGLE_UNIT') filters.singleUnit = ['true'];
  else if (rule.profile === 'MULTI_UNIT') filters.singleUnit = ['false'];
  return { filters };
}

/** 一次成波至多处理的出库单数，与后端的上限同数；圈到的比它多时要再点一次。 */
export const maxRequestsPerWaveRuleRun = 500;

/** 点之前的估计：圈得到几张、会放成几个波次。总数读不出（null）时如实说读不出。 */
export function waveRulePreviewText(rule: WaveRuleRecord, total: number | null): string {
  if (total === null) return '没读出此刻圈得到几张；照样可以成波，放出多少以答复为准。';
  if (total === 0) return '此刻没有圈得到的出库单：这个仓里没有合条件的已预占的单。';
  const handled = Math.min(total, maxRequestsPerWaveRuleRun);
  const waves = Math.ceil(handled / rule.maxRequests);
  const more = total > handled ? `一次最多处理 ${maxRequestsPerWaveRuleRun} 张，剩下的 ${total - handled} 张要再点一次。` : '';
  return `此刻圈得到 ${total} 张已预占的出库单，全都分配得出的话放成 ${waves} 个波次。${more}分配不出的单会跳过、留在已预占。`;
}

/** 一次执行的号：规则码加到秒的时刻。放出的波次按它依次取号，同一个号只成立一次。 */
export function mintWaveRuleRunId(ruleCode: string, now: Date = new Date()): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  return (
    `${ruleCode}-${now.getUTCFullYear()}${pad(now.getUTCMonth() + 1)}${pad(now.getUTCDate())}` +
    `-${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}${pad(now.getUTCSeconds())}`
  );
}

export interface WaveRuleRunBody {
  outcome: string;
  ruleRevision?: number;
  waves?: ReadonlyArray<{ waveId: string; requests: number }>;
  /** 放进波次的出库单数；圈到了而分配不出、留在已预占的；超过一次上限没轮到的。 */
  released?: number;
  skipped?: number;
  remaining?: number;
}

export interface WaveRuleRunNote {
  /** 放出了波次（这一次，或同一个执行号更早的那一次）。 */
  released: boolean;
  tone: 'success' | 'warning' | 'danger';
  text: string;
}

const refusalWords: Record<string, string> = {
  WAVE_RULE_NOT_FOUND: '这个仓没有登记过这条规则，没有成波。',
  WAVE_RULE_DEACTIVATED: '这条规则已经停用，没有成波。要用它，先到「仓配规则设置」登一版启用的。',
  NOT_CONFIGURED: '未配置：还没有采用分配策略，放不了波次。先到「仓配规则设置」采用。',
  CONTENT_CONFLICT: '这一次的执行号和已有的波次或另一次成波撞了，整次没有成立，一个波次都没放。关掉再点一次会换一个号。',
  CONTENT_INVALID: '后端不认这一次的内容，没有成波。',
};

/** 后端答了之后屏上的话。跳过与没轮到的如实带出：人要照它决定去看哪几张单、要不要再点一次。 */
export function waveRuleRunNote(body: WaveRuleRunBody): WaveRuleRunNote {
  const skipped = body.skipped ?? 0;
  const remaining = body.remaining ?? 0;
  const skippedText =
    skipped > 0 ? `有 ${skipped} 张圈到了但分配不出，留在已预占：多半是货不在可拣的库位，或少拣的库位还没盘过。出库单列表按「放不出」筛得出来。` : '';
  const remainingText = remaining > 0 ? `还有 ${remaining} 张这一次没轮到，再点一次接着放。` : '';
  if (body.outcome === 'ALLOCATED' || body.outcome === 'EXISTING_RESULT') {
    const waves = body.waves ?? [];
    const head =
      body.outcome === 'ALLOCATED'
        ? `放出了 ${waves.length} 个波次，共 ${body.released ?? 0} 张出库单。`
        : `这一次之前已经成过了（同一个执行号），没有重复放：当时放出 ${waves.length} 个波次，共 ${body.released ?? 0} 张出库单。`;
    return { released: true, tone: skipped > 0 || remaining > 0 ? 'warning' : 'success', text: `${head}${skippedText}${remainingText}` };
  }
  if (body.outcome === 'NOTHING_TO_RELEASE') {
    return {
      released: false,
      tone: 'warning',
      text: skipped > 0 ? `一个波次也没放出。${skippedText}` : '没有圈到合条件的已预占的出库单，一个波次也没放出。',
    };
  }
  return { released: false, tone: 'danger', text: refusalWords[body.outcome] ?? `后端答了一个这里不认识的结果（${body.outcome}），没有当成成波。` };
}

export interface AllocationExceptionRecord {
  /** WAVE 是由人放的波次（勾了「放行能放的」），RULE 是按规则成波的一次执行。 */
  source: string;
  /** 波次号或执行号。 */
  reference: string;
  recordedAt: string;
}

/** 出库单列表行上「放不出」那一句：短词摆在行上，整句悬停时看。没有例外为 null。 */
export function allocationExceptionNote(exception: AllocationExceptionRecord | undefined): { label: string; detail: string } | null {
  if (exception === undefined) return null;
  const when = exception.source === 'RULE' ? `按规则成波（${exception.reference}）` : `放波次 ${exception.reference}`;
  return {
    label: '上次放不出',
    detail: `${when}时这张单分配不出，被跳过了：多半是货不在可拣的库位，或少拣的库位还没盘过。先看它的货在哪个库位，补货或盘点之后再放。`,
  };
}

const releaseExceptionWords: Record<string, string> = {
  NOT_ALLOCATABLE: '分配不出',
  NOT_RESERVED: '不是已预占',
  NOT_FOUND: '没有这张单，或不属于这个仓',
};

/** 放波次勾了「放行能放的」之后，答复里没放进波次的那几张，说成一句。没有就是空串。 */
export function releaseExceptionsText(exceptions: ReadonlyArray<{ requestId: string; reason: string }> | undefined): string {
  if (exceptions === undefined || exceptions.length === 0) return '';
  const named = exceptions.map((exception) => `${exception.requestId}（${releaseExceptionWords[exception.reason] ?? exception.reason}）`);
  return `没放进波次的 ${exceptions.length} 张：${named.join('、')}。分配不出的留在已预占，出库单列表按「放不出」筛得出来`;
}

/** 登记下一版时该填的修订号：这条规则没登过是 1。 */
export function nextWaveRuleRevision(rules: readonly WaveRuleRecord[], warehouse: string, ruleCode: string): number {
  const current = rules.find((rule) => rule.warehouseCode === warehouse && rule.ruleCode === ruleCode);
  return current === undefined ? 1 : current.revision + 1;
}
