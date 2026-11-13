// 预计到达（ETA 预测）的查阅：读口与行转写。
//
// 单独成模块、不依赖 React：来源两格的读法与「人工录入只认当前版自己的留痕」是这一签最容易写错的地方，拿 node:test 钉住。
// 这一签只说登记了什么预测；它对客户可不可见是可见性判断的事，页面不由这里的字段推。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';
import { catalogueRequestPath, type CatalogueConditions, type CataloguePage } from '../catalogue-query';
import { formatInstant } from '../moment';
import { entrySourceLine, entrySourceWord, type ManualEntryRecord } from '../../vocabulary/provenance';

/** 一条预测线（包裹、里程碑、来源）的当前版。priorVersion 首版缺席；manualEntry 只在这一版是人工录入的时在场。 */
export interface ETAPredictionRecord {
  parcel: string;
  milestone: string;
  source: string;
  version: string;
  inputs: string;
  model: string;
  rangeFrom: string;
  rangeTo: string;
  confidence: string;
  predictedAt: string;
  priorVersion?: string;
  manualEntry?: ManualEntryRecord;
}

export interface ETAPredictionListResponseBody {
  outcome: 'ETA_PREDICTIONS_LISTED';
  predictions: ETAPredictionRecord[];
  page: CataloguePage;
}

export const etaPredictionsEndpoint = '/eta-prediction-records';

/** 收哪些键见 ports.ETAPredictionPages。 */
export function listETAPredictions(
  conditions: CatalogueConditions = {},
  after: string | null = null,
): Promise<ApiResult<ETAPredictionListResponseBody>> {
  return exchangeMasterData<ETAPredictionListResponseBody>(catalogueRequestPath(etaPredictionsEndpoint, conditions, after));
}

/** 预测口径两格（与录入口 source 的两个词同一份词表）：两种各算各的，不互相覆盖。 */
export const etaSourceLabels: Record<string, string> = {
  CARRIER_PROVIDED: '承运商告知',
  OPERATOR_DERIVED: '我们自己估的',
};

export interface ETAPredictionRow {
  key: string;
  parcel: string;
  milestone: string;
  basis: string;
  range: string;
  confidence: string;
  entryWord: string;
  entry: string;
  enteredAt: string;
  predictedAt: string;
  version: string;
  priorVersion: string;
  model: string;
  inputs: string;
}

/**
 * 当前版转写成一行。预计到达示成「起 ～ 止」的范围，不折成一个时间点：预测登记的就是范围，折了就丢了它有多宽。
 * 「口径」（承运商告知 / 自己估的）与「来源」（人工录入 / 接口送达）是两件事，各占一格——承运商告知的可以是人照着邮件录的。
 */
export function etaPredictionRowOf(record: ETAPredictionRecord): ETAPredictionRow {
  return {
    key: record.version,
    parcel: record.parcel,
    milestone: record.milestone,
    basis: etaSourceLabels[record.source] ?? record.source,
    range: `${formatInstant(record.rangeFrom)} ～ ${formatInstant(record.rangeTo)}`,
    confidence: record.confidence,
    entryWord: entrySourceWord(record.manualEntry),
    entry: entrySourceLine(record.manualEntry),
    enteredAt: record.manualEntry?.enteredAt ? formatInstant(record.manualEntry.enteredAt) : '',
    predictedAt: formatInstant(record.predictedAt),
    version: record.version,
    priorVersion: record.priorVersion ?? '',
    model: record.model,
    inputs: record.inputs,
  };
}
