// 商业版本的行详情里「这个对象的各个版本」那一栏（票 party-commercial-uiux/21）。
//
// 目录把各版按对象排在一起（票 08），但一眼看不出这个对象走过怎样的换版：哪一版现在在用，旧版是被截止还是退役、让位给了
// 谁、从什么时候起。这里从目录已取回的各行里，取同一对象同一范围的各版，按生效起点倒序列成一条历史，每版给一行：版本号、
// 状态、原区间，以及被收尾的话说清怎么收的。纯函数，node:test 直接跑。
//
// 只用目录行上已有的字：读面带状态的册才有状态与收尾回显（截止点、让位后继、退役时刻），别的册（信用、结算、价格政策、
// 接单规则包）只有区间，历史里就只列区间。原区间与收尾回显分开显示，收尾不改写原区间。

import { formatInstant, formatRange } from '../moment';

/** 一版在目录行上的几项；各册记录形状不同，由调用方投出来。 */
export interface HistoryFacts {
  objectId: string;
  version: string;
  scope?: string;
  /** 服务端状态码；读面没有状态的册不给。 */
  status?: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  selectionEndsAt?: string;
  successorVersion?: string;
  retiredAt?: string;
}

export interface HistoryEntry {
  /** 对象内的版本号，页面据它找回那一行。 */
  version: string;
  status?: string;
  /** 原有效区间的文字。 */
  interval: string;
  /** 被收尾时怎么收的；没收尾为空。 */
  closure?: string;
}

const versionOrder = new Intl.Collator('en', { numeric: true, sensitivity: 'base' });

/** 同一对象同一范围的各版，新的在前：先比生效起点，起点相同再按版本号自然序。 */
export function versionHistoryOf(all: readonly HistoryFacts[], of: HistoryFacts): HistoryEntry[] {
  return all
    .filter((row) => row.objectId === of.objectId && (row.scope ?? '') === (of.scope ?? ''))
    .sort(
      (a, b) =>
        b.effectiveStartsAt.localeCompare(a.effectiveStartsAt) || versionOrder.compare(b.version, a.version),
    )
    .map((row) => ({
      version: row.version,
      status: row.status,
      interval: formatRange(row.effectiveStartsAt, row.effectiveEndsAt),
      closure: closureText(row),
    }));
}

function closureText(row: HistoryFacts): string | undefined {
  if (row.retiredAt !== undefined) {
    const successor = row.successorVersion ? `，让位给 ${row.successorVersion}` : '';
    return `已退役：${formatInstant(row.retiredAt)} 起不再参与新的选择${successor}`;
  }
  if (row.selectionEndsAt !== undefined) {
    return `已截止：选用区间止于 ${formatInstant(row.selectionEndsAt)}，此后不再参选`;
  }
  return undefined;
}
