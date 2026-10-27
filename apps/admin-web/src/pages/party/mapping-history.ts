// 产品—渠道映射详情里「历史修订」那一栏（票 party-commercial-uiux-backend/02）。
//
// 映射按修订推进：调整绑定或区间是翻旧插新，不覆盖历史。读口按映射标识交出整条修订链，这里把每一笔列成一行：第几修订、
// 这一笔的有效区间、绑定了哪几个渠道（显式“未配置”如实写出，不留空）、依据。新的在前；每一笔都是一份完整的登记信封，
// 不做两笔之间的 diff，改了什么由人对着相邻两行看。纯函数，node:test 直接跑。

import { formatInstant, formatRange } from '../moment';
import type { ProductChannelMappingRecord } from './api';
import type { HistoryEntry } from './version-history';

export function mappingHistoryOf(revisions: readonly ProductChannelMappingRecord[]): HistoryEntry[] {
  return [...revisions]
    .sort((a, b) => b.revision - a.revision)
    .map((row) => ({
      version: String(row.revision),
      label: `第 ${row.revision} 修订`,
      interval: formatRange(row.effectiveStartsAt, row.effectiveEndsAt),
      note:
        `渠道：${row.channels.length > 0 ? row.channels.join('、') : '未配置（显式登记）'}；依据：${row.basis}；` +
        `登记于 ${formatInstant(row.registeredAt)}`,
    }));
}
