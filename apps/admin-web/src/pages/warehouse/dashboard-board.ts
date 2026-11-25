// 仓配工作台要摆的作业卡和异常行（纯 TypeScript，node:test 直接测）。读口怎么问在页面里；这里只把问到的数收成屏上的话。
//
// 卡上的数是读口答的总数。没问成的卡不写 0，写「没读成」。已迟用预计到达时刻（waiting.ts 的 overdueLabel）。
// 异常行按等得最久排，每行带着打开那张单据的地址。

import { documentHash } from './document-address';
import { adjustmentKind, countsAdjustmentsModule } from './counts-adjustments-address';
import { freezeKind, processingModule } from './processing-address';
import { countStationHash, dispatchStationHash, packStationHash, pickStationHash } from './station/stations';
import { elapsedLabel, overdueLabel } from './waiting';

export interface WorkCard {
  name: string;
  /** 没问成是 null，屏上不写成 0。 */
  count: number | null;
  note: string;
  hash: string;
}

export interface ExceptionRow {
  kind: string;
  title: string;
  waited: string;
  since: number;
  next: string;
  hash: string;
}

export interface AdviceWait {
  ownerAccount: string;
  preAdviceId: string;
  documentNumber?: string;
  status: string;
  expectedAt?: string;
}

const preAdviceList = '#/warehouse-pre-advices';
const returnsList = '#/warehouse-returns';
const freezeList = '#/warehouse-processing?tab=freeze';

function card(name: string, count: number | null, note: string, hash: string): WorkCard {
  return { name, count, note: count === null ? '没读成' : note, hash };
}

export function receiveCard(advices: AdviceWait[] | null, ownersTruncated: boolean, now: Date): WorkCard {
  if (advices === null) return card('收货', null, '', preAdviceList);
  const late = advices.filter((advice) => advice.status === 'ADVISED' && overdueLabel(advice.expectedAt, now) !== '').length;
  const tail = ownersTruncated ? '货主只看了第一页。' : '';
  return card('收货', advices.length, late > 0 ? `已迟 ${late} 张。${tail}` : `还没收完。${tail}`, preAdviceList);
}

export function countCard(name: string, count: number | null, note: string, hash: string): WorkCard {
  return card(name, count, note, hash);
}

export function outboundCards(totals: { allocated: number | null; reviewing: number | null; packed: number | null }): WorkCard[] {
  return [
    card('拣货', totals.allocated, '已放波次，等拣。', pickStationHash()),
    card('复核打包', totals.reviewing, '已拣或已复核。', packStationHash()),
    card('发货', totals.packed, '已打包，等交出。', dispatchStationHash()),
  ];
}

export interface ExceptionSource {
  now: Date;
  advices: AdviceWait[];
  unclaimed: ReadonlyArray<{ arrivalId: string; receivedAt: string }>;
  adjustments: ReadonlyArray<{ adjustmentId: string; submittedAt: string }>;
  freezes: ReadonlyArray<{ freezeId: string; frozenAt: string }>;
  /** 退货里等着人动手的，页面用退货自己的状态函数挑好再交来。 */
  returns: ReadonlyArray<{ title: string; at: string; next: string; hash: string; kind: string }>;
}

function row(kind: string, title: string, at: string, next: string, hash: string, now: Date): ExceptionRow | null {
  const since = Date.parse(at);
  if (Number.isNaN(since)) return null;
  const waited = elapsedLabel(at, now);
  if (waited === '') return null;
  return { kind, title, waited, since, next, hash };
}

export function exceptionRows(source: ExceptionSource): ExceptionRow[] {
  const rows: ExceptionRow[] = [];
  for (const advice of source.advices) {
    if (advice.status !== 'ADVISED' || overdueLabel(advice.expectedAt, source.now) === '') continue;
    const item = row('收货已迟', advice.documentNumber ?? advice.preAdviceId, advice.expectedAt ?? '', '去收货', documentHash('warehouse-pre-advices', [advice.ownerAccount, advice.preAdviceId]), source.now);
    if (item) rows.push(item);
  }
  for (const arrival of source.unclaimed) {
    const item = row('待认领', arrival.arrivalId, arrival.receivedAt, '认领或处置', documentHash('warehouse-unclaimed', [arrival.arrivalId]), source.now);
    if (item) rows.push(item);
  }
  for (const adjustment of source.adjustments) {
    const item = row('调整待批', adjustment.adjustmentId, adjustment.submittedAt, '批准或驳回', documentHash(countsAdjustmentsModule, [adjustmentKind, adjustment.adjustmentId]), source.now);
    if (item) rows.push(item);
  }
  for (const freeze of source.freezes) {
    const item = row('冻结中', freeze.freezeId, freeze.frozenAt, '看这张冻结', documentHash(processingModule, [freezeKind, freeze.freezeId]), source.now);
    if (item) rows.push(item);
  }
  for (const item of source.returns) {
    const made = row(item.kind, item.title, item.at, item.next, item.hash, source.now);
    if (made) rows.push(made);
  }
  return rows.sort((left, right) => left.since - right.since).slice(0, 12);
}

export { countStationHash, freezeList };
