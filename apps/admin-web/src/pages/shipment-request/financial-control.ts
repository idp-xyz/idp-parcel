import { labelFor } from '../../vocabulary/status';
import type { ReviewControlItemRecord, ReviewFinancialControlRecord } from './api';
import { controlFailureDispositionLabels } from './presentation';

// 接受前财务控制结论的屏上写法。委托详情与复核队列交的是同一份结论、同一个编码函数（票 declared-goods-lines/20），控制项的
// 写法也就只留一处。纯函数，node:test 直接钉。
//
// 受限原因是结算两本账在受限时写下的限制原因码（冻结那本写余额不足，信用那本写额度不足或逾期），小包托运原样转交在受限项的
// basis 上。认得的译成中文，认不得的照原码——页面落后于服务端时宁可露出原码，也不把它并进某个认得的说法。

const restrictionReasonWords: Record<string, string> = {
  AVAILABLE_BALANCE_INSUFFICIENT: '可用余额不足',
  AVAILABLE_CREDIT_INSUFFICIENT: '可用额度不足',
  ACCOUNT_OVERDUE: '账户逾期',
};

/** 词表收录的限制原因码，给契约测试对着 Go 源核。 */
export const knownRestrictionReasonCodes: readonly string[] = Object.keys(restrictionReasonWords);

export function restrictionReasonText(code: string): string {
  return restrictionReasonWords[code] ?? code;
}

/**
 * 一项控制的一行：种类与结论经词典译；受限项上的两格采用引用（正文登记的失败处置与责任引用，ADR-0132 决定三、四）
 * 有才显——成立项与采用之前记下的受限项两格缺席，不写「无」冒充登记过。
 *
 * withReason 才带受限原因：不带时是复核队列一向的写法，原因码在那一页的技术详情里。
 */
export function controlItemText(item: ReviewControlItemRecord, options: { withReason?: boolean } = {}): string {
  const parts = [`${labelFor(item.kind)}:${labelFor(item.conclusion)}`];
  if (options.withReason && item.conclusion === 'RESTRICTED') {
    // 受限项按构造必有原因；真缺了就照实说没给，不留一个看起来像通过的空位。
    parts.push(item.basis ? restrictionReasonText(item.basis) : '没给原因');
  }
  if (item.failureDisposition) {
    parts.push(`不通过时${controlFailureDispositionLabels[item.failureDisposition] ?? item.failureDisposition}`);
  }
  if (item.responsibility) parts.push(`责任方 ${item.responsibility}`);
  return parts.join(' · ');
}

export type ConclusionTone = 'good' | 'warn' | 'muted';

/** 运单详情上的结论：一个定性（通过、受限、不适用）加一句说是哪一种，逐项一行。 */
export interface FinancialControlConclusion {
  verdict: string;
  detail: string;
  tone: ConclusionTone;
  /** key 是种类与判断次序，同一份结论里不重。 */
  items: { key: string; text: string }[];
}

// 四种结论的说法。HELD 与 CREDIT_EXPOSED 都是通过，差在占的是钱还是信用；NOT_APPLICABLE 是合同约定这户不做财务控制，
// 不是「通过了」，单列一格，免得读成查过且满足。
const conclusionWords: Record<string, Omit<FinancialControlConclusion, 'items'>> = {
  HELD: { verdict: '通过', detail: '已冻结资金', tone: 'good' },
  CREDIT_EXPOSED: { verdict: '通过', detail: '已占用信用', tone: 'good' },
  RESTRICTED: { verdict: '受限', detail: '有控制项没通过，原因逐项列在下面', tone: 'warn' },
  NOT_APPLICABLE: { verdict: '不适用', detail: '合同约定无财务控制', tone: 'muted' },
};

/** 委托详情的 financialControl 摊成屏上的结论；没判过答 null，页面照实写「还没判」。认不得的结论照原码。 */
export function financialControlConclusionOf(control: ReviewFinancialControlRecord | undefined): FinancialControlConclusion | null {
  if (!control) return null;
  const words = conclusionWords[control.outcome] ?? { verdict: control.outcome, detail: '', tone: 'muted' as const };
  return {
    ...words,
    items: control.items.map((item) => ({ key: `${item.kind}:${item.order}`, text: controlItemText(item, { withReason: true }) })),
  };
}
