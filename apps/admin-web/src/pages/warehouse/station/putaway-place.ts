// 上架与移库核一个库位能不能当源或目标。读法与拣货放货位相同：按仓加检索词定向读，认码完全相同的那一行，翻页有上限。
//
// 规矩取自 RegisterInternalJobHandler.screenLine 与 screenMove：库位要在这个仓、没停用，库区没停用。
// 待出库库区既不能当源也不能当目标。整箱只能移进收货暂存或存储库区。移进次品库区时，商品行的库存状态会被改成次品。

import { listWarehouseCatalogue, type CatalogueListBody, type LocationRecord, type ZoneRecord } from '../api';
import { zoneUsageLabel } from '../rows';
import { seek, type SeekWords } from './catalogue-seek';
import { MAX_STAGING_PAGES, type CatalogueReader } from './pick-staging-check';

export type PlaceRole = 'source' | 'target-goods' | 'target-carton';

export type PlaceVerdict =
  | { kind: 'ok'; usage: string }
  | { kind: 'rejected'; reason: string }
  | { kind: 'unverified'; reason: string };

const words = (what: string): SeekWords => ({
  failed: (cause) => `读${what}名册没成：${cause}`,
  shape: `读${what}名册得到的答复不是名册的样子`,
  tooMany: (maxPages) => `${what}名册里含这个码的行太多，翻了 ${maxPages} 页还没认出它`,
});

export function createPlaceVerifier(read: CatalogueReader, maxPages = MAX_STAGING_PAGES) {
  return {
    async verify(warehouse: string, code: string, role: PlaceRole): Promise<PlaceVerdict> {
      const here = await seek<CatalogueListBody, LocationRecord>({
        read: (after) => read({ selectors: { family: 'location' }, filters: { warehouseCode: [warehouse] }, q: code }, after),
        rowsOf: (body) => body.locations,
        isIt: (row) => row.warehouseCode === warehouse && row.locationCode === code,
        words: words('库位'),
        maxPages,
      });
      if (here.kind === 'unread') return { kind: 'unverified', reason: here.reason };
      if (here.kind === 'absent') return { kind: 'rejected', reason: `登记册里没有库位 ${code}，或它不在这个仓。` };
      if (here.row.deactivated) return { kind: 'rejected', reason: `库位 ${code} 已停用。` };
      const zoneCode = here.row.zoneCode;
      const zone = await seek<CatalogueListBody, ZoneRecord>({
        read: (after) => read({ selectors: { family: 'zone' }, filters: { warehouseCode: [warehouse] }, q: zoneCode }, after),
        rowsOf: (body) => body.zones,
        isIt: (row) => row.warehouseCode === warehouse && row.zoneCode === zoneCode,
        words: words('库区'),
        maxPages,
      });
      if (zone.kind === 'unread') return { kind: 'unverified', reason: zone.reason };
      if (zone.kind === 'absent') return { kind: 'rejected', reason: `找不到库位 ${code} 所在的库区 ${zoneCode}。` };
      if (zone.row.deactivated) return { kind: 'rejected', reason: `库位 ${code} 所在的库区 ${zoneCode} 已停用。` };
      const usage = zone.row.usage;
      if (usage === 'OUTBOUND_STAGING') return { kind: 'rejected', reason: `库位 ${code} 在待出库库区，不能在这里移入或移出。` };
      if (role === 'target-carton' && usage !== 'RECEIVING_STAGING' && usage !== 'STORAGE') {
        return { kind: 'rejected', reason: `整箱只能移进收货暂存或存储库区，库区 ${zoneCode} 的用途是「${zoneUsageLabel(usage)}」。` };
      }
      return { kind: 'ok', usage };
    },
  };
}

export function livePlaceVerifier() {
  return createPlaceVerifier(listWarehouseCatalogue);
}
