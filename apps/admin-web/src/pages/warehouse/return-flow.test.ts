import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { operatorNamesOf } from '../operator-access/operator-names';
import {
  adviceInFilter,
  adviceLineNote,
  adviceLineRows,
  adviceRegistrant,
  dispositionActor,
  adviceRows,
  advicesOfReceipt,
  awaitsDisposition,
  countByStatus,
  findReturnAdvice,
  findReturnReceipt,
  isReturnOrder,
  numberingAbsenceNote,
  receiptInFilter,
  receiptMatchesAdvice,
  receiptsByAdvice,
  receiptsOfAdvice,
  receiptTally,
  returnAdviceNextStep,
  returnAdviceNumber,
  returnAdviceStatus,
  returnAdviceTrack,
  returnAdviceWaitingSince,
  returnDestinationLabel,
  returnDispositionText,
  returnReceiptNextStep,
  returnReceiptStatus,
  returnReceiptTrack,
  returnReceiptWaitingSince,
  siblingReceipts,
  sortAdviceRows,
  sortReceipts,
  type ReturnAdviceFacts,
  type ReturnReceiptFacts,
} from './return-flow';

const advice = (over: Partial<ReturnAdviceFacts> = {}): ReturnAdviceFacts => ({
  adviceId: 'RPA-1',
  documentNumber: 'RT-20261120-00001',
  ownerAccount: 'SYN-ACCOUNT-01',
  warehouseCode: 'SYN-WH-01',
  trackingNumber: 'SYN-TRK-RET-9001',
  receivedAt: '2026-11-20T06:00:00Z',
  lines: [{ goodsCode: 'SYN-SKU-EARBUD-01', unit: '件', quantity: 2 }],
  ...over,
});

const receipt = (over: Partial<ReturnReceiptFacts> = {}): ReturnReceiptFacts => ({
  factId: 'SYN-FACT-1',
  warehouseCode: 'SYN-WH-01',
  trackingNumber: 'SYN-TRK-RET-9001',
  ownerAccount: 'SYN-ACCOUNT-01',
  adviceId: 'RPA-1',
  arrivalId: '',
  goodsCode: 'SYN-SKU-EARBUD-01',
  unit: '件',
  quantity: 2,
  outcome: 'COUNTED',
  occurredAt: '2026-11-20T06:30:00Z',
  ...over,
});

const disposed = { decisionId: 'D-1', destination: 'GOOD_PUTAWAY', decidedAt: '2026-11-21T01:00:00Z' };

test('预报号：有预报号取预报号；读口答空串（仓的业务时区没登记，不给号）时退回预报标识', () => {
  equal(returnAdviceNumber(advice()), 'RT-20261120-00001');
  equal(returnAdviceNumber(advice({ documentNumber: '' })), 'RPA-1');
  equal(returnAdviceNumber(advice({ documentNumber: undefined })), 'RPA-1');
});

test('按单据键认出预报：键是预报号，没有预报号的预报键是它的标识；认不出、键为空都答 undefined', () => {
  const rows = [advice(), advice({ adviceId: 'RPA-2', documentNumber: '', trackingNumber: 'SYN-TRK-2' })];
  equal(findReturnAdvice(rows, 'RT-20261120-00001')?.adviceId, 'RPA-1');
  equal(findReturnAdvice(rows, 'RPA-2')?.adviceId, 'RPA-2');
  equal(findReturnAdvice(rows, 'RPA-1')?.adviceId, 'RPA-1');
  equal(findReturnAdvice(rows, 'RT-不存在'), undefined);
  equal(findReturnAdvice(rows, ''), undefined);
  equal(findReturnAdvice([], 'RPA-1'), undefined);
});

test('按作业事实号认出到仓记录，认不出、号为空都答 undefined', () => {
  const rows = [receipt(), receipt({ factId: 'SYN-FACT-2' })];
  equal(findReturnReceipt(rows, 'SYN-FACT-2')?.factId, 'SYN-FACT-2');
  equal(findReturnReceipt(rows, 'SYN-FACT-9'), undefined);
  equal(findReturnReceipt(rows, ''), undefined);
});

test('对上预报：记着预报标识的以它为准，标识是别家的就不是，退件单号再对得上也不算', () => {
  equal(receiptMatchesAdvice(receipt(), advice()), true);
  equal(receiptMatchesAdvice(receipt({ adviceId: 'RPA-2' }), advice()), false);
});

test('对上预报：没记预报标识的按系统自己认预报的口径对——同仓同退件单号，货主已认出的还得同一家', () => {
  const claimed = receipt({ adviceId: '', arrivalId: 'UCA-1' });
  equal(receiptMatchesAdvice(claimed, advice()), true);
  equal(receiptMatchesAdvice({ ...claimed, warehouseCode: 'SYN-WH-02' }, advice()), false);
  equal(receiptMatchesAdvice({ ...claimed, trackingNumber: 'SYN-TRK-OTHER' }, advice()), false);
  equal(receiptMatchesAdvice({ ...claimed, ownerAccount: 'SYN-ACCOUNT-02' }, advice()), false);
  // 认不出货主的转待认领没有货主，退件单号对得上就算（预报是它之后才登记的）。
  equal(receiptMatchesAdvice(receipt({ adviceId: '', arrivalId: 'UCA-1', ownerAccount: '', outcome: 'UNCLAIMED' }), advice()), true);
});

test('一张预报对上的到仓记录与一笔到仓记录对上的预报，保持概览里的先后', () => {
  const a = advice();
  const rows = [
    receipt({ factId: 'F-3', adviceId: '' }),
    receipt({ factId: 'F-2', adviceId: 'RPA-9' }),
    receipt({ factId: 'F-1' }),
  ];
  deepEqual(
    receiptsOfAdvice(a, rows).map((row) => row.factId),
    ['F-3', 'F-1'],
  );
  deepEqual(
    advicesOfReceipt(rows[0], [advice({ adviceId: 'RPA-5', documentNumber: 'RT-5', trackingNumber: 'X' }), a]).map((row) => row.adviceId),
    ['RPA-1'],
  );
});

test('整份预报一次对上到仓记录，与逐张判一样，且保持概览里的先后；同一笔到仓记录不会算两遍', () => {
  const advices = [
    advice(),
    advice({ adviceId: 'RPA-2', documentNumber: 'RT-2', trackingNumber: 'SYN-TRK-2' }),
    advice({ adviceId: 'RPA-3', documentNumber: 'RT-3', trackingNumber: 'SYN-TRK-2', ownerAccount: 'SYN-ACCOUNT-02' }),
    advice({ adviceId: 'RPA-4', documentNumber: '', trackingNumber: 'SYN-TRK-4', warehouseCode: 'SYN-WH-02' }),
  ];
  const receipts = [
    receipt({ factId: 'F-6', adviceId: '', trackingNumber: 'SYN-TRK-2', ownerAccount: '', outcome: 'UNCLAIMED', arrivalId: 'UCA-6' }),
    receipt({ factId: 'F-5', adviceId: '', trackingNumber: 'SYN-TRK-2', ownerAccount: 'SYN-ACCOUNT-01' }),
    receipt({ factId: 'F-4', adviceId: 'RPA-2', trackingNumber: 'SYN-TRK-2' }),
    receipt({ factId: 'F-3', adviceId: '', trackingNumber: 'SYN-TRK-4', warehouseCode: 'SYN-WH-01' }),
    receipt({ factId: 'F-2', adviceId: '', trackingNumber: 'SYN-TRK-4', warehouseCode: 'SYN-WH-02', ownerAccount: 'SYN-ACCOUNT-01' }),
    receipt({ factId: 'F-1' }),
  ];
  const bulk = receiptsByAdvice(advices, receipts);
  for (const one of advices) {
    deepEqual(
      (bulk.get(one.adviceId) ?? []).map((row) => row.factId),
      receiptsOfAdvice(one, receipts).map((row) => row.factId),
      one.adviceId,
    );
  }
  deepEqual(bulk.get('RPA-1')?.map((row) => row.factId), ['F-1']);
  // 同一个退件单号下有记着预报标识的那笔，也有没记的那几笔：两张预报与「没记」的那几笔各按自己的口径对。
  deepEqual(bulk.get('RPA-2')?.map((row) => row.factId), ['F-6', 'F-5', 'F-4']);
  deepEqual(bulk.get('RPA-3')?.map((row) => row.factId), ['F-6']);
  deepEqual(bulk.get('RPA-4')?.map((row) => row.factId), ['F-2']);
});

test('预报的状态：没有到仓记录是待到仓；入了暂存的都记了去向是已处置；还有没记去向的是已到仓', () => {
  equal(returnAdviceStatus([]), 'AWAITING_ARRIVAL');
  equal(returnAdviceStatus([receipt()]), 'ARRIVED');
  equal(returnAdviceStatus([receipt({ disposition: disposed })]), 'DISPOSED');
  equal(returnAdviceStatus([receipt({ disposition: disposed }), receipt({ factId: 'F-2' })]), 'ARRIVED');
  equal(returnAdviceStatus([receipt({ disposition: disposed }), receipt({ factId: 'F-2', disposition: disposed })]), 'DISPOSED');
});

test('预报的状态：转了待认领的那几笔不入库、不能处置，不拦已处置；只有转待认领没有入暂存的算已到仓', () => {
  const diverted = receipt({ factId: 'F-2', outcome: 'UNCLAIMED', ownerAccount: '', adviceId: '', arrivalId: 'UCA-1' });
  equal(returnAdviceStatus([receipt({ disposition: disposed }), diverted]), 'DISPOSED');
  equal(returnAdviceStatus([diverted]), 'ARRIVED');
});

test('预报行：每张预报带上对上的到仓记录与状态', () => {
  const rows = adviceRows(
    [advice(), advice({ adviceId: 'RPA-2', documentNumber: 'RT-2', trackingNumber: 'SYN-TRK-2' })],
    [receipt({ disposition: disposed })],
  );
  deepEqual(
    rows.map((row) => [row.advice.adviceId, row.receipts.length, row.status]),
    [
      ['RPA-1', 1, 'DISPOSED'],
      ['RPA-2', 0, 'AWAITING_ARRIVAL'],
    ],
  );
});

test('到仓记录的状态：入了暂存没记去向是待处置，记了是已处置，转待认领单列；陌生的结果落在其他', () => {
  equal(returnReceiptStatus(receipt()), 'AWAITING_DISPOSITION');
  equal(returnReceiptStatus(receipt({ disposition: disposed })), 'DISPOSED');
  equal(returnReceiptStatus(receipt({ outcome: 'UNCLAIMED', ownerAccount: '', adviceId: '' })), 'DIVERTED');
  equal(returnReceiptStatus(receipt({ outcome: 'MYSTERY' })), 'OTHER');
  equal(returnReceiptStatus(receipt({ outcome: 'MYSTERY', disposition: disposed })), 'DISPOSED');
});

test('待处置只有入了暂存没记去向的；转待认领的自己不入账，不算待处置', () => {
  equal(awaitsDisposition(receipt()), true);
  equal(awaitsDisposition(receipt({ disposition: disposed })), false);
  equal(awaitsDisposition(receipt({ outcome: 'UNCLAIMED' })), false);
  equal(awaitsDisposition(receipt({ outcome: 'MYSTERY' })), false);
});

test('到仓记录的各状态各几笔', () => {
  deepEqual(
    receiptTally([
      receipt(),
      receipt({ factId: 'F-2', disposition: disposed }),
      receipt({ factId: 'F-3', disposition: disposed }),
      receipt({ factId: 'F-4', outcome: 'UNCLAIMED' }),
      receipt({ factId: 'F-5', outcome: 'MYSTERY' }),
    ]),
    { total: 5, awaiting: 1, disposed: 2, diverted: 1 },
  );
});

test('预报进度条：待到仓卡在第一步；已到仓写还有几笔待处置；已处置走到末一步；只转了待认领的不画成走完', () => {
  const waiting = returnAdviceTrack([]);
  equal(waiting.currentId, 'ADVISED');
  equal(waiting.currentLabel, '已预报，待到仓');

  const arrived = returnAdviceTrack([receipt(), receipt({ factId: 'F-2' }), receipt({ factId: 'F-3', disposition: disposed })]);
  equal(arrived.currentId, 'ARRIVED');
  equal(arrived.currentLabel, '已到仓，2 笔待处置');

  const done = returnAdviceTrack([receipt({ disposition: disposed })]);
  equal(done.currentId, 'DISPOSED');
  equal(done.currentLabel, undefined);
  equal(done.halt, undefined);

  const diverted = returnAdviceTrack([receipt({ outcome: 'UNCLAIMED', ownerAccount: '', adviceId: '' })]);
  equal(diverted.currentId, 'ARRIVED');
  equal(diverted.currentLabel, '已到仓，转了待认领');

  // 读口以后多答一种到仓结果：到了仓是真的，待处置还是转待认领都不替它说。
  const unknown = returnAdviceTrack([receipt({ outcome: 'MYSTERY' })]);
  equal(unknown.currentId, 'ARRIVED');
  equal(unknown.currentLabel, undefined);
});

test('到仓进度条：待处置卡在第一步、用待办色；已处置走到末一步；转待认领换成两步并停在第二步，不替它说后面的认领与入库', () => {
  const waiting = returnReceiptTrack(receipt());
  equal(waiting.currentId, 'ARRIVED');
  equal(waiting.currentLabel, '已到仓，待处置');

  const done = returnReceiptTrack(receipt({ disposition: disposed }));
  equal(done.currentId, 'DISPOSED');
  equal(done.currentLabel, undefined);

  const diverted = returnReceiptTrack(receipt({ outcome: 'UNCLAIMED' }));
  deepEqual(
    diverted.steps.map((step) => step.id),
    ['ARRIVED', 'DIVERTED'],
  );
  equal(diverted.currentId, 'DIVERTED');
  equal(diverted.halt, undefined);

  const other = returnReceiptTrack(receipt({ outcome: 'MYSTERY' }));
  equal(other.currentId, 'ARRIVED');
  equal(other.currentLabel, 'MYSTERY');
});

test('预报的下一步只有待到仓的有：去作业台收退件；到了仓的下一步在到仓记录上，预报不指', () => {
  deepEqual(returnAdviceNextStep('AWAITING_ARRIVAL'), { label: '去作业台收退件', moduleId: 'warehouse-workstation' });
  equal(returnAdviceNextStep('ARRIVED'), null);
  equal(returnAdviceNextStep('DISPOSED'), null);
});

test('到仓记录的下一步：待处置就地记去向；去向已记就去把货移出退货暂存；转待认领与陌生结果不在这里指', () => {
  deepEqual(returnReceiptNextStep('AWAITING_DISPOSITION'), { label: '处置' });
  deepEqual(returnReceiptNextStep('DISPOSED'), { label: '移出退货暂存', station: 'putaway' });
  equal(returnReceiptNextStep('DIVERTED'), null);
  equal(returnReceiptNextStep('OTHER'), null);
});

test('等了多久：待到仓的从受理算，待处置的从到仓算，别的不在等谁', () => {
  equal(returnAdviceWaitingSince('AWAITING_ARRIVAL', advice()), '2026-11-20T06:00:00Z');
  equal(returnAdviceWaitingSince('ARRIVED', advice()), undefined);
  equal(returnAdviceWaitingSince('DISPOSED', advice()), undefined);
  equal(returnReceiptWaitingSince(receipt()), '2026-11-20T06:30:00Z');
  equal(returnReceiptWaitingSince(receipt({ disposition: disposed })), undefined);
  equal(returnReceiptWaitingSince(receipt({ outcome: 'UNCLAIMED' })), undefined);
});

test('同一件退件里别的商品：同仓同退件单号的其余到仓记录，不含自己', () => {
  const self = receipt();
  const rows = [
    self,
    receipt({ factId: 'F-2', goodsCode: 'SYN-SKU-2' }),
    receipt({ factId: 'F-3', trackingNumber: 'SYN-TRK-OTHER' }),
    receipt({ factId: 'F-4', warehouseCode: 'SYN-WH-02' }),
  ];
  deepEqual(
    siblingReceipts(self, rows).map((row) => row.factId),
    ['F-2'],
  );
});

test('预报商品行对到仓：同商品同单位合并预计，只算入了暂存的，预报里没写的商品到了也列出来', () => {
  const a = advice({
    lines: [
      { goodsCode: 'SKU-A', unit: '件', quantity: 2 },
      { goodsCode: 'SKU-B', unit: '件', quantity: 1 },
      { goodsCode: 'SKU-A', unit: '件', quantity: 3 },
      { goodsCode: 'SKU-A', unit: '箱', quantity: 1 },
    ],
  });
  const rows = adviceLineRows(a, [
    receipt({ goodsCode: 'SKU-A', unit: '件', quantity: 4 }),
    receipt({ factId: 'F-2', goodsCode: 'SKU-A', unit: '件', quantity: 1 }),
    receipt({ factId: 'F-3', goodsCode: 'SKU-B', unit: '件', quantity: 1, outcome: 'UNCLAIMED' }),
    receipt({ factId: 'F-4', goodsCode: 'SKU-X', unit: '件', quantity: 2 }),
  ]);
  deepEqual(rows, [
    { goodsCode: 'SKU-A', unit: '件', expected: 5, arrived: 5 },
    { goodsCode: 'SKU-B', unit: '件', expected: 1, arrived: 0 },
    { goodsCode: 'SKU-A', unit: '箱', expected: 1, arrived: 0 },
    { goodsCode: 'SKU-X', unit: '件', expected: 0, arrived: 2 },
  ]);
  deepEqual(adviceLineRows({ lines: undefined }, []), []);
});

test('一行商品到了没有：预报里没有、还没到、还差几个、到齐、多到几个', () => {
  deepEqual(adviceLineNote({ expected: 0, arrived: 2 }), { label: '预报里没有', tone: 'warning' });
  deepEqual(adviceLineNote({ expected: 3, arrived: 0 }), { label: '还没到', tone: 'neutral' });
  deepEqual(adviceLineNote({ expected: 3, arrived: 1 }), { label: '还差 2', tone: 'warning' });
  deepEqual(adviceLineNote({ expected: 3, arrived: 3 }), { label: '到齐', tone: 'success' });
  deepEqual(adviceLineNote({ expected: 3, arrived: 5 }), { label: '多到 2', tone: 'warning' });
});

test('预报筛选：货主与仓按相等，检索词不分大小写、按包含匹配预报标识、预报号、退件单号、货主与仓', () => {
  const a = advice();
  const none = { keyword: '', owner: '', warehouse: '' };
  equal(adviceInFilter(a, none), true);
  equal(adviceInFilter(a, { ...none, owner: 'SYN-ACCOUNT-01' }), true);
  equal(adviceInFilter(a, { ...none, owner: 'SYN-ACCOUNT-02' }), false);
  equal(adviceInFilter(a, { ...none, warehouse: 'SYN-WH-02' }), false);
  equal(adviceInFilter(a, { ...none, keyword: ' syn-trk-ret ' }), true);
  equal(adviceInFilter(a, { ...none, keyword: 'rt-2026' }), true);
  equal(adviceInFilter(a, { ...none, keyword: 'rpa-1' }), true);
  equal(adviceInFilter(a, { ...none, keyword: '不存在' }), false);
  equal(adviceInFilter(advice({ documentNumber: undefined }), { ...none, keyword: 'rt-' }), false);
});

test('到仓记录筛选：认不出货主的没有货主，选了货主就不在其中；检索词覆盖事实号、退件单号、商品、货主与仓', () => {
  const r = receipt();
  const none = { keyword: '', owner: '', warehouse: '' };
  equal(receiptInFilter(r, none), true);
  equal(receiptInFilter(r, { ...none, keyword: 'FACT-1' }), true);
  equal(receiptInFilter(r, { ...none, keyword: 'earbud' }), true);
  equal(receiptInFilter(r, { ...none, keyword: 'TRK-RET' }), true);
  equal(receiptInFilter(r, { ...none, keyword: '不存在' }), false);
  const diverted = receipt({ outcome: 'UNCLAIMED', ownerAccount: '' });
  equal(receiptInFilter(diverted, { ...none, owner: 'SYN-ACCOUNT-01' }), false);
  equal(receiptInFilter(diverted, none), true);
  equal(receiptInFilter(r, { ...none, warehouse: 'SYN-WH-02' }), false);
});

test('各状态各几条：没出现的状态不在结果里', () => {
  deepEqual(countByStatus(['a', 'b', 'a'], (row) => row), { a: 2, b: 1 });
  deepEqual(countByStatus([], (row: string) => row), {});
});

test('排序值的判断：只认 newest 与 oldest', () => {
  equal(isReturnOrder('newest'), true);
  equal(isReturnOrder('oldest'), true);
  equal(isReturnOrder('-receivedAt'), false);
  equal(isReturnOrder(null), false);
});

test('预报排序：newest 新到旧，oldest 等得最久的在前，同刻按标识，时刻认不出的排在最老一端', () => {
  const rows = [
    { advice: advice({ adviceId: 'B', receivedAt: '2026-11-20T06:00:00Z' }) },
    { advice: advice({ adviceId: 'A', receivedAt: '2026-11-20T06:00:00Z' }) },
    { advice: advice({ adviceId: 'C', receivedAt: '2026-11-22T06:00:00Z' }) },
    { advice: advice({ adviceId: 'D', receivedAt: '不是时刻' }) },
  ];
  deepEqual(
    sortAdviceRows(rows, 'newest').map((row) => row.advice.adviceId),
    ['C', 'A', 'B', 'D'],
  );
  deepEqual(
    sortAdviceRows(rows, 'oldest').map((row) => row.advice.adviceId),
    ['D', 'A', 'B', 'C'],
  );
});

test('到仓记录排序：待处置排在最前，各组里再按时刻；转待认领不顶在前面占位置', () => {
  const rows = [
    receipt({ factId: 'DONE-NEW', disposition: disposed, occurredAt: '2026-11-24T00:00:00Z' }),
    receipt({ factId: 'WAIT-NEW', occurredAt: '2026-11-23T00:00:00Z' }),
    receipt({ factId: 'DIV', outcome: 'UNCLAIMED', occurredAt: '2026-11-25T00:00:00Z' }),
    receipt({ factId: 'WAIT-OLD', occurredAt: '2026-11-20T00:00:00Z' }),
  ];
  deepEqual(
    sortReceipts(rows, 'newest').map((row) => row.factId),
    ['WAIT-NEW', 'WAIT-OLD', 'DIV', 'DONE-NEW'],
  );
  deepEqual(
    sortReceipts(rows, 'oldest').map((row) => row.factId),
    ['WAIT-OLD', 'WAIT-NEW', 'DONE-NEW', 'DIV'],
  );
});

test('处置去向的叫法：四个后端词各有中文，「转次品」不叫库存状态那个「次品」，陌生的原样交回', () => {
  equal(returnDestinationLabel('GOOD_PUTAWAY'), '良品上架');
  equal(returnDestinationLabel('DEFECTIVE'), '转次品');
  equal(returnDestinationLabel('DESTROY'), '销毁');
  equal(returnDestinationLabel('RETURN_TO_OWNER'), '退还货主');
  equal(returnDestinationLabel('DONATE'), 'DONATE');
});

test('没有预报号时说为什么：时区没登记有专句，别的原因不外露', () => {
  equal(numberingAbsenceNote('NUMBERING_TIME_ZONE_NOT_CONFIGURED'), '仓的业务时区没登记，没有给预报号');
  equal(numberingAbsenceNote('SOMETHING_NEW'), '没有给预报号');
  equal(numberingAbsenceNote(''), '没有给预报号');
  equal(numberingAbsenceNote(undefined), '没有给预报号');
});

test('登记人：运营代录的是运营登记人，货主自己提交的没有登记人、说明白是货主提交的', () => {
  equal(adviceRegistrant({ origin: 'OPERATOR', operator: 'SEED-WF/config-operator' }), 'SEED-WF/config-operator');
  equal(adviceRegistrant({ origin: 'CUSTOMER', operator: '' }), '货主自己提交');
  equal(adviceRegistrant({ origin: 'CUSTOMER' }), '货主自己提交');
  equal(adviceRegistrant({ origin: 'OPERATOR', operator: '' }), '—');
});

test('处置人：登了姓名示姓名，没登只示 sub，货主自己提交的没有运营主体', () => {
  const operator = 'http://127.0.0.1:5556/dex#ChZTWU4tT1BFUkFUT1ItQ09ORklHLTAxEgVsb2NhbA';
  const names = operatorNamesOf([{ operator, name: '演示主管' }]);
  equal(dispositionActor({ origin: 'OPERATOR', operator }, names), '演示主管');
  equal(dispositionActor({ origin: 'OPERATOR', operator }), 'ChZTWU4tT1BFUkFUT1ItQ09ORklHLTAxEgVsb2NhbA');
  equal(dispositionActor({ origin: 'CUSTOMER', operator: '' }), '货主自己提交');
  equal(dispositionActor({ origin: 'OPERATOR' }), '—');
});

test('处置情况一句话：记了去向写去向，没记的写待处置，转待认领的写认领后另起一笔再处置', () => {
  equal(returnDispositionText(receipt({ disposition: disposed })), '良品上架');
  equal(returnDispositionText(receipt({ disposition: { destination: 'DEFECTIVE' } })), '转次品');
  equal(returnDispositionText(receipt()), '待处置');
  equal(returnDispositionText(receipt({ outcome: 'UNCLAIMED' })), '认领后另起一笔入库再处置');
  equal(returnDispositionText(receipt({ outcome: 'MYSTERY' })), '—');
});
