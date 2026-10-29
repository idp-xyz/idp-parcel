// 表单状态 → 结构化工作簿（ADR-0349 决定四）。服务端对这份 JSON 与对一份 Excel 模板读法相同，所以这里生成的是
// 「一份填好的模板」：表名与列键逐字等于规格口，没填的格缺席，card 表字段列全。
//
// 模板里的标识（价表、价表行、规则、条件节点、依赖……）互相引用，运营不该也不必去配：这里按方案标识和序号推出来
// （idOf 的兜底值），表单页面上看不到任何标识。状态里的 idHints 只给由现成工作簿还原的场合用。
//
// 生成的同时记下「每一行来自表单哪个控件」（origins），预览返回的问题按（表、行、列）查回控件，见 problems.ts。
// 问题行号 = 数组下标 + 2（第 1 行是列键行）。

import type { PriceCardTemplateSpec, PriceCardWorkbook, PriceCardWorkbookCell } from '../api';
import { normalizeMoment } from '../series-form';
import { field } from './fields';
import { featureMeasure } from '../price-card-content';
import { feeHasGroup, type CardFormState, type ConditionRow, type FeeItemState, type RateRow } from './model';
import { cardFieldKeys, columnTitle, familySheet, sheetNames } from './spec';

export interface Target {
  field: string;
  label: string;
  /** 这个控件所在位置的人话；缺省用所在行的位置。基本信息里的格被别的表引用（币种、重量单位）时靠它说回「基本信息」。 */
  path?: string;
}

/** 一行工作簿数据来自表单的哪里：运营看得懂的位置，以及每一列对应的控件。 */
export interface Origin {
  /** 位置的人话，如「费用项 2「超尺寸费」· 条件 1」。 */
  path: string;
  cells: Record<string, Target>;
  /** 列对不上具体控件时落的控件。 */
  fallback: Target;
}

export interface OriginIndex {
  rows: Record<string, Origin[]>;
  /** card 表的问题以字段名为列。 */
  cardFields: Record<string, Target>;
  /** 表层面的问题（行号 0）与找不到行的问题落这里。 */
  sheets: Record<string, { path: string; target: Target }>;
}

export interface GeneratedWorkbook {
  workbook: PriceCardWorkbook;
  origins: OriginIndex;
}

type Cells = Record<string, string | undefined>;

const trim = (value: string) => value.trim();

function weightUnitOf(state: CardFormState) {
  return trim(state.basics.weightUnit);
}

/** 费用项在位置串里的写法：「费用项 2「超尺寸费」」。 */
export function feePath(index: number, fee: FeeItemState): string {
  const name = trim(fee.name) || trim(fee.code);
  return name === '' ? `费用项 ${index + 1}` : `费用项 ${index + 1}「${name}」`;
}

/** 偏远档位价表里出现的档位，按行序去重。 */
export function remoteTiers(fee: FeeItemState): string[] {
  const tiers: string[] = [];
  for (const row of fee.tableRows) {
    const tier = trim(row.cells.zone ?? '');
    if (tier !== '' && !tiers.includes(tier)) tiers.push(tier);
  }
  return tiers;
}

export function formToWorkbook(state: CardFormState, spec: PriceCardTemplateSpec): GeneratedWorkbook {
  const { basics } = state;
  const sheets: Record<string, Record<string, PriceCardWorkbookCell>[]> = {};
  const origins: OriginIndex = { rows: {}, cardFields: {}, sheets: {} };
  const planId = trim(basics.planId);
  const planVersion = trim(basics.planVersion);
  const idOf = (slot: string, fallback: string) => state.idHints[slot] ?? fallback;

  const add = (sheet: string, cells: Cells, origin: Origin) => {
    const row: Record<string, PriceCardWorkbookCell> = {};
    for (const [key, value] of Object.entries(cells)) {
      if (value !== undefined && value !== '') row[key] = value;
    }
    (sheets[sheet] ??= []).push(row);
    (origins.rows[sheet] ??= []).push(origin);
  };
  const defaultSheet = (sheet: string, path: string, target: Target) => {
    origins.sheets[sheet] = { path, target };
  };

  const startsAt = normalizeMoment(basics.periodStartsAt);
  const endsAt = normalizeMoment(basics.periodEndsAt);
  const currency = trim(basics.currency);
  const unit = weightUnitOf(state);

  const basicsTarget = (key: string, label: string): Target => ({ field: field.basics(key), label, path: '基本信息' });

  // ---- 主价表 ----
  const tableId = idOf('table', `${planId}-TABLE`);
  const tableVersion = idOf('tableVersion', planVersion);
  const family = trim(state.base.family);
  const baseTargetOf = (sheet: string, column: string, rowUid?: string): Target =>
    rowUid === undefined
      ? { field: field.base('family'), label: '价表类型' }
      : { field: field.baseRow(rowUid, column), label: columnTitle(spec, sheet, column) };

  add(
    sheetNames.tables,
    { tableId, tableVersion, family, currency, weightUnit: unit, periodStartsAt: startsAt, periodEndsAt: endsAt },
    {
      path: '基础运费',
      fallback: { field: field.base('family'), label: '价表类型' },
      cells: {
        family: { field: field.base('family'), label: '价表类型' },
        currency: basicsTarget('currency', '币种'),
        weightUnit: basicsTarget('weightUnit', '重量单位'),
        periodStartsAt: basicsTarget('periodStartsAt', '生效起点'),
        periodEndsAt: basicsTarget('periodEndsAt', '生效止点'),
      },
    },
  );
  const rateSheet = familySheet[family];
  if (rateSheet !== undefined) {
    state.base.rows.forEach((row, index) => {
      const cells: Cells = { tableId, entryId: idOf(`rate:${row.uid}`, `${planId}-RATE-${index + 1}`) };
      const targets: Record<string, Target> = {};
      for (const [column, value] of Object.entries(row.cells)) {
        cells[column] = trim(value);
        targets[column] = { field: field.baseRow(row.uid, column), label: columnTitle(spec, rateSheet, column) };
      }
      add(rateSheet, cells, {
        path: `基础运费 · 第 ${index + 1} 行`,
        cells: targets,
        fallback: baseTargetOf(rateSheet, 'zone', row.uid),
      });
    });
    defaultSheet(rateSheet, '基础运费', baseTargetOf('', ''));
  }
  defaultSheet(sheetNames.tables, '基础运费', baseTargetOf('', ''));

  // ---- 计重进位段 ----
  const addSegments = (policy: 'CHARGEABLE' | 'VOLUMETRIC', key: 'chargeable' | 'volumetric', path: string) => {
    state.weight[key].forEach((segment, index) => {
      add(
        sheetNames.weightRounding,
        { policy, mode: trim(segment.mode), increment: trim(segment.increment), maximum: trim(segment.maximum), unit },
        {
          path: `${path} · 第 ${index + 1} 段`,
          fallback: { field: field.segment(key, segment.uid, 'mode'), label: '进位模式' },
          cells: {
            policy: { field: field.segment(key, segment.uid, 'mode'), label: '进位模式' },
            mode: { field: field.segment(key, segment.uid, 'mode'), label: '进位模式' },
            increment: { field: field.segment(key, segment.uid, 'increment'), label: '进位单位' },
            maximum: { field: field.segment(key, segment.uid, 'maximum'), label: '本段上界' },
            unit: basicsTarget('weightUnit', '重量单位'),
          },
        },
      );
    });
  };
  const weightMax = trim(state.weight.method) === 'MAX';
  addSegments('CHARGEABLE', 'chargeable', '计价重量进位');
  if (weightMax) addSegments('VOLUMETRIC', 'volumetric', '体积重进位');
  defaultSheet(sheetNames.weightRounding, '重量规则', { field: field.weight('method'), label: '计重方法' });

  // ---- 费用项 ----
  const fixedCount = { value: 0 };
  const seriesBound: string[] = [];
  state.fees.forEach((fee, index) => {
    const n = index + 1;
    const path = feePath(index, fee);
    const nameTarget: Target = { field: field.fee(fee.uid, 'name'), label: '名称' };
    const codeTarget: Target = { field: field.fee(fee.uid, 'code'), label: '费用代码' };
    const amountTarget: Target = { field: field.fee(fee.uid, 'amount'), label: '金额' };
    const chargeCode = trim(fee.code);
    const description = trim(fee.name);

    if (fee.kind === 'FIXED') {
      fixedCount.value += 1;
      add(
        sheetNames.fixedCharges,
        {
          ruleId: idOf(`fee:${fee.uid}:rule`, `${planId}-RULE-${n}`),
          chargeCode,
          description,
          effect: 'ADD',
          amount: trim(fee.amount),
          order: String(fixedCount.value),
        },
        { path, fallback: nameTarget, cells: { chargeCode: codeTarget, description: nameTarget, amount: amountTarget } },
      );
      return;
    }

    // 触发条件：偏远档位由价表里的档位推出，其余取条件构造器里的行。
    const predicates: ConditionRow[] =
      fee.kind === 'TABLE_REMOTE'
        ? remoteTiers(fee).map((tier, tierIndex) => ({
            uid: `tier-${tierIndex}`,
            source: 'ADDRESS_TYPE',
            operator: 'EQ',
            value: tier,
            unit: '',
          }))
        : fee.conditions;
    const conditionRoot = idOf(`fee:${fee.uid}:cond`, `${planId}-COND-${n}`);
    const conditionTarget: Target = { field: field.fee(fee.uid, 'conditions'), label: '触发条件' };
    const grouped = predicates.length > 1;
    if (grouped) {
      add(
        sheetNames.conditions,
        { nodeId: conditionRoot, kind: fee.kind === 'TABLE_REMOTE' ? 'ANY_OF' : fee.combine },
        { path: `${path} · 条件组合`, fallback: conditionTarget, cells: { kind: { field: field.fee(fee.uid, 'combine'), label: '条件组合' } } },
      );
    }
    predicates.forEach((row, rowIndex) => {
      const nodeId = grouped
        ? idOf(`fee:${fee.uid}:cond:${row.uid}`, `${planId}-COND-${n}-${rowIndex + 1}`)
        : conditionRoot;
      const numeric = featureMeasure(row.source) !== 'category';
      const control = (key: string, label: string): Target =>
        fee.kind === 'TABLE_REMOTE'
          ? { field: field.feeRow(fee.uid, fee.tableRows[0]?.uid ?? '', 'zone'), label: '档位' }
          : { field: field.condition(fee.uid, row.uid, key), label };
      add(
        sheetNames.conditions,
        {
          nodeId,
          parentId: grouped ? conditionRoot : undefined,
          kind: 'PREDICATE',
          source: trim(row.source),
          operator: trim(row.operator),
          threshold: numeric ? trim(row.value) : undefined,
          unit: numeric ? trim(row.unit) : undefined,
          category: numeric ? undefined : trim(row.value),
        },
        {
          path: `${path} · 条件 ${rowIndex + 1}`,
          fallback: control('source', '特征'),
          cells: {
            source: control('source', '特征'),
            operator: control('operator', '比较'),
            threshold: control('value', '比较值'),
            category: control('value', '取值'),
            unit: control('unit', '单位'),
          },
        },
      );
    });

    // 计算方式。
    const calcId = idOf(`fee:${fee.uid}:calc`, `${planId}-CALC-${n}`);
    const calcOrigin = (cells: Record<string, Target>): Origin => ({ path, fallback: nameTarget, cells });
    let calcCells: Cells;
    let calcTargets: Record<string, Target>;
    switch (fee.kind) {
      case 'CONDITIONAL':
        calcCells = { nodeId: calcId, method: 'FIXED_AMOUNT', amount: trim(fee.amount) };
        calcTargets = { amount: amountTarget };
        break;
      case 'PERCENT': {
        const depId = idOf(`fee:${fee.uid}:dep`, `${planId}-DEP-${n}`);
        const seriesMode = fee.percentMode === 'SERIES';
        calcCells = seriesMode
          ? { nodeId: calcId, method: 'PERCENT_OF_BASIS', basisDependencyId: depId, seriesKind: 'FUEL_RATE', seriesFactor: trim(fee.seriesFactor) }
          : { nodeId: calcId, method: 'PERCENT_OF_BASIS', basisDependencyId: depId, percentage: trim(fee.percentage) };
        calcTargets = {
          percentage: { field: field.fee(fee.uid, 'percentage'), label: '百分比' },
          seriesFactor: { field: field.fee(fee.uid, 'seriesFactor'), label: '系数' },
          seriesKind: { field: field.fee(fee.uid, 'seriesId'), label: '燃油费率序列' },
          basisDependencyId: { field: field.fee(fee.uid, 'basis'), label: '计算基数' },
        };
        add(
          sheetNames.chargeDependencies,
          basisCells(fee, depId, trim(basics.baseChargeCode)),
          {
            path,
            fallback: { field: field.fee(fee.uid, 'basis'), label: '计算基数' },
            cells: { chargeCode: codeTarget, includes: { field: field.fee(fee.uid, 'basis'), label: '计算基数' } },
          },
        );
        if (seriesMode) {
          const seriesId = trim(fee.seriesId);
          if (!seriesBound.includes(seriesId)) {
            seriesBound.push(seriesId);
            add(
              sheetNames.referenceSeries,
              { kind: 'FUEL_RATE', seriesId },
              {
                path,
                fallback: { field: field.fee(fee.uid, 'seriesId'), label: '燃油费率序列' },
                cells: { seriesId: { field: field.fee(fee.uid, 'seriesId'), label: '燃油费率序列' } },
              },
            );
          }
        }
        break;
      }
      case 'TABLE_ZONE':
      case 'TABLE_REMOTE': {
        const itemTable = idOf(`fee:${fee.uid}:table`, `${planId}-TABLE-${n}`);
        calcCells = {
          nodeId: calcId,
          method: fee.kind === 'TABLE_REMOTE' ? 'TABLE_LOOKUP_BY_REMOTE_TIER' : 'TABLE_LOOKUP',
          tableId: itemTable,
        };
        calcTargets = { method: { field: field.fee(fee.uid, 'rows'), label: '价表' }, tableId: { field: field.fee(fee.uid, 'rows'), label: '价表' } };
        if (fee.kind === 'TABLE_REMOTE') {
          calcTargets.method = { field: field.refs('remoteTierCatalogue'), label: '偏远档位目录' };
        }
        add(
          sheetNames.tables,
          {
            tableId: itemTable,
            tableVersion: idOf(`fee:${fee.uid}:tableVersion`, planVersion),
            family: 'WEIGHT_ZONE',
            currency,
            weightUnit: unit,
            periodStartsAt: startsAt,
            periodEndsAt: endsAt,
          },
          {
            path: `${path} · 价表`,
            fallback: { field: field.fee(fee.uid, 'rows'), label: '价表' },
            cells: {
              currency: basicsTarget('currency', '币种'),
              weightUnit: basicsTarget('weightUnit', '重量单位'),
              periodStartsAt: basicsTarget('periodStartsAt', '生效起点'),
              periodEndsAt: basicsTarget('periodEndsAt', '生效止点'),
            },
          },
        );
        fee.tableRows.forEach((row: RateRow, rowIndex) => {
          const cells: Cells = { tableId: itemTable, entryId: idOf(`fee:${fee.uid}:rate:${row.uid}`, `${planId}-RATE-${n}-${rowIndex + 1}`) };
          const targets: Record<string, Target> = {};
          for (const [column, value] of Object.entries(row.cells)) {
            cells[column] = trim(value);
            targets[column] = {
              field: field.feeRow(fee.uid, row.uid, column),
              label: column === 'zone' && fee.kind === 'TABLE_REMOTE' ? '档位' : columnTitle(spec, sheetNames.ratesWeightZone, column),
            };
          }
          add(sheetNames.ratesWeightZone, cells, {
            path: `${path} · 价表第 ${rowIndex + 1} 行`,
            cells: targets,
            fallback: { field: field.feeRow(fee.uid, row.uid, 'zone'), label: fee.kind === 'TABLE_REMOTE' ? '档位' : '分区' },
          });
        });
        break;
      }
      default:
        throw new Error(`未知的费用项类型 ${String(fee.kind)}`);
    }
    add(sheetNames.calculations, calcCells, calcOrigin(calcTargets));

    // 附加费规则。
    const groupName = trim(fee.groupName);
    const hasGroup = feeHasGroup(fee.kind) && groupName !== '';
    add(
      sheetNames.surcharges,
      {
        ruleId: idOf(`fee:${fee.uid}:rule`, `${planId}-RULE-${n}`),
        chargeCode,
        description,
        effect: 'ADD',
        conditionId: conditionRoot,
        calculationId: calcId,
        exclusivity: hasGroup ? 'GROUPED' : 'STANDALONE',
        exclusivityGroup: hasGroup ? groupName : undefined,
        priority: hasGroup ? trim(fee.priority) : undefined,
      },
      {
        path,
        fallback: nameTarget,
        cells: {
          chargeCode: codeTarget,
          description: nameTarget,
          conditionId: conditionTarget,
          calculationId: amountTarget,
          exclusivity: { field: field.fee(fee.uid, 'group'), label: '互斥组' },
          exclusivityGroup: { field: field.fee(fee.uid, 'group'), label: '互斥组' },
          priority: { field: field.fee(fee.uid, 'priority'), label: '优先级' },
        },
      },
    );
  });
  defaultSheet(sheetNames.surcharges, '其他费用项', { field: 'step.fees', label: '费用项' });
  defaultSheet(sheetNames.conditions, '其他费用项', { field: 'step.fees', label: '费用项' });
  defaultSheet(sheetNames.calculations, '其他费用项', { field: 'step.fees', label: '费用项' });
  defaultSheet(sheetNames.fixedCharges, '其他费用项', { field: 'step.fees', label: '费用项' });
  defaultSheet(sheetNames.chargeDependencies, '其他费用项', { field: 'step.fees', label: '费用项' });
  defaultSheet(sheetNames.referenceSeries, '其他费用项', { field: 'step.fees', label: '费用项' });

  // ---- 总额上下限 ----
  const minimum = trim(state.bounds.minimum);
  const maximum = trim(state.bounds.maximum);
  if (minimum !== '' || maximum !== '') {
    add(
      sheetNames.totalBounds,
      { minimum, maximum },
      {
        path: '总额上下限',
        fallback: { field: field.bounds('minimum'), label: '最低收费' },
        cells: {
          minimum: { field: field.bounds('minimum'), label: '最低收费' },
          maximum: { field: field.bounds('maximum'), label: '最高收费' },
        },
      },
    );
  }
  defaultSheet(sheetNames.totalBounds, '总额上下限', { field: field.bounds('minimum'), label: '最低收费' });

  // ---- 参考目录 ----
  const zone = trim(state.refs.zoneCatalogue);
  const remote = trim(state.refs.remoteTierCatalogue);
  const catalogueOrigin = (key: 'zoneCatalogue' | 'remoteTierCatalogue', label: string): Origin => ({
    path: '参考数据',
    fallback: { field: field.refs(key), label },
    cells: { catalogueId: { field: field.refs(key), label }, kind: { field: field.refs(key), label } },
  });
  if (zone !== '') add(sheetNames.referenceCatalogues, { kind: 'ZONE', catalogueId: zone }, catalogueOrigin('zoneCatalogue', '分区目录'));
  if (remote !== '') {
    add(sheetNames.referenceCatalogues, { kind: 'REMOTE_TIER', catalogueId: remote }, catalogueOrigin('remoteTierCatalogue', '偏远档位目录'));
  }
  defaultSheet(sheetNames.referenceCatalogues, '参考数据', { field: field.refs('zoneCatalogue'), label: '分区目录' });

  // ---- card 表：字段列全，没填的写 null ----
  const cardValues: Record<string, string | undefined> = {
    templateVersion: spec.templateVersion,
    planId,
    planVersion,
    scope: trim(basics.scope),
    direction: trim(basics.direction),
    purpose: trim(basics.purpose),
    aggregation: trim(basics.aggregation),
    baseChargeCode: trim(basics.baseChargeCode),
    periodStartsAt: startsAt,
    periodEndsAt: endsAt,
    rateTableId: tableId,
    weightPolicyId: idOf('weight', `${planId}-WEIGHT`),
    weightPolicyVersion: idOf('weightVersion', planVersion),
    weightMethod: trim(state.weight.method),
    volumetricDivisor: weightMax ? trim(state.weight.divisor) : undefined,
    volumetricLengthUnit: weightMax ? trim(state.weight.lengthUnit) : undefined,
    amountRoundingMode: state.amountRounding.enabled ? trim(state.amountRounding.mode) : undefined,
    amountRoundingIncrement: state.amountRounding.enabled ? trim(state.amountRounding.increment) : undefined,
    amountRoundingPoints: state.amountRounding.enabled ? pointsText(state.amountRounding.points, spec) : undefined,
    directionAuthorizationId: trim(basics.authId),
    directionAuthorizationVersion: trim(basics.authVersion),
  };
  const cardTargets: Record<string, Target> = {
    planId: basicsTarget('planId', '方案标识'),
    planVersion: basicsTarget('planVersion', '版本'),
    scope: basicsTarget('scope', '适用范围'),
    direction: basicsTarget('direction', '价格方向'),
    purpose: basicsTarget('purpose', '用途'),
    aggregation: basicsTarget('aggregation', '计价对象'),
    baseChargeCode: basicsTarget('baseChargeCode', '基础运费代码'),
    periodStartsAt: basicsTarget('periodStartsAt', '生效起点'),
    periodEndsAt: basicsTarget('periodEndsAt', '生效止点'),
    rateTableId: { field: field.base('family'), label: '价表类型', path: '基础运费' },
    weightPolicyId: { field: field.weight('method'), label: '计重方法', path: '重量规则' },
    weightPolicyVersion: { field: field.weight('method'), label: '计重方法', path: '重量规则' },
    weightMethod: { field: field.weight('method'), label: '计重方法', path: '重量规则' },
    volumetricDivisor: { field: field.weight('divisor'), label: '体积重除数', path: '重量规则' },
    volumetricLengthUnit: { field: field.weight('lengthUnit'), label: '长度单位', path: '重量规则' },
    amountRoundingMode: { field: field.rounding('mode'), label: '取整方式', path: '金额取整' },
    amountRoundingIncrement: { field: field.rounding('increment'), label: '进位单位', path: '金额取整' },
    amountRoundingPoints: { field: field.rounding('points'), label: '取整的环节', path: '金额取整' },
    directionAuthorizationId: basicsTarget('authId', '价格方向授权标识'),
    directionAuthorizationVersion: basicsTarget('authVersion', '价格方向授权版本'),
  };
  origins.cardFields = cardTargets;
  const cardRows = cardFieldKeys(spec).map((key) => {
    const value = cardValues[key];
    return { field: key, value: value === undefined || value === '' ? null : value } as Record<string, PriceCardWorkbookCell>;
  });
  sheets[sheetNames.card] = cardRows;

  return { workbook: { templateVersion: spec.templateVersion, sheets }, origins };
}

function basisCells(fee: FeeItemState, dependencyId: string, baseChargeCode: string): Cells {
  const chargeCode = trim(fee.code);
  return fee.basis === 'ALL'
    ? { dependencyId, chargeCode, composition: 'ALL_CHARGES' }
    : { dependencyId, chargeCode, composition: 'LISTED_CHARGES', includes: baseChargeCode };
}

/** 应用点按规格口给的先后写成分号串，不按点选的先后：同一份选择永远生成同一份工作簿。 */
function pointsText(points: readonly string[], spec: PriceCardTemplateSpec): string {
  const order = spec.sheets.find((sheet) => sheet.name === sheetNames.card)?.columns.find((column) => column.key === 'amountRoundingPoints')
    ?.allowedValues ?? [];
  const chosen = new Set(points);
  const ordered = order.filter((value) => chosen.has(value));
  const unknown = points.filter((value) => !order.includes(value));
  return [...ordered, ...unknown].join(';');
}
