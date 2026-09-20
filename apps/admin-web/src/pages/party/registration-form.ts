// 参与方模块各登记表单（责任法人 / 业务参与方 / 参与方关系 / 身份停用）共用的纯逻辑（票 admin-web-group-legal-
// entities/13 第 1 / 3 条抬出）。各册的 *-form.ts 各自只留本册的形状（草稿、载荷键、认领路径、修订建议取哪一格作键），
// 跨册同一判的规则住在这里；此前 revisionOf 住在 business-party-form.ts 里导出、别的册反向依赖兄弟模块（票 10 评审 N3），
// 建议值顶格与落地判据则在各表单组件里各写一遍（票 10 评审 N1），「最新修订加一」在各册的 *-form.ts 里各写一遍、只差取键
// 的字段（票 13 判断项 → 票 14 第 2 条）。组件那半的壳是 party-registration-fields.tsx 的 useRegistrationForm，它只调这里的规则。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：只做把格编对的事。

import type { ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';

const positiveInteger = /^[1-9]\d*$/;

/**
 * 修订号编成正整数；编不出交回 undefined，由各表单的问题表拦在送之前、载荷里该键缺席——不造一个假数顶上。
 * 首尾空白在这一格允许：它编成的是数不是身份串，周围空白不构成第二个值；身份串各表单自己裁不裁与它无关。
 */
export function revisionOf(raw: string): number | undefined {
  const trimmed = raw.trim();
  return positiveInteger.test(trimmed) ? Number(trimmed) : undefined;
}

/** 带修订号格的草稿：各册的登记都是「一笔一个修订」，这一格各表单同形。 */
export interface RevisionedDraft {
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
}

/**
 * 修订号建议值：标识在已取回的册里 → 该行的最新修订 + 1；不在（或册没取到、标识为空）→ 1。**只是建议**：列表答的是
 * 最新修订、且取回那一刻起就可能过期，连续性（登记）或错位（停用）仍由服务端按册面判；这一格省的是操作者翻一次册，
 * 不替服务端作数。
 *
 * 各册的行形各异，取标识的字段由 keyOf 交进来；取出的键与 id **逐字比**，共用体不裁空白——裁不裁是各册载荷的判据
 * （业务参与方 / 关系 / 停用不裁、法人裁，理由各在其 *-form.ts 文件头），建议得与该册送上去的东西说同一个对象，所以要
 * 裁的在自己的包装里裁完再交进来，不在这里替所有册决定。
 */
export function suggestedNextRevision<Row extends { revision: number }>(
  rows: readonly Row[] | null,
  id: string,
  keyOf: (row: Row) => string,
): number {
  if (id === '' || rows === null) return 1;
  const latest = rows.find((row) => keyOf(row) === id);
  return latest ? latest.revision + 1 : 1;
}

/**
 * 送出与校验用的草稿：操作者没改过修订号时把建议值顶进去（其余格不动）；改过就用草稿原值，直到点「用建议值」
 * 复位。建议只是省一次翻册，连续性（登记）或错位（停用）仍由服务端按册面判，所以顶进去的也只是个待送的字符串。
 */
export function effectiveDraftOf<Draft extends RevisionedDraft>(
  draft: Draft,
  revisionEdited: boolean,
  suggestion: number,
): Draft {
  return revisionEdited ? draft : { ...draft, revision: String(suggestion) };
}

/**
 * 登记册答的是这一口的「已落册」线上名（PartyRegistryOutcome 里 PartyIdentityRegistered 的 REGISTERED、
 * PartyIdentityDeactivated 的 DEACTIVATED）才算落地、才该触发读签重取：重放、冲突、未受理都没写进去，未配置与各类
 * 失败更没有——重取只会让人以为写进去了。
 */
export function registrationLanded(answer: ApiResult<RegistrationResponseBody>, landedOutcome: string): boolean {
  return answer.kind === 'outcome' && answer.body.outcome === landedOutcome;
}
