import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { exceptionRows, outboundCards, receiveCard } from './dashboard-board';

const now = new Date('2026-11-25T12:00:00Z');

test('收货卡：已迟只数已预报且预计时刻已过的；没读成不写成 0', () => {
  const card = receiveCard(
    [
      { ownerAccount: 'O', preAdviceId: 'A', status: 'ADVISED', expectedAt: '2026-11-20T00:00:00Z' },
      { ownerAccount: 'O', preAdviceId: 'B', status: 'RECEIVING', expectedAt: '2026-11-20T00:00:00Z' },
    ],
    false,
    now,
  );
  equal(card.count, 2);
  equal(card.note.startsWith('已迟 1'), true);
  equal(receiveCard(null, false, now).note, '没读成');
  equal(receiveCard(null, false, now).count, null);
});

test('出库三张卡指到对应工位；没读成的那张不写成 0', () => {
  const cards = outboundCards({ allocated: 3, reviewing: null, packed: 0 });
  equal(cards[0].hash, '#/station/pick');
  equal(cards[0].count, 3);
  equal(cards[1].count, null);
  equal(cards[1].note, '没读成');
  equal(cards[2].hash, '#/station/dispatch');
  equal(cards[2].count, 0);
});

test('异常按等得最久排，最多 12 行', () => {
  const rows = exceptionRows({
    now,
    advices: [],
    unclaimed: [{ arrivalId: 'U-1', receivedAt: '2026-11-24T00:00:00Z' }],
    adjustments: [{ adjustmentId: 'ADJ-1', submittedAt: '2026-11-20T00:00:00Z' }],
    freezes: [],
    returns: [],
  });
  equal(rows[0].title, 'ADJ-1');
  equal(rows[1].kind, '待认领');
  equal(rows[0].hash.startsWith('#/warehouse-counts-adjustments/'), true);
});
