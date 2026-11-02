import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { documentNumberAbsenceNoteOf, parcelNoOf, shipmentDisplayNoOf, shipmentNoOf } from './ref-no';
import { documentNumberAbsenceNote } from '../../vocabulary/document-numbering';

// 本文件钉运单号的屏上写法(ADR-0355 决定十一):有单据号显单据号、原样不裁;没有单据号的委托照旧显去前缀的委托标识,
// 技术详情按记下的无号情形说(说法取单据编号词典),不一律写成上线前开立,读口没说情形就不猜。

test('有单据号时运单号就是单据号,原样不裁前缀', () => {
  equal(shipmentDisplayNoOf({ shipmentRequestId: 'SHR-01JABC', documentNumber: 'P20261102000123' }), 'P20261102000123');
  // 租户模板恰好以 SHR- 起头也不裁:那是号本身的一部分。
  equal(shipmentDisplayNoOf({ shipmentRequestId: 'SHR-01JABC', documentNumber: 'SHR-2026-0001' }), 'SHR-2026-0001');
});

test('没有单据号的委托照旧显去前缀的委托标识', () => {
  equal(shipmentDisplayNoOf({ shipmentRequestId: 'SHR-01JABC' }), '01JABC');
  equal(shipmentNoOf('SYN-REQ-1'), 'SYN-REQ-1');
  equal(parcelNoOf('DPCL-01JXYZ'), '01JXYZ');
});

test('技术详情的无号说明按记下的情形取词典的说法,有号或读口没说情形时没有这一句', () => {
  equal(documentNumberAbsenceNoteOf({ documentNumber: 'P20261102000123' }), undefined);
  equal(
    documentNumberAbsenceNoteOf({ documentNumberAbsence: 'ISSUED_BEFORE_NUMBERING' }),
    documentNumberAbsenceNote('ISSUED_BEFORE_NUMBERING'),
  );
  equal(
    documentNumberAbsenceNoteOf({ documentNumberAbsence: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' }),
    documentNumberAbsenceNote('NUMBERING_TIME_ZONE_NOT_CONFIGURED'),
  );
  equal(documentNumberAbsenceNoteOf({}), undefined);
});
