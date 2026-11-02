import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { documentNumberAbsenceNoteOf, parcelNoOf, shipmentDisplayNoOf, shipmentNoOf } from './ref-no';

// 本文件钉运单号的屏上写法(ADR-0355 决定十一):有单据号显单据号、原样不裁;没有单据号的委托照旧显去前缀的委托标识,
// 技术详情按记下的无号情形分两句说,不一律写成上线前开立。

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

test('技术详情的无号说明按记下的情形分两句,有号时没有这一句', () => {
  equal(documentNumberAbsenceNoteOf({ documentNumber: 'P20261102000123' }), undefined);
  equal(documentNumberAbsenceNoteOf({ documentNumberAbsence: 'ISSUED_BEFORE_NUMBERING' }), '上线前开立，无单据号');
  equal(
    documentNumberAbsenceNoteOf({ documentNumberAbsence: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' }),
    '开立时单据编号时区未配置，无单据号',
  );
  // 读口没带情形:两列皆空就是上线前开立。
  equal(documentNumberAbsenceNoteOf({}), '上线前开立，无单据号');
});
