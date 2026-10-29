// 基础数据页的行模型、检索与分组纯逻辑（ADR-0335 决定七）。node:test 钉着；页面只负责摆。
//
// 三本目录都整表取回、在前端检索与分组：目录是产品内置的几百行常量，不分页、不分租户，读口的 ?q= / ?region= / ?category=
// 留给表单候选之类的后续消费者。检索规则与服务端同口径：按码或名称、不分大小写、子串匹配，空检索不筛。

import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
import { matchesSearch } from '../party/list-order';
import { labelFor } from '../../vocabulary/status';
import type { CountryRecord, CurrencyRecord, UnitRecord } from './api';

const REGION_SCOPE = 'refdata-region';
const UNIT_CATEGORY_SCOPE = 'refdata-unit-category';

/** 大洲码经词典译；集外取值原样显示。 */
export function regionLabel(code: string): string {
  return labelFor(code, REGION_SCOPE);
}

/** 单位类别码经词典译；集外取值原样显示。 */
export function unitCategoryLabel(code: string): string {
  return labelFor(code, UNIT_CATEGORY_SCOPE);
}

/** 筛选胶囊的一格：值、显示词、计数。 */
export interface FilterCount {
  value: string;
  label: string;
  count: number;
}

// 胶囊次序是阅读便利，不含语义；集外的新取值排在已知之后，不丢。镜像后端的封闭词（refdata 包的 Region 与 UnitCategory）。
const regionOrder = ['ASIA', 'EUROPE', 'AFRICA', 'NORTH_AMERICA', 'SOUTH_AMERICA', 'OCEANIA', 'ANTARCTICA'];
const unitCategoryOrder = ['WEIGHT', 'LENGTH', 'VOLUME', 'COUNT'];

function countBy<Row>(
  rows: readonly Row[],
  valueOf: (row: Row) => string,
  labelOf: (value: string) => string,
  order: readonly string[],
): FilterCount[] {
  const counts = new Map<string, number>();
  for (const row of rows) counts.set(valueOf(row), (counts.get(valueOf(row)) ?? 0) + 1);
  const rank = (value: string) => {
    const index = order.indexOf(value);
    return index < 0 ? order.length : index;
  };
  return [...counts.entries()]
    .sort(([left], [right]) => rank(left) - rank(right) || (left < right ? -1 : left > right ? 1 : 0))
    .map(([value, count]) => ({ value, label: labelOf(value), count }));
}

export function countByRegion(rows: readonly CountryRecord[]): FilterCount[] {
  return countBy(rows, (row) => row.region, regionLabel, regionOrder);
}

export function countByUnitCategory(rows: readonly UnitRecord[]): FilterCount[] {
  return countBy(rows, (row) => row.category, unitCategoryLabel, unitCategoryOrder);
}

/** 小数位的人话：0 位是“无小数”（日元、韩元这一类），其余“N 位小数”。 */
export function minorUnitScaleText(scale: number): string {
  return scale === 0 ? '无小数' : `${scale} 位小数`;
}

export function countByMinorUnitScale(rows: readonly CurrencyRecord[]): FilterCount[] {
  return countBy(rows, (row) => String(row.minorUnitScale), (value) => minorUnitScaleText(Number(value)), ['0', '2', '3']);
}

export function filterCountries(rows: readonly CountryRecord[], filter: { search: string; region: string }): CountryRecord[] {
  return rows.filter(
    (row) =>
      (filter.region === 'ALL' || row.region === filter.region) &&
      matchesSearch(filter.search, [row.code, row.nameZh, row.nameEn, regionLabel(row.region)]),
  );
}

export function filterCurrencies(rows: readonly CurrencyRecord[], filter: { search: string; scale: string }): CurrencyRecord[] {
  return rows.filter(
    (row) =>
      (filter.scale === 'ALL' || String(row.minorUnitScale) === filter.scale) &&
      matchesSearch(filter.search, [row.code, row.nameZh, row.nameEn, row.symbol]),
  );
}

export function filterUnits(rows: readonly UnitRecord[], filter: { search: string; category: string }): UnitRecord[] {
  return rows.filter(
    (row) =>
      (filter.category === 'ALL' || row.category === filter.category) &&
      matchesSearch(filter.search, [row.code, row.nameZh, unitCategoryLabel(row.category)]),
  );
}

/** 去掉名称里的括注：“立方米（CBM 写法）”在句子里读作“立方米”。 */
export function shortUnitName(nameZh: string): string {
  return nameZh.replace(/（[^）]*）/g, '').trim();
}

/**
 * 换算一格的人话：基准单位写“基准单位”；与基准同值的别名写“与立方米同值”；其余“1 磅 = 0.45359237 千克”。
 * 系数是精确十进制文本，原样接进句子，不经浮点、不取整。基准单位不在传入的表里时退回它的码。
 */
export function unitConversionText(unit: UnitRecord, all: readonly UnitRecord[]): string {
  if (unit.code === unit.baseUnit) return '基准单位';
  const base = all.find((row) => row.code === unit.baseUnit);
  const baseName = base === undefined ? unit.baseUnit : shortUnitName(base.nameZh);
  if (unit.factorToBase === '1') return `与${baseName}同值`;
  return `1 ${shortUnitName(unit.nameZh)} = ${unit.factorToBase} ${baseName}`;
}

/** 工具条右侧的计数句：没筛时只报总数，筛过了报“显示 N / 共 M”。 */
export function countSummary(total: number, shown: number, noun: string): string {
  return shown === total ? `共 ${total} ${noun}` : `显示 ${shown} / 共 ${total} ${noun}`;
}

export function noMatchNote(noun: string): string {
  return `没有匹配的${noun}。换个关键词，或清除筛选。`;
}

/** 三本目录共用的页面态：目录是内置常量，空表只可能是读口出了毛病，如实说，不虚构。 */
export function referenceViewState<Body>(
  answer: ApiResult<Body> | null,
  rowCount: number,
  retry: () => void,
  module: ModuleInfo,
  endpoint: string,
  noun: string,
): TemplateViewState {
  return catalogueViewState(answer, rowCount, retry, {
    module,
    endpoint,
    emptyTitle: `${noun}目录是空的`,
    emptyDescription: `${noun}目录随产品发布，不该为空；这不是“还没登记”，请联系产品支持。`,
  });
}
