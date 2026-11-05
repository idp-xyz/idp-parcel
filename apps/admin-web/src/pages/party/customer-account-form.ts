// 货主客户账户登记表单的纯逻辑（票 business-parties-object-page/03）：形状照 business-party-form.ts——格少、低频、无矩阵，直接逐字段
// 表单（ADR-0101 决定八）。全部是纯函数，node:test 钉着；组件 CustomerAccountRegistrationForm.tsx 只负责摆。
//
// **不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：账户编码与所属参与方是否为空、参与方在不在册、是不是同一
// 租户、修订号连不连续，一律原样送上去让服务端逐格答；这里只做编码层：修订号编成 JSON 整数、墙钟时刻换成 RFC 3339、可缺的键缺席。
// **各格不裁首尾空白**，理由同 business-party-form.ts 文件头；**载荷不带租户格**，镜像受控 CLI 输入里 `customerAccounts` 数组的一项。

import type { CustomerAccountRecord } from './api';
import { wallTimeToRfc3339 } from '../moment';
import { revisionOf, suggestedNextRevision } from './registration-form';

export interface CustomerAccountDraft {
  accountId: string;
  customerPartyId: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟），留空即缺席。 */
  effectiveFrom: string;
}

/** 键名逐字取登记快照提示（presentation.ts 的 customer-account）。 */
export interface CustomerAccountRegistrationItem {
  accountId: string;
  customerPartyId: string;
  revision?: number;
  basis: string;
  effectiveFrom?: string;
}

export interface CustomerAccountRegistrationPayload {
  customerAccounts: CustomerAccountRegistrationItem[];
}

export function emptyCustomerAccountDraft(): CustomerAccountDraft {
  return { accountId: '', customerPartyId: '', revision: '', basis: '', effectiveFrom: '' };
}

export function customerAccountPayloadOf(draft: CustomerAccountDraft, timeZone: string): CustomerAccountRegistrationPayload {
  const item: CustomerAccountRegistrationItem = {
    accountId: draft.accountId,
    customerPartyId: draft.customerPartyId,
    basis: draft.basis,
  };
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  if (draft.effectiveFrom.trim() !== '') {
    const moment = wallTimeToRfc3339(draft.effectiveFrom, timeZone);
    if (moment !== null) item.effectiveFrom = moment;
  }
  return { customerAccounts: [item] };
}

/** 本地能判的编码层问题：修订号编不进正整数、生效时刻换不成 RFC 3339。其余是服务端的话。 */
export function customerAccountLocalProblems(draft: CustomerAccountDraft, timeZone: string): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['customerAccounts[0].revision'] = ['修订号要填正整数'];
  }
  if (draft.effectiveFrom.trim() !== '' && wallTimeToRfc3339(draft.effectiveFrom, timeZone) === null) {
    problems['customerAccounts[0].effectiveFrom'] = ['生效时刻要是可解析的日期时间'];
  }
  return problems;
}

/** 修订号建议值：按账户编码原串在已取回的账户册里找（不裁空白，与载荷同判）。 */
export function suggestedCustomerAccountRevision(rows: readonly CustomerAccountRecord[] | null, accountId: string): number {
  return suggestedNextRevision(rows, accountId, (row) => row.accountId);
}
