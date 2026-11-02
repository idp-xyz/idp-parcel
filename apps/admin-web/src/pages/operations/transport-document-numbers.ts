// 运输班次页上委托、订舱引用的单据号怎么上屏（ADR-0355 决定十一）。
//
// 单独成模块、不依赖 React：订舱区与总单区都引用委托、订舱，号与无号原因在两处要一种写法。无号的说法只取用语词典
// （vocabulary/document-numbering 的 documentNumberAbsenceNote），这里不另写一份。

import { documentNumberAbsenceNote } from '../../vocabulary/document-numbering';
import type { CarrierMasterDocumentRecord } from './records-api';

/**
 * 一个引用的无号说法：有号或读口没给原因（引用为空或单据不在册）时留空——不替它补一个「上线前开立」。
 */
export function referenceNoteOf(number?: string, absence?: string): string {
  return !number && absence ? documentNumberAbsenceNote(absence) : '';
}

/**
 * 技术详情里一个引用的「号」那一格：有号显号；无号按读口记下的原因注明，不拿标识顶（标识另列）；读口两键都没给时留空。
 */
export function referencedNumberOf(number?: string, absence?: string): string {
  return number ?? referenceNoteOf(number, absence);
}

/**
 * 总单一版所引委托、订舱：号列与主列同一条纪律——有号显号、没号显标识；标识与无号原因进技术详情。没引用的那一半留空。
 */
export function masterDocumentReferencesOf(record: CarrierMasterDocumentRecord): Record<string, string> {
  return {
    commissionNumber: record.commissionDocumentNumber ?? record.commission ?? '',
    commissionId: record.commission ?? '',
    commissionNote: referenceNoteOf(record.commissionDocumentNumber, record.commissionDocumentNumberAbsence),
    bookingNumber: record.bookingDocumentNumber ?? record.booking ?? '',
    bookingId: record.booking ?? '',
    bookingNote: referenceNoteOf(record.bookingDocumentNumber, record.bookingDocumentNumberAbsence),
  };
}
