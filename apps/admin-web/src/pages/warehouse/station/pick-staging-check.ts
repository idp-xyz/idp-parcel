// 拣货工位核放货位：扫到的码能不能当放货位（纯 TypeScript，node:test 直接测，读口作参数传入）。
//
// 规矩取自 application.WaveHandler.Pick，次序也是它的次序：库位登记在这台设备绑的仓、没停用；它所在的库区没停用，
// 且用途是待出库（OUTBOUND_STAGING）。不满足后端答内容不合，这里先在屏上说清是哪一条。
//
// 不拿「读一页名册」当依据：库位册与库区册的行数不设上限，读口一页至多 200 行，第一页里没有不等于没登记。所以每扫到一个码就
// 发定向的读：库位册按仓筛（筛选维 warehouseCode），检索词 q 填这个码；库区册同样按仓筛，q 填库位所在的库区。
// 读口的 q 是对（仓编码、库位编码、所属库区）三列的字面包含、不分大小写（cataloguepage.Binding 的 ILIKE），库区册是（仓编码、
// 库区编码、名称）三列；而库位码、库区码逐字保存、分大小写（domain.requiredValue）。所以答回来的行不一定就是要找的那一个——码是
// 别的码的一部分、或是仓编码库区编码的一部分时会带出一串——要在行里认出码完全相同的那一行，这一页没有就沿 page.next 往下翻，
// 翻到认出，或翻完（读口说没有下一页）。翻页设上限，到了上限还没认出来就说没核成，不说成没有。
//
// 登记册里没有这个库位，只在这个仓里问完没有、别的仓里也问完没有的时候才说。读口没问成（没通、没答、接入被拒、答复不成名册
// 的样子）与翻页到上限，都不是「没有」：判成没核成，不记下这个放货位，让人再扫一次。

import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import { listWarehouseCatalogue, type CatalogueListBody, type LocationRecord, type ZoneRecord } from '../api';
import { zoneUsageLabel } from '../rows';
import { seek, type SeekWords } from './catalogue-seek';

/** 一页一页读名册：交条件与游标，答复原样交回。真读口是 listWarehouseCatalogue，测试传造的。 */
export type CatalogueReader = (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<CatalogueListBody>>;

export type StagingVerdict =
  /** 核得过：能当放货位。 */
  | { kind: 'ok' }
  /** 问到了，不行：reason 是整句（带句号），说清是哪一条不满足。 */
  | { kind: 'rejected'; reason: string }
  /** 没核成：读口没问成或没问完，说不出行不行。reason 是原因，不带句号。 */
  | { kind: 'unverified'; reason: string };

export interface StagingVerifier {
  verify(warehouse: string, code: string): Promise<StagingVerdict>;
}

/** 一次定向的读最多翻几页（每页读口至多 200 行）。包含匹配的结果多得翻不完时，说没核成。 */
export const MAX_STAGING_PAGES = 10;

// 读口对检索词的长度上限（后端 cataloguepage 的 MaxKeywordRunes），按字符计；超过它整个请求答 400，这里就不问了。
const keywordLimit = 100;

/** 说给人听的册名：库位、库区。 */
function registerWords(what: string): SeekWords {
  return {
    failed: (cause) => `读${what}名册没成：${cause}`,
    shape: `读${what}名册得到的答复不是名册的样子`,
    tooMany: (maxPages) => `${what}名册里含这个码的行太多，翻了 ${maxPages} 页还没认出它`,
  };
}

const unverified = (reason: string): StagingVerdict => ({ kind: 'unverified', reason });
const rejected = (reason: string): StagingVerdict => ({ kind: 'rejected', reason });

/** 码长得读口的检索词收不下：说原因；收得下答 null。 */
function overLimit(subject: string, code: string): string | null {
  const length = [...code].length;
  return length > keywordLimit ? `${subject}有 ${length} 个字符，比读口收的检索词（最多 ${keywordLimit} 个字符）还长，没法去登记册里找它` : null;
}

export function createStagingVerifier(read: CatalogueReader, maxPages: number = MAX_STAGING_PAGES): StagingVerifier {
  return {
    async verify(warehouse, code) {
      const long = overLimit('这个码', code);
      if (long !== null) return unverified(long);

      const here = await seek<CatalogueListBody, LocationRecord>({
        read: (after) => read({ selectors: { family: 'location' }, filters: { warehouseCode: [warehouse] }, q: code }, after),
        rowsOf: (body) => body.locations,
        isIt: (row) => row.warehouseCode === warehouse && row.locationCode === code,
        words: registerWords('库位'),
        maxPages,
      });
      if (here.kind === 'unread') return unverified(here.reason);
      if (here.kind === 'absent') {
        // 这个仓里没有：要知道是登记在别的仓，还是哪里都没有，才有话可说
        const elsewhere = await seek<CatalogueListBody, LocationRecord>({
          read: (after) => read({ selectors: { family: 'location' }, q: code }, after),
          rowsOf: (body) => body.locations,
          isIt: (row) => row.locationCode === code && row.warehouseCode !== warehouse,
          words: registerWords('库位'),
          maxPages,
        });
        if (elsewhere.kind === 'unread') return unverified(elsewhere.reason);
        return rejected(
          elsewhere.kind === 'found'
            ? `库位 ${code} 属于仓 ${elsewhere.row.warehouseCode}，这台设备绑的是仓 ${warehouse}。`
            : `登记册里没有库位 ${code}。`,
        );
      }

      const place = here.row;
      if (place.deactivated) return rejected(`库位 ${code} 已停用，不能放货。`);
      const zoneCode = place.zoneCode;
      const zoneLong = overLimit('库位所在的库区码', zoneCode);
      if (zoneLong !== null) return unverified(zoneLong);

      const zone = await seek<CatalogueListBody, ZoneRecord>({
        read: (after) => read({ selectors: { family: 'zone' }, filters: { warehouseCode: [warehouse] }, q: zoneCode }, after),
        rowsOf: (body) => body.zones,
        isIt: (row) => row.warehouseCode === warehouse && row.zoneCode === zoneCode,
        words: registerWords('库区'),
        maxPages,
      });
      if (zone.kind === 'unread') return unverified(zone.reason);
      if (zone.kind === 'absent') return rejected(`找不到库位 ${code} 所在的库区 ${zoneCode}。`);
      if (zone.row.deactivated) return rejected(`库位 ${code} 所在的库区 ${zoneCode} 已停用。`);
      if (zone.row.usage !== 'OUTBOUND_STAGING') {
        return rejected(
          `库位 ${code} 所在的库区 ${zoneCode} 用途是「${zoneUsageLabel(zone.row.usage)}」，不是待出库库区，拣出来的货不能放那里。`,
        );
      }
      return { kind: 'ok' };
    },
  };
}

/** 接真读口的那一份。 */
export function liveStagingVerifier(): StagingVerifier {
  return createStagingVerifier(listWarehouseCatalogue);
}
