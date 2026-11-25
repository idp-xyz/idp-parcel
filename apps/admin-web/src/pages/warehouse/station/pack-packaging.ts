// 打包选包材（纯 TypeScript，node:test 直接测，读口作参数传入）：扫进来的包材码登没登记、在不在用，和点选用的那份列表。
//
// 到 Go 端核过的：打包口只把包材当文本记下、不查册（registrationjson.PackCommand 头注），所以「包材要在用、要在册」是这个工位自己的规矩：
// 旧作业台打包表单的包材下拉也只列在用的（resolveLookupOptions 的 packaging）。包材册读口声明的筛选维只有 deactivated，q 覆盖包材编码与
// 名称（字面包含、不分大小写）。
//
// 扫到的码走定向的读，认出编码完全相同的那一行（catalogue-seek.ts），不拿名册第一页当依据；点选的列表按「在用」筛，最多翻几页，
// 没翻完要在屏上如实说只列了前几个、其余请扫码。

import type { CatalogueConditions } from '../../catalogue-query';
import { listWarehouseCatalogue, type CatalogueListBody, type PackagingRecord } from '../api';
import { seek, type SeekWords } from './catalogue-seek';
import type { PackagingMaterial } from './pack-parcels';
import type { CatalogueReader } from './pick-staging-check';
import { describeReadFailure } from './work-fact-submit';

export function materialOf(record: PackagingRecord): PackagingMaterial {
  return {
    code: record.packagingMaterialCode,
    name: record.name,
    length: record.dimensions?.length ?? '',
    width: record.dimensions?.width ?? '',
    height: record.dimensions?.height ?? '',
    dimensionUnit: record.dimensions?.unit ?? '',
    weightUnit: record.weight?.unit ?? '',
    deactivated: record.deactivated,
  };
}

export type PackagingLookup =
  | { kind: 'found'; material: PackagingMaterial }
  /** 问完了没有这个包材。 */
  | { kind: 'absent' }
  /** 没问成，说不出有没有。 */
  | { kind: 'unverified'; reason: string };

export interface PackagingFinder {
  find(code: string): Promise<PackagingLookup>;
}

/** 一次定向的读最多翻几页（每页读口至多 200 行）。 */
export const MAX_PACKAGING_PAGES = 10;

// 读口对检索词的长度上限（后端 cataloguepage 的 MaxKeywordRunes），按字符计；超过它整个请求答 400，这里就不问了。
const keywordLimit = 100;

const packagingWords: SeekWords = {
  failed: (cause) => `读包材名册没成：${cause}`,
  shape: '读包材名册得到的答复不是名册的样子',
  tooMany: (maxPages) => `包材名册里含这个码的行太多，翻了 ${maxPages} 页还没认出它`,
};

/** 不加筛选地找：停用的包材也认得出来，才好说「已停用」，而不是说成没有。 */
export function createPackagingFinder(read: CatalogueReader, maxPages: number = MAX_PACKAGING_PAGES): PackagingFinder {
  return {
    async find(code) {
      const length = [...code].length;
      if (length > keywordLimit) {
        return {
          kind: 'unverified',
          reason: `这个码有 ${length} 个字符，比读口收的检索词（最多 ${keywordLimit} 个字符）还长，没法去包材名册里找它`,
        };
      }
      const found = await seek<CatalogueListBody, PackagingRecord>({
        read: (after) => read({ selectors: { family: 'packaging' }, q: code }, after),
        rowsOf: (body) => body.packagingMaterials,
        isIt: (row) => row.packagingMaterialCode === code,
        words: packagingWords,
        maxPages,
      });
      if (found.kind === 'unread') return { kind: 'unverified', reason: found.reason };
      return found.kind === 'found' ? { kind: 'found', material: materialOf(found.row) } : { kind: 'absent' };
    },
  };
}

/** 接真读口的那一份。 */
export function livePackagingFinder(): PackagingFinder {
  return createPackagingFinder(listWarehouseCatalogue);
}

// —— 点选用的列表 ——

export interface PackagingList {
  items: readonly PackagingMaterial[];
  /** 翻到页数上限，读口还有下一页：列出来的不是全部。 */
  truncated: boolean;
}

export type PackagingListAnswer = { kind: 'list'; list: PackagingList } | { kind: 'failed'; reason: string };

/** 点选列表最多翻几页。包材一般只有几十种，翻五页（一千种）还没完就让人扫码。 */
export const MAX_PACKAGING_LIST_PAGES = 5;

export async function loadPackagingList(read: CatalogueReader, maxPages: number = MAX_PACKAGING_LIST_PAGES): Promise<PackagingListAnswer> {
  const conditions: CatalogueConditions = { selectors: { family: 'packaging' }, filters: { deactivated: ['false'] } };
  const items: PackagingMaterial[] = [];
  let after: string | null = null;
  for (let pages = 1; ; pages += 1) {
    const answer = await read(conditions, after);
    if (answer.kind !== 'outcome') return { kind: 'failed', reason: describeReadFailure(answer) };
    const rows = answer.body.packagingMaterials;
    const next = answer.body.page?.next;
    if (!Array.isArray(rows) || next === undefined) return { kind: 'failed', reason: '读包材名册得到的答复不是名册的样子' };
    for (const row of rows) {
      if (!row.deactivated) items.push(materialOf(row));
    }
    if (next === null) return { kind: 'list', list: { items, truncated: false } };
    if (pages >= maxPages) return { kind: 'list', list: { items, truncated: true } };
    after = next;
  }
}

export function liveLoadPackagingList(): Promise<PackagingListAnswer> {
  return loadPackagingList(listWarehouseCatalogue);
}
