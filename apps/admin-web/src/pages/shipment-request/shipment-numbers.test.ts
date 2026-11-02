import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { shipmentIdBatches, shipmentNumberIndexOf, shipmentNumberReader } from './shipment-numbers';
import type { ApiResult, ShipmentRequestSummary, ViewsListResponseBody } from './api';

// 本文件钉别的上下文的页面上出现的委托怎么显运单号:按读口上限分批去重;各批合成一份索引、缺一批整份当读不到;有号显号,
// 无号显委托标识并按读口记下的情形注明,读不到注明读不到,看不见的与还在读的只显委托标识、不替它说什么。

const summary = (overrides: Partial<ShipmentRequestSummary>): ShipmentRequestSummary => ({
  shipmentRequestId: 'SHR-1',
  customerAccountId: 'SYN-ACCOUNT-01',
  source: 'src',
  sourceRequestKey: 'K-1',
  state: 'ACCEPTED',
  submissionVersionId: 'SV-1',
  declaredParcelCount: 1,
  submittedAt: '2026-11-02T03:00:00Z',
  ...overrides,
});

const listed = (...requests: ShipmentRequestSummary[]): ApiResult<ViewsListResponseBody> => ({
  kind: 'outcome',
  status: 200,
  body: { outcome: 'LISTED', requests },
});

test('分批:去重、保留首次出现的次序、空标识不问,每批不超过上限', () => {
  deepEqual(shipmentIdBatches(['SHR-B', 'SHR-A', 'SHR-B', '', ' ', 'SHR-C'], 2), [['SHR-B', 'SHR-A'], ['SHR-C']]);
  deepEqual(shipmentIdBatches([]), []);
  const many = Array.from({ length: 250 }, (_, index) => `SHR-${index}`);
  deepEqual(
    shipmentIdBatches(many).map((batch) => batch.length),
    [100, 100, 50],
  );
});

test('各批答复合成一份索引;任一批没形成答案整份当读不到', () => {
  const index = shipmentNumberIndexOf([listed(summary({ shipmentRequestId: 'SHR-1' })), listed(summary({ shipmentRequestId: 'SHR-2' }))]);
  deepEqual(index === null ? null : [...index.keys()], ['SHR-1', 'SHR-2']);
  equal(shipmentNumberIndexOf([listed(summary({})), { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }]), null);
  equal(shipmentNumberIndexOf([{ kind: 'unconfigured' }]), null);
});

test('有号显运单号,不注明', () => {
  const read = shipmentNumberReader(new Map([['SHR-01JA', summary({ shipmentRequestId: 'SHR-01JA', documentNumber: 'P20261102000001' })]]));
  deepEqual(read('SHR-01JA'), { text: 'P20261102000001', documentNumber: 'P20261102000001', note: '' });
});

test('无号显去前缀的委托标识,按读口记下的情形注明,不一律说上线前开立', () => {
  const read = shipmentNumberReader(
    new Map([
      ['SHR-OLD', summary({ shipmentRequestId: 'SHR-OLD', documentNumberAbsence: 'ISSUED_BEFORE_NUMBERING' })],
      ['SHR-TZ', summary({ shipmentRequestId: 'SHR-TZ', documentNumberAbsence: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' })],
    ]),
  );
  deepEqual(read('SHR-OLD'), { text: 'OLD', note: '上线前开立，无单据号' });
  deepEqual(read('SHR-TZ'), { text: 'TZ', note: '开立时单据编号时区未配置，无单据号' });
});

test('看不见的(作用域外或查无)与还在读的只显委托标识,不替它说什么', () => {
  deepEqual(shipmentNumberReader(new Map())('SHR-GONE'), { text: 'GONE', note: '' });
  deepEqual(shipmentNumberReader(undefined)('SHR-1'), { text: '1', note: '' });
});

test('读口没答:号这一格降级成委托标识并注明读不到', () => {
  deepEqual(shipmentNumberReader(null)('SHR-1'), { text: '1', note: '运单号读不到(运单查阅这次没有答复),暂显委托标识' });
});
