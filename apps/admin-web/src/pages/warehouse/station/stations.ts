// 作业台的六个工位、它们的地址，以及地址落到哪个工位上（纯 TypeScript，node:test 直接测）。
//
// 六个工位分几笔做出来；做好一个，把它的 `ready` 改成真并在 resolveStationRoute 里给它一支。没做好的仍在旧的仓内作业台页里，
// 选工位页如实这样写、链过去，不摆点不动的假入口。

import { stationHash, type StationAddress } from '../../../shell/station-address';

export type StationId = 'pick' | 'pack' | 'receive' | 'dispatch' | 'putaway' | 'count';

export interface StationEntry {
  id: StationId;
  name: string;
  /** 一句话：这个工位做什么。 */
  summary: string;
  /** 全屏工位做好了没有；没做好的还在仓内作业台里做。 */
  ready: boolean;
}

export const stationEntries: readonly StationEntry[] = [
  { id: 'pick', name: '拣货', summary: '按波次里还没拣的格，逐格扫库位、扫商品，扫满自动提交。', ready: true },
  { id: 'pack', name: '复核打包', summary: '扫出库单号，逐件复核，再选包材称重打包。', ready: false },
  { id: 'receive', name: '收货', summary: '扫入库预报，逐件扫货，一次提交一批收货。', ready: false },
  { id: 'dispatch', name: '发货交接', summary: '扫出库单号，逐个包裹确认交出。', ready: false },
  { id: 'putaway', name: '上架与移库', summary: '选货主，扫源库位、商品和目标库位。', ready: false },
  { id: 'count', name: '盘点清点', summary: '选一张盘点单，按库位逐件清点。', ready: false },
];

/** 旧的仓内作业台页。还没搬进全屏的工位仍在那里做；「退出作业台」也回到它。 */
export const legacyWorkstationHash = '#/warehouse-workstation';

export const stationHomeHash = stationHash(null);

/** 拣货工位的地址。给了波次号就只做那个波次，不给就是本仓全部波次。 */
export function pickStationHash(wave?: string): string {
  return stationHash('pick', { wave });
}

export function stationEntryOf(station: string | null): StationEntry | undefined {
  return stationEntries.find((entry) => entry.id === station);
}

export type StationRoute =
  /** 选工位。 */
  | { kind: 'home' }
  | { kind: 'pick'; wave: string | null }
  /** 词是六个工位之一，但全屏的还没做好：说明它还在仓内作业台里。 */
  | { kind: 'not-ready'; entry: StationEntry }
  /** 词不是任何一个工位。 */
  | { kind: 'unknown'; station: string };

/** 地址落到哪个工位。 */
export function resolveStationRoute(address: StationAddress): StationRoute {
  if (address.station === null) return { kind: 'home' };
  const entry = stationEntryOf(address.station);
  if (entry === undefined) return { kind: 'unknown', station: address.station };
  if (!entry.ready) return { kind: 'not-ready', entry };
  const wave = address.query.get('wave')?.trim() ?? '';
  return { kind: 'pick', wave: wave === '' ? null : wave };
}
