// 集团与法人页的列表纯逻辑（票 admin-web-group-legal-entities/01 立，票 15 改成工作台口径）。全部是纯函数，node:test 钉着；
// 页面只负责摆。
//
// 检索、状态与注册地筛选、表头排序下推到目录读口（ADR-0144；读口的可排维与筛选维由后端 GroupLegalEntityPages 声明）：
// legalEntityListConditions 编条件，页面按游标翻页。读口按法人标识、参与方标识与名称、注册地与注册号检索；状态词不在其内，
// 要按状态看用状态筛选。「待处理」由身份层与名称转写推出来，读口筛不了，在到手的那一页上筛（legalEntityNeedsAttention）：
// 整册一页装得下时它是精确的，否则只筛本页（complete-register.ts）。

import type { WorkbenchSort } from '../../templates/workbench';
import type { CatalogueConditions } from '../catalogue-query';
import type { GroupLegalEntityRecord, RegistrationNumberTypeRecord } from './api';
import { identityLayerCellsOf } from './legal-entity-identity';
import { identityStatusLabels, type IdentityStatusCode } from './presentation';
import { catalogueSortOf, codeFilterOptions, type CatalogueSortColumns, type CodeFilter } from './list-order';

/** 身份状态封闭三格（domain IdentityStatus 原名，从 identityStatusLabels 的键派生）加「全部」。 */
export type LegalEntityStatusFilter = CodeFilter<IdentityStatusCode>;

// 胶囊的码与词都从 identityStatusLabels 派生——那份词表是 CONTEXT 原词在前端的唯一一处。
export const legalEntityStatusFilterOptions = codeFilterOptions(identityStatusLabels, '全部');

/** 表头可排的列 → 读口声明的可排维。公司主体列不可排：读口不按名称排。 */
export const legalEntitySortColumns: CatalogueSortColumns = {
  'effective-from': 'effectiveFrom',
  'registered-at': 'registeredAt',
};

/** 默认登记时间新→旧（票 01 裁决 2：运营配置员最常看「刚登进去的那条」）。 */
export const defaultLegalEntitySort: WorkbenchSort = { key: 'registered-at', dir: -1 };

/** 法人册一问的条件：检索词、状态与注册地（「全部」即那一维缺席）与表头排序；登记时间新→旧是读口的缺省序，不写进查询串。 */
export function legalEntityListConditions(
  keyword: string,
  status: LegalEntityStatusFilter,
  registrationCountry: string,
  sort: WorkbenchSort,
): CatalogueConditions {
  return {
    q: keyword,
    filters: {
      status: status === 'ALL' ? [] : [status],
      registrationCountry: registrationCountry === 'ALL' ? [] : [registrationCountry],
    },
    sort: catalogueSortOf(sort, legalEntitySortColumns, '-registeredAt'),
  };
}

/**
 * 注册地筛选的选项：注册号类型目录里登过身份层类型的国家（登记时服务端按它判注册地），加上这一页与此刻选中的那一个——
 * 目录只取了第一页、或类型后来撤了，册上照样可能有别的国家。去重、按码排。
 */
export function registrationCountryOptions(
  types: readonly RegistrationNumberTypeRecord[],
  rows: readonly GroupLegalEntityRecord[],
  selected: string,
): string[] {
  const countries = new Set(types.filter((type) => type.layer === 'IDENTITY').map((type) => type.countryCode));
  for (const row of rows) {
    const country = identityLayerCellsOf(row)?.country;
    if (country) countries.add(country);
  }
  if (selected !== 'ALL') countries.add(selected);
  return [...countries].filter((country) => country !== '').sort();
}

/**
 * 待补：最新修订登记于身份层落地之前（注册国家与终身注册号两格没有），或参与方册查无这个身份（名称转写不到）。
 * 两件都要有人去处理，是这张册上唯一「需要人去做点什么」的形态，但处置不同：前者登记下一修订补齐；后者是写入门
 * 失败留下的悬空（api.ts GroupLegalEntityRecord 头注），法人钉着的参与方身份改不了，要去查参与方册。停用是终局，不算待补。
 */
export function legalEntityNeedsAttention(row: GroupLegalEntityRecord): boolean {
  return !row.identityLayerRegistered || !row.partyNameKnown;
}

export interface LegalEntityCounts {
  total: number;
  byStatus: Record<IdentityStatusCode, number>;
  attention: number;
}

/** 工具栏筛选胶囊的计数。只拿整册来数（complete-register.ts 的 completeRegisterRows 判过的那一份）。 */
export function countLegalEntities(rows: readonly GroupLegalEntityRecord[]): LegalEntityCounts {
  const byStatus: Record<IdentityStatusCode, number> = { REGISTERED: 0, EFFECTIVE: 0, DEACTIVATED: 0 };
  let attention = 0;
  for (const row of rows) {
    if (row.status in byStatus) byStatus[row.status as IdentityStatusCode] += 1;
    if (legalEntityNeedsAttention(row)) attention += 1;
  }
  return { total: rows.length, byStatus, attention };
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const legalEntityNoMatchNote = '当前筛选条件下没有匹配的公司主体';
