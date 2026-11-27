// 发货交接右边的队列：本仓已打包、等交出的出库单。按读口往下翻，翻不完就说只列了前多少张。
// 张数用读口答的总数，不用这一页的行数冒充。

import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { OutboundRequestListBody, OutboundRequestRecord } from '../api';

export const MAX_DISPATCH_PAGES = 5;

export interface DispatchQueue {
  rows: OutboundRequestRecord[];
  total: number | null;
  truncated: boolean;
}

export type DispatchQueueAnswer =
  | { kind: 'queue'; queue: DispatchQueue }
  | { kind: 'failed'; failure: Exclude<ApiResult<OutboundRequestListBody>, { kind: 'outcome' }> };

export function dispatchQueueNote(queue: DispatchQueue | null): string {
  if (queue === null) return '在读已打包、等交出的出库单。';
  const total = queue.total ?? queue.rows.length;
  const cut = queue.truncated ? `列表只列了前 ${queue.rows.length} 张。` : '';
  return `已打包、等交出的出库单 ${total} 张。${cut}`;
}

export async function loadDispatchQueue(input: {
  warehouse: string;
  read: (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<OutboundRequestListBody>>;
  maxPages?: number;
}): Promise<DispatchQueueAnswer> {
  const maxPages = input.maxPages ?? MAX_DISPATCH_PAGES;
  const rows: OutboundRequestRecord[] = [];
  let after: string | null = null;
  let pages = 0;
  let total: number | null = null;
  let next: string | null = null;
  do {
    const answer = await input.read(
      { filters: { warehouseCode: [input.warehouse], status: ['PACKED'] }, sort: 'receivedAt', pageSize: 200 },
      after,
    );
    if (answer.kind !== 'outcome') return { kind: 'failed', failure: answer };
    rows.push(...(answer.body.requests ?? []));
    total = typeof answer.body.page?.total === 'number' ? answer.body.page.total : null;
    next = answer.body.page?.next ?? null;
    after = next;
    pages += 1;
  } while (next !== null && pages < maxPages);
  return { kind: 'queue', queue: { rows, total, truncated: next !== null } };
}
