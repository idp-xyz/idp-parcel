// 工作簿 → 规范化文档的「展示用」译法：把表单生成的工作簿还原成价卡内容读口交出的那种文档形状，好让详情栏的
// 解析（price-card-content.ts）原样念出「这张卡会怎么收」。
//
// 这不是服务端译码的复刻，也不进任何提交：提交的永远是工作簿，判定与摘要都在服务端。这里只读已经通过预览的那份工作簿，
// 不做任何校验，遇到读不通的格直接抛 DocumentUnreadable，调用方退回「暂不展示」。形状以 domain 的方案快照为准；验收测试
// 拿种子里的规范化文档当参照，核这里译出的与服务端登记的一致。

import type { PriceCardWorkbook } from '../api';
import { featureMeasure } from '../price-card-content';
import { sheetNames } from './spec';

export class DocumentUnreadable extends Error {}

type Row = Record<string, string | number | null>;
type Doc = Record<string, unknown>;

/** 文档里「无上界」的写法。 */
const openEnd = '0001-01-01T00:00:00Z';

function decimal(text: string): { coefficient: string; scale: number } {
  const match = /^(-?)(\d+)(?:\.(\d+))?$/.exec(text);
  if (match === null) throw new DocumentUnreadable(`不是十进制数：${text}`);
  const [, sign, whole, fraction = ''] = match;
  const digits = (whole + fraction).replace(/^0+(?=\d)/, '');
  return { coefficient: sign === '-' && digits !== '0' ? `-${digits}` : digits, scale: fraction.length };
}

const money = (text: string, currency: string) => ({ amount: decimal(text), currency });
const quantity = (text: string, unit: string) => ({ value: decimal(text), unit });

export function workbookToDocument(workbook: PriceCardWorkbook): Doc {
  const sheet = (name: string): Row[] => workbook.sheets[name] ?? [];
  const get = (row: Row, key: string): string => {
    const value = row[key];
    return value === null || value === undefined ? '' : String(value);
  };
  const cardRows = sheet(sheetNames.card);
  const card = (key: string): string => {
    const found = cardRows.find((row) => row.field === key);
    return found === undefined ? '' : get(found, 'value');
  };

  const tables = new Map(sheet(sheetNames.tables).map((row) => [get(row, 'tableId'), row]));
  const rateTable = (tableId: string): Doc => {
    const header = tables.get(tableId);
    if (header === undefined) throw new DocumentUnreadable(`没有价表 ${tableId}`);
    const unit = get(header, 'weightUnit');
    const currency = get(header, 'currency');
    const family = get(header, 'family');
    const own = (name: string) => sheet(name).filter((row) => get(row, 'tableId') === tableId);
    const table: Doc = {
      reference: { kind: 'rate-table', id: tableId, version: get(header, 'tableVersion') },
      family,
      currency,
      unit,
      period: { startsAt: get(header, 'periodStartsAt'), endsAt: get(header, 'periodEndsAt') || openEnd },
    };
    if (family === 'WEIGHT_ZONE') {
      table.entries = own(sheetNames.ratesWeightZone).map((row) => ({
        id: get(row, 'entryId'),
        zone: get(row, 'zone'),
        minimum: quantity(get(row, 'minimum'), unit),
        ...(get(row, 'maximum') === '' ? {} : { maximum: quantity(get(row, 'maximum'), unit) }),
        amount: money(get(row, 'amount'), currency),
      }));
    } else if (family === 'FIRST_CONTINUE') {
      table.firstContinue = own(sheetNames.ratesFirstContinue).map((row) => ({
        id: get(row, 'entryId'),
        zone: get(row, 'zone'),
        firstWeight: quantity(get(row, 'firstWeight'), unit),
        firstAmount: money(get(row, 'firstAmount'), currency),
        step: quantity(get(row, 'step'), unit),
        stepAmount: money(get(row, 'stepAmount'), currency),
      }));
    } else if (family === 'UNIT_PRICE') {
      table.unitPrice = own(sheetNames.ratesUnitPrice).map((row) => ({
        id: get(row, 'entryId'),
        zone: get(row, 'zone'),
        amountPerUnit: money(get(row, 'amountPerUnit'), currency),
      }));
    } else {
      throw new DocumentUnreadable(`价表族 ${family} 不认识`);
    }
    return table;
  };

  const mainTableId = card('rateTableId');
  const mainHeader = tables.get(mainTableId);
  if (mainHeader === undefined) throw new DocumentUnreadable('没有主价表');
  const currency = get(mainHeader, 'currency');

  // ---- 条件树 ----
  const conditions = sheet(sheetNames.conditions);
  const childrenOf = (nodeId: string) => conditions.filter((row) => get(row, 'parentId') === nodeId);
  const trigger = (nodeId: string): Doc => {
    const row = conditions.find((candidate) => get(candidate, 'nodeId') === nodeId);
    if (row === undefined) throw new DocumentUnreadable(`没有条件节点 ${nodeId}`);
    const kind = get(row, 'kind');
    if (kind !== 'PREDICATE') {
      return { kind, operands: childrenOf(nodeId).map((child) => trigger(get(child, 'nodeId'))) };
    }
    const source = get(row, 'source');
    const operator = get(row, 'operator');
    const measure = featureMeasure(source);
    const predicate: Doc = { source, operator: operator === '' ? 'EQ' : operator };
    if (measure === 'category') predicate.category = get(row, 'category');
    else {
      const value = quantity(get(row, 'threshold'), get(row, 'unit'));
      if (measure === 'length') predicate.lengthThreshold = value;
      else if (measure === 'volume') predicate.volumeThreshold = value;
      else predicate.weightThreshold = value;
    }
    return { kind: 'PREDICATE', predicate };
  };

  // ---- 计算树 ----
  const calculations = sheet(sheetNames.calculations);
  const calculation = (nodeId: string): Doc => {
    const row = calculations.find((candidate) => get(candidate, 'nodeId') === nodeId);
    if (row === undefined) throw new DocumentUnreadable(`没有计算节点 ${nodeId}`);
    const method = get(row, 'method');
    switch (method) {
      case 'FIXED_AMOUNT':
        return { method, amount: money(get(row, 'amount'), currency) };
      case 'TABLE_LOOKUP':
        return { method, table: rateTable(get(row, 'tableId')) };
      case 'TABLE_LOOKUP_BY_REMOTE_TIER':
        return { method: 'TABLE_LOOKUP', tableKey: 'REMOTE_TIER', table: rateTable(get(row, 'tableId')) };
      case 'PERCENT_OF_BASIS': {
        const out: Doc = { method, basis: get(row, 'basisDependencyId') };
        if (get(row, 'seriesKind') !== '') {
          out.seriesKind = get(row, 'seriesKind');
          out.seriesFactor = decimal(get(row, 'seriesFactor'));
        } else {
          out.percentage = decimal(get(row, 'percentage'));
        }
        return out;
      }
      default:
        throw new DocumentUnreadable(`计算方法 ${method} 不认识`);
    }
  };

  // ---- 重量策略 ----
  const segmentsOf = (policy: string) =>
    sheet(sheetNames.weightRounding)
      .filter((row) => get(row, 'policy') === policy)
      .map((row) => ({
        mode: get(row, 'mode'),
        increment: quantity(get(row, 'increment'), get(row, 'unit')),
        ...(get(row, 'maximum') === '' ? {} : { maximum: quantity(get(row, 'maximum'), get(row, 'unit')) }),
      }));
  const weightPolicy: Doc = {
    reference: { kind: 'weight-policy', id: card('weightPolicyId'), version: card('weightPolicyVersion') },
    method: card('weightMethod'),
    rounding: segmentsOf('CHARGEABLE'),
  };
  if (card('volumetricDivisor') !== '') {
    weightPolicy.volumetricFactor = {
      divisor: decimal(card('volumetricDivisor')),
      lengthUnit: card('volumetricLengthUnit'),
      rounding: segmentsOf('VOLUMETRIC'),
    };
  }

  const doc: Doc = {
    canonicalization: 'FORM-PREVIEW',
    reference: { kind: 'pricing-plan', id: card('planId'), version: card('planVersion') },
    scope: card('scope'),
    direction: card('direction'),
    purpose: card('purpose'),
    baseChargeCode: card('baseChargeCode'),
    aggregation: card('aggregation'),
    period: { startsAt: card('periodStartsAt'), endsAt: card('periodEndsAt') || openEnd },
    rateTable: rateTable(mainTableId),
    weightPolicy,
  };

  const fixed = sheet(sheetNames.fixedCharges);
  if (fixed.length > 0) {
    doc.rules = fixed.map((row) => ({
      id: get(row, 'ruleId'),
      code: get(row, 'chargeCode'),
      description: get(row, 'description'),
      effect: get(row, 'effect'),
      amount: money(get(row, 'amount'), currency),
      order: Number(get(row, 'order')),
      ...(get(row, 'unit') === '' ? {} : { unit: get(row, 'unit') }),
    }));
  }

  const surcharges = sheet(sheetNames.surcharges);
  if (surcharges.length > 0) {
    doc.surchargeRules = surcharges.map((row) => ({
      id: get(row, 'ruleId'),
      code: get(row, 'chargeCode'),
      description: get(row, 'description'),
      effect: get(row, 'effect'),
      condition: trigger(get(row, 'conditionId')),
      calculation: calculation(get(row, 'calculationId')),
      exclusivity: get(row, 'exclusivity'),
      ...(get(row, 'exclusivityGroup') === '' ? {} : { exclusivityGroup: get(row, 'exclusivityGroup') }),
      ...(get(row, 'priority') === '' ? {} : { priority: Number(get(row, 'priority')) }),
      ...(get(row, 'unit') === '' ? {} : { unit: get(row, 'unit') }),
    }));
  }

  const dependencies = sheet(sheetNames.chargeDependencies);
  if (dependencies.length > 0) {
    const codes = (text: string) => text.split(';').map((code) => code.trim()).filter((code) => code !== '');
    doc.chargeDependencies = dependencies.map((row) => ({
      id: get(row, 'dependencyId'),
      code: get(row, 'chargeCode'),
      composition: get(row, 'composition'),
      ...(get(row, 'includes') === '' ? {} : { includes: codes(get(row, 'includes')) }),
      ...(get(row, 'excludes') === '' ? {} : { excludes: codes(get(row, 'excludes')) }),
    }));
  }

  const series = sheet(sheetNames.referenceSeries);
  if (series.length > 0) doc.referenceSeries = series.map((row) => ({ kind: get(row, 'kind'), seriesId: get(row, 'seriesId') }));

  if (card('amountRoundingMode') !== '') {
    doc.amountRounding = {
      mode: card('amountRoundingMode'),
      increment: money(card('amountRoundingIncrement'), currency),
      points: card('amountRoundingPoints').split(';'),
    };
  }

  const catalogues = sheet(sheetNames.referenceCatalogues);
  if (catalogues.length > 0) {
    doc.referenceCatalogues = catalogues.map((row) => ({ kind: get(row, 'kind'), catalogueId: get(row, 'catalogueId') }));
  }

  const bounds = sheet(sheetNames.totalBounds)[0];
  if (bounds !== undefined) {
    doc.totalChargeBounds = {
      ...(get(bounds, 'minimum') === '' ? {} : { minimum: money(get(bounds, 'minimum'), currency) }),
      ...(get(bounds, 'maximum') === '' ? {} : { maximum: money(get(bounds, 'maximum'), currency) }),
    };
  }

  return doc;
}
