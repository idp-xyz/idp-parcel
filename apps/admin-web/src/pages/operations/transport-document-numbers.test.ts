import { test } from 'node:test';
import { equal, deepEqual } from 'node:assert/strict';
import { documentNumberAbsenceNote } from '../../vocabulary/document-numbering';
import { masterDocumentReferencesOf, referencedNumberOf } from './transport-document-numbers';
import type { CarrierMasterDocumentRecord } from './records-api';

function masterDocument(overrides: Partial<CarrierMasterDocumentRecord> = {}): CarrierMasterDocumentRecord {
  return {
    document: 'MAWB-1',
    version: 'v1',
    issuer: 'SYN-CARRIER-X',
    scope: 'SYN-LANE-1',
    standing: 'IN_FORCE',
    associationCount: '0',
    recordedAt: '2026-11-02T03:00:00Z',
    ...overrides,
  };
}

test('引用的号：有号显号，无号按原因注明，两键都没给时留空不补「上线前开立」', () => {
  equal(referencedNumberOf('TC-202611-00004', undefined), 'TC-202611-00004');
  equal(referencedNumberOf(undefined, 'ISSUED_BEFORE_NUMBERING'), documentNumberAbsenceNote('ISSUED_BEFORE_NUMBERING'));
  equal(
    referencedNumberOf(undefined, 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'),
    documentNumberAbsenceNote('NUMBERING_TIME_ZONE_NOT_CONFIGURED'),
  );
  equal(referencedNumberOf(undefined, undefined), '');
});

test('总单所引委托、订舱：号列有号显号、没号显标识；标识与无号原因进技术详情；没引用的那一半留空', () => {
  deepEqual(
    masterDocumentReferencesOf(
      masterDocument({
        commission: 'SYN-COMMISSION-1',
        commissionDocumentNumber: 'TC-202611-00004',
        booking: 'SYN-BOOKING-1',
        bookingDocumentNumberAbsence: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED',
      }),
    ),
    {
      commissionNumber: 'TC-202611-00004',
      commissionId: 'SYN-COMMISSION-1',
      commissionNote: '',
      bookingNumber: 'SYN-BOOKING-1',
      bookingId: 'SYN-BOOKING-1',
      bookingNote: documentNumberAbsenceNote('NUMBERING_TIME_ZONE_NOT_CONFIGURED'),
    },
  );
  deepEqual(masterDocumentReferencesOf(masterDocument({ booking: 'SYN-BOOKING-GHOST' })), {
    commissionNumber: '',
    commissionId: '',
    commissionNote: '',
    bookingNumber: 'SYN-BOOKING-GHOST',
    bookingId: 'SYN-BOOKING-GHOST',
    bookingNote: '',
  });
});
