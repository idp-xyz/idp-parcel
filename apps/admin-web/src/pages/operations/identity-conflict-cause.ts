// 身份冲突来由在管理台上的写法（票 no-reception-identity/07；ADR-0211 决定一）。
//
// 单独成模块：待识别实物区的「冲突来由」一列、「重核」动作能不能发起、重核答复里那一句冲突怎么解，三处都要读来由，词只在这里定
// 一份。最容易错的是把来由缺席读成「只因歧义信号」或反过来——来由这一格落地之前的冲突行不带来由，它们照旧只经处置解开，页面
// 如实写「来由未记」，不替它推。

import type { UnidentifiedItemRecord } from './records-api';

/** 唯一可经重核解开的那一格来由的线上名。 */
export const ambiguityOnlyCause = 'AMBIGUITY_ONLY';

/** 冲突来由封闭三值（ports.IdentityConflictCause），每格一句：从哪来、怎么解。 */
export const identityConflictCauseLabels: Record<string, string> = {
  AMBIGUITY_ONLY: '只因歧义信号：号码在册只标识一件，另有一份已提交、未决的委托也报了它；那份委托被拒绝或撤回后可再重核',
  MULTIPLE_CANDIDATES: '多候选：号码此刻标识多件包裹，只经处置解开',
  ONE_MARK_ON_TWO_UNITS: '一码两物：号码指向的包裹已关联到另一件实物，只经处置解开',
};

/** 「重核」动作的不给理由与答复里要点名来由时用的短名。 */
const causeShortNames: Record<string, string> = {
  AMBIGUITY_ONLY: '只因歧义信号',
  MULTIPLE_CANDIDATES: '多候选',
  ONE_MARK_ON_TWO_UNITS: '一码两物',
};

type CauseFields = Pick<UnidentifiedItemRecord, 'identityConflict' | 'identityConflictCause'>;

/**
 * 「冲突来由」一格。不记冲突写无冲突；记了冲突却不带来由的是来由这一格落地之前的行，写「来由未记」并说它只经处置解开；页面未收录
 * 的码原样示出，不归进某一格，也不许诺能重核。
 */
export function identityConflictCauseCellOf(record: CauseFields): string {
  if (!record.identityConflict) return '无冲突';
  const cause = record.identityConflictCause;
  if (cause === undefined || cause === '') return '来由未记：此前落下的冲突，只经处置解开';
  return identityConflictCauseLabels[cause] ?? `来由 ${cause}（页面未收录），按只经处置办`;
}

/** 来由的短名：三值之一、来由未记，或未收录的码原样。 */
export function identityConflictCauseShortOf(cause: string | undefined): string {
  if (cause === undefined || cause === '') return '来由未记';
  return causeShortNames[cause] ?? `来由 ${cause}`;
}

/**
 * 这一行的冲突能不能经重核解开：只有来由恰是只因歧义信号的（ADR-0211 决定二）。来由未记与未收录的码一律不算——页面放错了，
 * 服务端照样答「记着身份冲突」，可看的人会以为点一下就能解开。
 */
export function conflictReleasableByRecheck(record: CauseFields): boolean {
  return record.identityConflict && record.identityConflictCause === ambiguityOnlyCause;
}
