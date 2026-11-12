import { refPageHash } from '../party/ref-links';
import type { AcceptanceTaskRecord } from './api';
import { estimateGapCardSideWords, settlementReasonOf, stallReasonOf } from './stall-reasons';
import { supplementGapLinesOf } from './supplement-gaps';

// 受理任务「最近一次没能推进」那一条在运单详情上怎么说（ADR-0380 决定五，票 declared-goods-lines/24）：停因带上结算的细原因与
// 这一版申报的服务产品，再说谁来续办。「受理进度」与「受理前财务控制」读同一份，免得一处说没有在用价卡、另一处只说缺登记。
// 申报不全的估价（ADR-0381 决定五，票 declared-goods-lines/27）再按缺项该谁补说细：客户可补的列缺口清单、卡侧缺项写明这张卡
// 做不了受理前估价与该登什么。纯函数，node:test 直接钉。

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
function resumeNoteOf(task: AcceptanceTaskRecord, priceCardSide: boolean, cardRegistration?: string): ResumeNote | undefined {
  const path = task.lastAttemptResumePath;
  if (task.state !== 'RUNNING' || !isResumePath(path)) return undefined;
  switch (path) {
    case 'OPERATOR_REGISTRATION':
      // 估价按提交的接收时点取卡：只从现在起生效的卡解不开这一单，所以要写明生效期得覆盖提交时点。别的登记（授权规则、财务
      // 控制策略、估价采用）不在价格表里，指过去是指错地方。
      // 卡侧缺项（ADR-0381 决定一）另说该登什么样的卡：光说「生效期覆盖提交时点」会让人登一张照样解不开它的卡。
      if (cardRegistration !== undefined)
        return {
          path,
          text: `登记一张${cardRegistration}、生效期覆盖提交时点的售价卡后自动续办`,
          link: { href: refPageHash('price-card'), label: '去价格表' },
        };
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

// 受理前财务控制那一步的补件缺口归的判组（ADR-0381 决定四）；别组的缺口（必要资料、可达性）不混进估价这一句。
const FINANCIAL_CONTROL_GAP_GROUP = 'PRE_ACCEPTANCE_FINANCIAL_CONTROL';

interface DeclarationIncompleteView {
  /** 接在「估价没形成：」后面的那截（不含服务产品）。 */
  text: string;
  /** 等客户补充时接在句尾的「——客户补充后自动续办」；卡侧缺项不带（那一句由续办那一格说）。 */
  tail: string;
  /** 卡侧缺项时该登什么样的卡（接在「登记一张」后面）；客户可补的缺席。 */
  registration?: string;
}

/**
 * 申报不全（结算细原因 ESTIMATE_DECLARATION_INCOMPLETE，ADR-0381 决定五）怎么说细。缺项码读处理尝试那一格
 * （lastAttemptEstimateGapCodes），含卡侧两码（ZONE / ADDRESS_NATURE）就是这张卡做不了受理前估价——写明卡缺什么与该登什么，
 * 不催客户补一件白补的事；否则等客户补充时逐条列任务上的补件缺口（指件的带件、委托级的写「委托缺…」），句尾说客户补充后
 * 自动续办。两格都说不出细的（如照旧内部重试那一路）答 undefined，走底句。
 */
function declarationIncompleteViewOf(task: AcceptanceTaskRecord): DeclarationIncompleteView | undefined {
  const gapCodes = task.lastAttemptEstimateGapCodes ?? [];
  const cardSide = Object.entries(estimateGapCardSideWords).filter(([code]) => gapCodes.includes(code));
  if (cardSide.length > 0) {
    return {
      text: `在用的卡${cardSide.map(([, words]) => words.wants).join('、')}，受理前估价给不出`,
      tail: '',
      registration: cardSide.map(([, words]) => words.registration).join('、'),
    };
  }
  if (task.lastAttemptResumePath !== 'CUSTOMER_SUPPLEMENT') return undefined;
  const gaps = supplementGapLinesOf((task.supplementGaps ?? []).filter((gap) => gap.group === FINANCIAL_CONTROL_GAP_GROUP))
    .map((line) => (line.parcelId ? `${line.parcelId} 缺${line.missing}` : `委托缺${line.missing}`))
    .join('、');
  return { text: `申报缺了计价要的资料${gaps ? `（${gaps}）` : ''}`, tail: '——客户补充后自动续办' };
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
    const declaration = settlement.code === 'ESTIMATE_DECLARATION_INCOMPLETE' ? declarationIncompleteViewOf(task) : undefined;
    const detail = `${declaration?.text ?? settlement.text}${product}${declaration?.tail ?? ''}`;
    const view: LastAttemptView = {
      text: `${settlement.estimate ? '估价没形成' : '财务控制没形成'}：${detail}`,
      financialStall: detail,
    };
    const resume = withResume ? resumeNoteOf(task, settlement.priceCardSide, declaration?.registration) : undefined;
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
