// 称重量方册的行转写。
//
// 单独成模块而不写在页面里：这一册上屏的两个词（对象种类、作废与否）取自用语词典，页面组件要跑起来得有 React 与 DOM，
// 转写是纯函数就能拿 node:test 钉住它们上屏的字（同 reception-rows 的先例）。

import { formatInstant } from '../catalogue-view';
import { labelFor } from '../../vocabulary/status';
import type { ActualMeasurementRecord } from './records-api';

export interface MeasurementReviewRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

/** 一行实际测量转写成称重量方册的一行；重量与长宽高照登记时的原数拼上各自的单位，不换算。 */
export function measurementRowOf(record: ActualMeasurementRecord): MeasurementReviewRow {
  return {
    key: record.sourceId,
    values: {
      operationalItem: record.objectId,
      weight: `${record.weightValue} ${record.weightUnit}`,
      dimensions: `${record.length}×${record.width}×${record.height} ${record.dimensionUnit}`,
      occurredAt: formatInstant(record.occurredAt),
      voided: labelFor(String(record.voided), 'measurement-voided'),
      sourceId: record.sourceId,
      objectKind: labelFor(record.objectKind),
      location: record.location,
      basis: record.basis,
      performedBy: record.performedBy,
      enteredBy: record.enteredBy ?? '',
      originalDocument: record.originalDocument ?? '',
      recordedAt: formatInstant(record.recordedAt),
    },
  };
}
