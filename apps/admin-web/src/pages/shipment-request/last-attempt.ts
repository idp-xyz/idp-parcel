import { refPageHash } from '../party/ref-links';
import type { AcceptanceTaskRecord } from './api';
import { settlementReasonOf, stallReasonOf } from './stall-reasons';

// 受理任务「最近一次没能推进」那一条在运单详情上怎么说（ADR-0380 决定五，票 declared-goods-lines/24）：停因带上结算的细原因与
// 这一版申报的服务产品，再说谁来续办。「受理进度」与「受理前财务控制」读同一份，免得一处说没有在用价卡、另一处只说缺登记。
// 纯函数，node:test 直接钉。

/** 续办方，领域 ResumePath 的全部原词；与 Go 源对得上由 stall-reasons.contract.test.ts 核。 */
export const knownResumePaths = [
  'CUSTOMER_SUPPLEMENT',
  'INTERNAL_RETRY',
  'MANUAL_REVIEW',
  'OPERATOR_REGISTRATION',
  'AUTHORIZED_DISPOSITION',
] as const;

export type ResumePath = (typeof knownResumePaths)[number];

export interface ResumeNote {
  path: ResumePath;
  text: string;
  link?: { href: string; label: string };
}

export interface LastAttemptView {
  /** 「受理进度」里最近处理结果那一句。 */
  text: string;
  /** 停在财务控制那一步时接在「停在：」后面的那截，不重复「估价没形成」；停在别处缺席。 */
  financialStall?: string;
  /** 谁来续办；说不出或不必另说时缺席，判法见 resumeNoteOf。 */
  resume?: ResumeNote;
}

// 等运营登记那一族停因的句尾，照 stall-reasons.ts 词表的写法。续办另起一句时，它与「…登记后自动续办」说的是同一件事。
const REGISTRATION_WAIT_TAIL = '，等运营登记';

function isResumePath(word: string | undefined): word is ResumePath {
  return (knownResumePaths as readonly string[]).includes(word ?? '');
}

/**
 * 续办方只读处理尝试落库的那一格，不从停因推：停因说的是停在哪，续办方是同一次尝试另记的事实，推出来的一旦与记下的不一致，
 * 就是催错人。缺席或认不得的词不说——宁可少一句，也不猜。等客户补充照旧由补充缺口那几行说缺什么，不另起一句。
 *
 * 任务不在运行（已完成或已停止）时也不说：最近一次尝试还留着，但那份等待已经过去，再说「登记后自动续办」就是叫人去做一件不再
 * 有用的事。
 */
function resumeNoteOf(task: AcceptanceTaskRecord, priceCardSide: boolean): ResumeNote | undefined {
  const path = task.lastAttemptResumePath;
  if (task.state !== 'RUNNING' || !isResumePath(path)) return undefined;
  switch (path) {
    case 'OPERATOR_REGISTRATION':
      // 估价按提交的接收时点取卡：只从现在起生效的卡解不开这一单，所以要写明生效期得覆盖提交时点。别的登记（授权规则、财务
      // 控制策略、估价采用）不在价格表里，指过去是指错地方。
      return priceCardSide
        ? {
            path,
            text: '登记一张生效期覆盖提交时点的价卡后自动续办',
            link: { href: refPageHash('price-card'), label: '去价格表' },
          }
        : { path, text: '运营登记后自动续办' };
    case 'INTERNAL_RETRY':
      return { path, text: '系统会自动重试' };
    case 'MANUAL_REVIEW':
      return { path, text: '复核角色记下复核后续办' };
    case 'AUTHORIZED_DISPOSITION':
      return { path, text: '授权处置角色选定去向后续办' };
    case 'CUSTOMER_SUPPLEMENT':
      return undefined;
  }
}

/**
 * 没有处理尝试（两格都缺席或是空串）时答 null。`resume: false` 给不另起续办一行的页：停因整句说全，不带续办那一截。
 *
 * 服务产品只接在财务控制那一步的停因上：它说的是哪个产品估不出价，接到等客户补充、等人工复核上只是噪声，那几种的写法照旧
 * （ADR-0380 决定五）。
 */
export function lastAttemptOf(task: AcceptanceTaskRecord, options: { resume?: boolean } = {}): LastAttemptView | null {
  const withResume = options.resume !== false;
  const product = task.lastAttemptServiceProduct ? `（服务产品 ${task.lastAttemptServiceProduct}）` : '';
  if (task.lastAttemptSettlementReason) {
    const settlement = settlementReasonOf(task.lastAttemptSettlementReason);
    const detail = `${settlement.text}${product}`;
    const view: LastAttemptView = {
      text: `${settlement.estimate ? '估价没形成' : '财务控制没形成'}：${detail}`,
      financialStall: detail,
    };
    const resume = withResume ? resumeNoteOf(task, settlement.priceCardSide) : undefined;
    return resume ? { ...view, resume } : view;
  }
  if (!task.lastAttemptReason) return null;
  const stall = stallReasonOf(task.lastAttemptReason);
  const resume = withResume ? resumeNoteOf(task, false) : undefined;
  const reasonText =
    resume?.path === 'OPERATOR_REGISTRATION' && stall.text.endsWith(REGISTRATION_WAIT_TAIL)
      ? stall.text.slice(0, -REGISTRATION_WAIT_TAIL.length)
      : stall.text;
  if (!stall.financialControl) return resume ? { text: reasonText, resume } : { text: reasonText };
  const text = `${reasonText}${product}`;
  return resume ? { text, financialStall: text, resume } : { text, financialStall: text };
}
