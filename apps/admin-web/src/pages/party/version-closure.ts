// 旧版收尾的纯逻辑（票 party-commercial-uiux/16，ADR-0233）。
//
// 换版不收口：同一对象另发一版，旧版仍是「在用」，两版区间重叠的部分解析会答「适用冲突」，直到登记方显式收尾。收尾有两种，
// 由旧版与后继此刻的起点先后决定，服务端各开一口、不替登记方挑：
//   - 后继晚于旧版起点：截止，旧版只在后继生效时点之前照旧可选（落一条区间更正，截止点取后继的生效时点，不由人填）；
//   - 后继与旧版同一起点：退役，旧版整版不再参与新的选择（退役时刻取登记时刻，不由人填）。
//
// 这里做的事：从重读的册面里认出旧版与可让位的后继、把草稿编成载荷、把确认框与答复要说的话拼出来。两版在不在册、都生效
// 没有、后继接不接得住旧版此后那一段，仍由服务端按整册判；前端的筛选只为不让人对着显然收不了的组合填表，服务端的门一道不少。
//
// 只开读面带状态的册（服务产品、客户合同、供应商协议、授权规则、接受前财务控制策略、客户服务规则）：状态是判「旧版是否在用」的
// 唯一依据，读面没有状态的册看不出哪一版还在参选，也就列不出可收尾的旧版。
// 纯函数，node:test 直接跑。

import type { VersionClosureMode, VersionClosureResponseBody } from './api';
import { formatInstant } from '../moment';
import { fieldProblemText, type FieldProblemText } from './field-problem-text';
import type { KnownVersion } from './known-versions';
import { wallTimeOf } from './party-relationship-end-form';
import { wallMomentOf, wallMomentProblem } from './publication-form-shared';

/** 服务端 `CommercialObjectKind` 名称中读面带状态的那几类。 */
export type ClosureObjectKind =
  | 'SERVICE_PRODUCT'
  | 'CUSTOMER_CONTRACT'
  | 'SUPPLIER_AGREEMENT'
  | 'AUTHORIZATION_RULE'
  | 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY'
  | 'CUSTOMER_SERVICE_RULE';

/**
 * 行上点中的那一版。只认「哪一类、哪一对象、哪一版、哪个范围」：起点、终点、状态以抽屉打开时重读的册面为准，不拿行上
 * 可能已陈旧的值去判。
 */
export interface ClosureTarget {
  kind: ClosureObjectKind;
  /** 例：「客户合同」。 */
  kindLabel: string;
  objectId: string;
  version: string;
  scope: string;
}

const inUse = 'EFFECTIVE';

const versionOrder = new Intl.Collator('en', { numeric: true, sensitivity: 'base' });

function startOf(row: KnownVersion): number {
  return Date.parse(row.effectiveStartsAt);
}

function endOf(row: KnownVersion): number {
  return row.effectiveEndsAt === undefined ? Number.POSITIVE_INFINITY : Date.parse(row.effectiveEndsAt);
}

export function closureTargetOf(
  kind: ClosureObjectKind,
  kindLabel: string,
  row: Pick<ClosureTarget, 'objectId' | 'version' | 'scope'>,
): ClosureTarget {
  return { kind, kindLabel, objectId: row.objectId, version: row.version, scope: row.scope };
}

/** 重读的册面里与目标同一对象、同一范围、同一版号的那一行；已不在册面上即 null。 */
export function closureOldOf(all: readonly KnownVersion[], target: ClosureTarget): KnownVersion | null {
  return (
    all.find((row) => row.objectId === target.objectId && row.scope === target.scope && row.version === target.version) ??
    null
  );
}

/**
 * 能让旧版让位的后继：同一对象、同一范围、在用、起点不早于旧版，且起点落在旧版此刻的选用区间之内。起点早于旧版的不是换版；
 * 起点不早于旧版终点的两版本来不重叠，无从收尾。晚于旧版起点的走截止，同起点的走退役。起点近的在前，同起点新版号在前。
 */
export function closureSuccessors(old: KnownVersion, all: readonly KnownVersion[]): KnownVersion[] {
  const oldStart = startOf(old);
  const oldEnd = endOf(old);
  return all
    .filter((row) => {
      if (row.objectId !== old.objectId || row.scope !== old.scope || row.version === old.version) return false;
      if (row.status !== inUse) return false;
      const start = startOf(row);
      return !Number.isNaN(start) && start >= oldStart && start < oldEnd;
    })
    .sort((a, b) => startOf(a) - startOf(b) || versionOrder.compare(b.version, a.version));
}

/** 一版在册面上的身份键，页面拿它认「这一行能不能收尾」。 */
export function closureKeyOf(row: Pick<KnownVersion, 'scope' | 'objectId' | 'version'>): string {
  return `${row.scope ?? ''}\u0000${row.objectId}\u0000${row.version}`;
}

/**
 * 已读到的这一页里，有至少一个后继可让位的在用版本。行上的「收尾」按钮只给这些：每个在用版本都带一个收尾按钮，绝大多数
 * 点开只会说没有后继。分页的册里后继可能在别的页，那一行就看不到按钮，这是只读已取回那一页的代价。
 */
export function closableKeys(all: readonly KnownVersion[]): Set<string> {
  return new Set(
    all.filter((row) => row.status === inUse && closureSuccessors(row, all).length > 0).map(closureKeyOf),
  );
}

/** 旧版自己能不能被收尾：只有在用的版本参选，别的没有可收的。 */
export function closureOldProblem(old: KnownVersion | null): string | null {
  if (old === null) return '这一版已经不在册面上，刷新目录后再看。';
  if (old.status !== inUse) return '这一版现在不是「在用」，它已不参与新的选择，没有可收尾的。';
  return null;
}

export function closureModeOf(old: KnownVersion, successor: KnownVersion): VersionClosureMode {
  return startOf(successor) > startOf(old) ? 'cut' : 'retirement';
}

export const versionClosureModeLabels: Record<VersionClosureMode, string> = {
  cut: '截止',
  retirement: '退役',
};

export interface ClosureDraft {
  successor: string;
  reference: string;
  /** 更正时刻（本地墙钟），只截止用。 */
  correctedAt: string;
}

/**
 * 更正时刻在打开时就定下，之后不随提交时间重算：同一次截止重放靠依据与更正时刻逐字相同来认，请求超时后原样再发才认得出
 * 是同一次；提交时再取「此刻」，重发的就成了另一条，旧版终点已在截止点上，后一次反而被拒。
 * 后继只有一个时直接选上，多于一个时留空让人选——选错后继落下的是另一件事。
 */
export function initialClosureDraft(successors: readonly KnownVersion[], now: Date, timeZone: string): ClosureDraft {
  return {
    successor: successors.length === 1 ? successors[0].version : '',
    reference: '',
    correctedAt: wallTimeOf(now.toISOString(), timeZone),
  };
}

/**
 * 本地能判的编码层问题，按格名归组，摆在格下。只报「填了但填错」：必填格还空着是没填完，不是错，开窗口就满屏红字没有信息，
 * 由 closureIncomplete 管着按钮。依据是否被册面接受仍由服务端答。
 */
export function closureLocalProblems(
  draft: ClosureDraft,
  mode: VersionClosureMode | null,
  timeZone: string,
): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (mode === 'cut') {
    const problem = wallMomentProblem(draft.correctedAt, timeZone);
    if (problem !== null) problems.correctedAt = [problem];
  }
  return problems;
}

/** 必填格有没有填全：后继与依据，截止另要更正时刻。没填全就不让进确认框。 */
export function closureIncomplete(draft: ClosureDraft, mode: VersionClosureMode | null): boolean {
  if (draft.successor === '' || draft.reference.trim() === '') return true;
  return mode === 'cut' && draft.correctedAt.trim() === '';
}

interface ClosureItem {
  kind: ClosureObjectKind;
  scope: string;
  objectId: string;
  version: string;
  successorVersion: string;
  reference: string;
  correctedAt?: string;
}

/** 草稿 → 载荷：一节恰一项；键名逐字取服务端批文一项的 json 标签。依据不裁首尾空白，原样送。 */
export function closurePayloadOf(
  target: ClosureTarget,
  mode: VersionClosureMode,
  draft: ClosureDraft,
  timeZone: string,
): { cuts: ClosureItem[] } | { retirements: ClosureItem[] } {
  const item: ClosureItem = {
    kind: target.kind,
    scope: target.scope,
    objectId: target.objectId,
    version: target.version,
    successorVersion: draft.successor,
    reference: draft.reference,
  };
  if (mode === 'cut') return { cuts: [{ ...item, correctedAt: wallMomentOf(draft.correctedAt, timeZone) }] };
  return { retirements: [item] };
}

export function closureConfirmTitle(mode: VersionClosureMode, target: ClosureTarget): string {
  return `${versionClosureModeLabels[mode]}${target.kindLabel} ${target.objectId} 的 ${target.version}？`;
}

export function closureConfirmMessage(mode: VersionClosureMode): string {
  return mode === 'cut'
    ? '截止后，旧版只在后继生效时点之前照旧可选，之后不再参选，重叠造成的「适用冲突」随之消除。'
    : '退役后，旧版整版不再参与任何新的选择，重叠造成的「适用冲突」随之消除；已经形成的委托、交易与结算继续引用它。';
}

function describeRange(startsAt: string, endsAt?: string): string {
  return `${formatInstant(startsAt)} → ${endsAt ? formatInstant(endsAt) : '持续有效'}`;
}

/** 确认框要复述的几行：谁被收尾、让位给谁、落下的是什么、依据；末几句说清不可撤回与入口不替人查的事。 */
export function closureConfirmLines(
  target: ClosureTarget,
  old: KnownVersion,
  successor: KnownVersion,
  mode: VersionClosureMode,
  draft: ClosureDraft,
  timeZone: string,
): string[] {
  const lines = [
    `旧版 ${old.version}：${describeRange(old.effectiveStartsAt, old.effectiveEndsAt)}`,
    `后继 ${successor.version}：从 ${formatInstant(successor.effectiveStartsAt)} 起`,
  ];
  if (mode === 'cut') {
    lines.push(
      `截止点：${formatInstant(successor.effectiveStartsAt)}（取后继的生效时点，不用另填）`,
      `依据：${draft.reference}；更正时刻：${formatInstant(wallMomentOf(draft.correctedAt, timeZone))}`,
      '写入后没有撤回截止的入口。',
    );
  } else {
    lines.push(
      '后继与旧版同一起点，旧版一刻都不剩，所以按退役处理，不是截止。',
      `依据：${draft.reference}；退役时刻取提交这一刻，不用填`,
      '一个版本只退役一次，写入后没有撤回退役的入口。',
    );
  }
  if (target.kind === 'SUPPLIER_AGREEMENT') {
    lines.push(
      '本入口不替你查有没有在途的运输委托仍指名这一版；收尾后这些委托的成本形成不了时，会列在实际履约「未形成」里，要换协议版本补上。',
    );
  }
  return lines;
}

// 领域错误的原话（internal/partycommercial/domain/version_closure.go 与 commercial_version.go）→ 中文。原话以 `party commercial:`
// 开头，通用译表会一律折成「填写的内容不合要求」，对收尾这种「组合不成立」的拒绝没有信息。次序要紧：专门的排在前面。
const causeRules: { pattern: RegExp; text: string }[] = [
  { pattern: /names the same version/, text: '旧版和后继不能是同一版。' },
  {
    pattern: /is not registered in this scope/,
    text: '这个范围的册上找不到所指名的那一版。另一对象、另一类或另一范围下的同名版本不算。',
  },
  {
    pattern: /must both be effective/,
    text: '旧版和后继都得是「在用」。已计划生效的后继还没起作用，此时收尾旧版会让后继生效之后一版都选不中。',
  },
  {
    pattern: /does not reach the end of the predecessor's selection/,
    text: '后继接不住旧版此后的那一段：它的区间到不了旧版现在的终点，收尾后那一段会变成「无适用依据」。',
  },
  {
    pattern: /not strictly inside the predecessor's selection interval/,
    text: '后继的生效时点要严格落在旧版现在的选用区间之内：不晚于旧版起点的走同起点退役，不早于旧版终点的两版本来没有重叠。',
  },
  {
    pattern: /policy body does not start at the cut/,
    text: '后继自带区间的政策正文要从截止点开始，否则两版在那一段仍会重叠或留缝。',
  },
  {
    pattern: /do not start at the same instant/,
    text: '同起点退役要求两版的选用起点相同；后继晚于旧版起点的，请改走截止。',
  },
  {
    pattern: /already retired in favour of another successor or by another decision/,
    text: '旧版已经凭另一个后继或另一份决定退役。一个版本只退役一次，不能再改写「何时、凭什么不再可选」。',
  },
  { pattern: /invalid validity correction/, text: '更正依据为空，或更正时刻无效。' },
  { pattern: /invalid commercial version transition/, text: '旧版现在不在可收尾的状态，或依据、时刻不合要求。' },
];

/** 未受理的原因：先给中文，原话收进可展开的一格；认不出的也不折成通用一句，只说这一次收尾不成立。 */
export function closureCauseText(raw: string): FieldProblemText {
  for (const rule of causeRules) {
    if (rule.pattern.test(raw)) return { text: rule.text, original: raw };
  }
  const translated = fieldProblemText(raw);
  return translated.original === null ? translated : { text: '这一次收尾不成立，没有写入任何内容。', original: raw };
}

export interface ClosureAnswer {
  /** 写入了（或认出已写过）一次收尾。 */
  done: boolean;
  text: string;
  cause?: FieldProblemText;
}

/** 收尾口业务答复的中文说法；其余答复态（未登录、未授权、传输失败……）由共用的登记答案组件呈现。 */
export function closureAnswerOf(
  mode: VersionClosureMode,
  target: ClosureTarget,
  body: VersionClosureResponseBody,
): ClosureAnswer {
  const echo =
    mode === 'cut'
      ? body.correctedStartsAt
        ? `${target.version} 此后的选用区间是 ${describeRange(body.correctedStartsAt, body.correctedEndsAt)}。`
        : ''
      : body.retiredAt
        ? `${target.version} 从 ${formatInstant(body.retiredAt)} 起不再参与新的选择，让位给 ${body.successorVersion ?? ''}。`
        : '';
  switch (body.outcome) {
    case 'REGISTERED':
      return { done: true, text: `已${versionClosureModeLabels[mode]}：${echo}` };
    case 'ALREADY_REGISTERED':
      return {
        done: true,
        text: `这一次${versionClosureModeLabels[mode]}已经登记过，册面没有再变。${echo}`,
      };
    case 'NOT_ACCEPTED':
      return {
        done: false,
        text: '没有受理，册面一个字节没写。',
        cause: body.cause ? closureCauseText(body.cause) : undefined,
      };
    default:
      return { done: false, text: `册上的答复是「${body.outcome}」，这个答案页面还不认识，请核对册面。` };
  }
}
