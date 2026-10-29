// 工作簿 → 表单状态（「表单 → 工作簿」的反方向）。
//
// 产品页面不用它：后端没有「已登记价卡反推成模板」的读口（ADR-0349 决定八），编辑器只能新建。它留着是验收的办法：拿一份
// 现成的、服务端认可的工作簿（FULL-03 样例）还原成表单状态，再生成回来，核「表单能表达这张卡、而且表达得一字不差」。
// 将来后端有了反向读口，打开已有价卡改一版就接在这里。
//
// 还原时把现成的标识记进 idHints，生成时原样用回，所以生成的工作簿连标识都与原件相同，内容摘要也就相同。
// 表单表达不了的结构（取较大者、排除条款、条件最低计价重量、版本清单依赖、嵌套条件组……）抛 NotRepresentable，
// 不静默丢掉。

import type { PriceCardWorkbook } from '../api';
import { featureMeasure } from '../price-card-content';
import {
  emptyState,
  newCondition,
  newRateRow,
  newSegment,
  nextUid,
  type CardFormState,
  type ConditionRow,
  type FeeItemState,
  type FeeKind,
  type RateRow,
  type RoundingSegment,
} from './model';
import { familySheet, sheetNames } from './spec';

export class NotRepresentable extends Error {}

type Row = Record<string, string | number | null>;

export function workbookToForm(workbook: PriceCardWorkbook): CardFormState {
  const state = emptyState();
  const hints = state.idHints;
  const sheet = (name: string): Row[] => workbook.sheets[name] ?? [];
  const text = (row: Row, key: string): string => {
    const value = row[key];
    return value === null || value === undefined ? '' : String(value);
  };
  const card = (key: string): string => {
    const found = sheet(sheetNames.card).find((row) => row.field === key);
    return found === undefined ? '' : text(found, 'value');
  };
  for (const unsupported of ['exclusions', 'manifest_dependencies']) {
    if (sheet(unsupported).length > 0) throw new NotRepresentable(`表单不表达 ${unsupported}`);
  }

  const mainTableId = card('rateTableId');
  const tables = sheet(sheetNames.tables);
  const header = tables.find((row) => text(row, 'tableId') === mainTableId);
  if (header === undefined) throw new NotRepresentable('没有主价表');

  Object.assign(state.basics, {
    planId: card('planId'),
    planVersion: card('planVersion'),
    scope: card('scope'),
    direction: card('direction'),
    purpose: card('purpose'),
    aggregation: card('aggregation'),
    baseChargeCode: card('baseChargeCode'),
    currency: text(header, 'currency'),
    weightUnit: text(header, 'weightUnit'),
    periodStartsAt: card('periodStartsAt'),
    periodEndsAt: card('periodEndsAt'),
    authId: card('directionAuthorizationId'),
    authVersion: card('directionAuthorizationVersion'),
  });
  hints.table = mainTableId;
  hints.tableVersion = text(header, 'tableVersion');
  hints.weight = card('weightPolicyId');
  hints.weightVersion = card('weightPolicyVersion');

  // 基础运费价表。
  const family = text(header, 'family');
  state.base.family = family;
  const baseSheet = familySheet[family];
  state.base.rows = sheet(baseSheet)
    .filter((row) => text(row, 'tableId') === mainTableId)
    .map((row) => rateRowOf(row, hints, (uid) => `rate:${uid}`));

  // 重量规则。
  state.weight.method = card('weightMethod');
  state.weight.divisor = card('volumetricDivisor');
  state.weight.lengthUnit = card('volumetricLengthUnit');
  const segments = (policy: string): RoundingSegment[] => {
    const rows = sheet(sheetNames.weightRounding).filter((row) => text(row, 'policy') === policy);
    return rows.length === 0
      ? [newSegment()]
      : rows.map((row) => newSegment({ mode: text(row, 'mode'), increment: text(row, 'increment'), maximum: text(row, 'maximum') }));
  };
  state.weight.chargeable = segments('CHARGEABLE');
  state.weight.volumetric = segments('VOLUMETRIC');

  if (card('amountRoundingMode') !== '') {
    state.amountRounding = {
      enabled: true,
      mode: card('amountRoundingMode'),
      increment: card('amountRoundingIncrement'),
      points: card('amountRoundingPoints').split(';'),
    };
  }

  const bounds = sheet(sheetNames.totalBounds)[0];
  if (bounds !== undefined) state.bounds = { minimum: text(bounds, 'minimum'), maximum: text(bounds, 'maximum') };

  for (const row of sheet(sheetNames.referenceCatalogues)) {
    if (text(row, 'kind') === 'ZONE') state.refs.zoneCatalogue = text(row, 'catalogueId');
    if (text(row, 'kind') === 'REMOTE_TIER') state.refs.remoteTierCatalogue = text(row, 'catalogueId');
  }

  // 费用项：先固定费用，再附加费，各按表里的先后。
  for (const row of sheet(sheetNames.fixedCharges)) {
    if (text(row, 'effect') !== 'ADD' || text(row, 'unit') !== '') throw new NotRepresentable('固定费用只表达按计价对象加收');
    const fee = blankFee('FIXED');
    fee.name = text(row, 'description');
    fee.code = text(row, 'chargeCode');
    fee.codeEdited = true;
    fee.amount = text(row, 'amount');
    hints[`fee:${fee.uid}:rule`] = text(row, 'ruleId');
    state.fees.push(fee);
  }

  const conditions = sheet(sheetNames.conditions);
  const calculations = sheet(sheetNames.calculations);
  const dependencies = sheet(sheetNames.chargeDependencies);
  const seriesBindings = sheet(sheetNames.referenceSeries);
  for (const rule of sheet(sheetNames.surcharges)) {
    if (text(rule, 'effect') !== 'ADD' || text(rule, 'unit') !== '' || text(rule, 'minimumWeightId') !== '') {
      throw new NotRepresentable('附加费只表达按计价对象加收、不带条件最低计价重量');
    }
    const calc = calculations.find((row) => text(row, 'nodeId') === text(rule, 'calculationId'));
    if (calc === undefined || text(calc, 'parentId') !== '') throw new NotRepresentable('计算树只表达单个根节点');
    const method = text(calc, 'method');
    const kind: FeeKind | undefined = (
      { FIXED_AMOUNT: 'CONDITIONAL', PERCENT_OF_BASIS: 'PERCENT', TABLE_LOOKUP: 'TABLE_ZONE', TABLE_LOOKUP_BY_REMOTE_TIER: 'TABLE_REMOTE' } as Record<
        string,
        FeeKind
      >
    )[method];
    if (kind === undefined) throw new NotRepresentable(`计算类型 ${method} 表单不表达`);

    const fee = blankFee(kind);
    fee.name = text(rule, 'description');
    fee.code = text(rule, 'chargeCode');
    fee.codeEdited = true;
    hints[`fee:${fee.uid}:rule`] = text(rule, 'ruleId');
    hints[`fee:${fee.uid}:calc`] = text(calc, 'nodeId');
    if (text(rule, 'exclusivity') === 'GROUPED') {
      fee.groupName = text(rule, 'exclusivityGroup');
      fee.priority = text(rule, 'priority');
    }

    // 触发条件：一个判定，或一层「全部 / 任一」套判定。
    const rootId = text(rule, 'conditionId');
    hints[`fee:${fee.uid}:cond`] = rootId;
    const root = conditions.find((row) => text(row, 'nodeId') === rootId);
    if (root === undefined) throw new NotRepresentable('触发条件不存在');
    const rows: ConditionRow[] = [];
    const predicateRow = (node: Row, hintSlot: (uid: string) => string): ConditionRow => {
      if (text(node, 'kind') !== 'PREDICATE') throw new NotRepresentable('条件只表达一层组合');
      const measure = featureMeasure(text(node, 'source'));
      const row = newCondition({
        source: text(node, 'source'),
        operator: text(node, 'operator'),
        value: measure === 'category' ? text(node, 'category') : text(node, 'threshold'),
        unit: text(node, 'unit'),
      });
      hints[hintSlot(row.uid)] = text(node, 'nodeId');
      return row;
    };
    if (text(root, 'kind') === 'PREDICATE') {
      rows.push(predicateRow(root, () => `fee:${fee.uid}:cond`));
    } else {
      fee.combine = text(root, 'kind') as 'ALL_OF' | 'ANY_OF';
      conditions
        .filter((row) => text(row, 'parentId') === rootId)
        .forEach((child, index) => {
          rows.push(
            predicateRow(child, (uid) => (kind === 'TABLE_REMOTE' ? `fee:${fee.uid}:cond:tier-${index}` : `fee:${fee.uid}:cond:${uid}`)),
          );
        });
    }
    if (kind !== 'TABLE_REMOTE') fee.conditions = rows;

    switch (kind) {
      case 'CONDITIONAL':
        fee.amount = text(calc, 'amount');
        break;
      case 'PERCENT': {
        const depId = text(calc, 'basisDependencyId');
        hints[`fee:${fee.uid}:dep`] = depId;
        const dependency = dependencies.find((row) => text(row, 'dependencyId') === depId);
        if (dependency === undefined) throw new NotRepresentable('基数依赖不存在');
        if (text(dependency, 'composition') === 'ALL_CHARGES' && text(dependency, 'excludes') === '') fee.basis = 'ALL';
        else if (text(dependency, 'composition') === 'LISTED_CHARGES' && text(dependency, 'includes') === state.basics.baseChargeCode && text(dependency, 'excludes') === '') {
          fee.basis = 'BASE';
        } else throw new NotRepresentable('百分比的基数只表达「基础运费」或「其他全部费用」');
        if (text(calc, 'seriesKind') !== '') {
          if (text(calc, 'seriesKind') !== 'FUEL_RATE') throw new NotRepresentable('只表达燃油费率序列');
          fee.percentMode = 'SERIES';
          fee.seriesFactor = text(calc, 'seriesFactor');
          const fuel = seriesBindings.filter((row) => text(row, 'kind') === 'FUEL_RATE');
          if (fuel.length !== 1) throw new NotRepresentable('燃油序列绑定不唯一');
          fee.seriesId = text(fuel[0], 'seriesId');
        } else {
          fee.percentMode = 'FIXED';
          fee.percentage = text(calc, 'percentage');
        }
        break;
      }
      case 'TABLE_ZONE':
      case 'TABLE_REMOTE': {
        const tableId = text(calc, 'tableId');
        hints[`fee:${fee.uid}:table`] = tableId;
        const itemHeader = tables.find((row) => text(row, 'tableId') === tableId);
        if (itemHeader === undefined || text(itemHeader, 'family') !== 'WEIGHT_ZONE') throw new NotRepresentable('附加费价表只表达按分区 / 档位与重量段');
        hints[`fee:${fee.uid}:tableVersion`] = text(itemHeader, 'tableVersion');
        fee.tableRows = sheet(sheetNames.ratesWeightZone)
          .filter((row) => text(row, 'tableId') === tableId)
          .map((row) => rateRowOf(row, hints, (uid) => `fee:${fee.uid}:rate:${uid}`));
        if (kind === 'TABLE_REMOTE') {
          const tiers = [...new Set(fee.tableRows.map((row) => row.cells.zone))];
          const triggered = rows.map((row) => row.value);
          if (tiers.join('|') !== triggered.join('|') || rows.some((row) => row.source !== 'ADDRESS_TYPE' || row.operator !== 'EQ')) {
            throw new NotRepresentable('偏远档位的触发条件须恰是价表里的各档位');
          }
        }
        break;
      }
      default:
        break;
    }
    state.fees.push(fee);
  }
  return state;
}

function blankFee(kind: FeeKind): FeeItemState {
  return {
    uid: nextUid('f'),
    kind,
    name: '',
    code: '',
    codeEdited: false,
    amount: '',
    percentMode: 'SERIES',
    seriesId: '',
    seriesFactor: '1',
    percentage: '',
    basis: 'BASE',
    tableRows: [],
    combine: 'ALL_OF',
    conditions: [],
    groupName: '',
    priority: '',
  };
}

function rateRowOf(row: Row, hints: Record<string, string>, slotOf: (uid: string) => string): RateRow {
  const cells: Record<string, string> = {};
  for (const [key, value] of Object.entries(row)) {
    if (key === 'tableId' || key === 'entryId' || value === null || value === '') continue;
    cells[key] = String(value);
  }
  const made = newRateRow(cells);
  hints[slotOf(made.uid)] = String(row.entryId);
  return made;
}
