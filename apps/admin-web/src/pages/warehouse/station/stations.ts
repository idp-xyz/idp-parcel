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
  { id: 'pack', name: '复核打包', summary: '扫出库单号，逐件复核，再选包材称重打包。', ready: true },
  { id: 'receive', name: '收货', summary: '扫入库预报，逐件扫货，一次提交一批收货。', ready: true },
  { id: 'dispatch', name: '发货交接', summary: '扫出库单号，逐个包裹确认交出。', ready: true },
  { id: 'putaway', name: '上架与移库', summary: '选货主，扫源库位、商品和目标库位。', ready: true },
  { id: 'count', name: '盘点清点', summary: '选一张盘点单，按库位逐件清点。', ready: false },
];

/** 旧的仓内作业台页。还没搬进全屏的工位仍在那里做；「退出作业台」也回到它。 */
export const legacyWorkstationHash = '#/warehouse-workstation';

export const stationHomeHash = stationHash(null);

/** 拣货工位的地址。给了波次号就只做那个波次，不给就是本仓全部波次。 */
export function pickStationHash(wave?: string): string {
  return stationHash('pick', { wave });
}

/** 复核打包工位的地址。给了出库请求号就直接去做那一张单，不给就等人扫出库单号。 */
export function packStationHash(request?: string): string {
  return stationHash('pack', { request });
}

/** 收货工位的地址。给了预报标识就直接去做那一张，不给就等人扫入库单号。 */
export function receiveStationHash(advice?: string): string {
  return stationHash('receive', { advice });
}

/** 发货交接工位的地址。给了出库请求号就直接去做那一张。 */
export function dispatchStationHash(request?: string): string {
  return stationHash('dispatch', { request });
}

/** 上架与移库工位的地址。货主不写进地址，进去自己选。 */
export function putawayStationHash(): string {
  return stationHash('putaway');
}

export function stationEntryOf(station: string | null): StationEntry | undefined {
  return stationEntries.find((entry) => entry.id === station);
}

export type StationRoute =
  /** 选工位。 */
  | { kind: 'home' }
  | { kind: 'pick'; wave: string | null }
  | { kind: 'pack'; request: string | null }
  | { kind: 'receive'; advice: string | null }
  | { kind: 'dispatch'; request: string | null }
  | { kind: 'putaway' }
  /** 词是六个工位之一，但全屏的还没做好：说明它还在仓内作业台里。 */
  | { kind: 'not-ready'; entry: StationEntry }
  /** 词不是任何一个工位。 */
  | { kind: 'unknown'; station: string };

const textOf = (address: StationAddress, name: string): string | null => {
  const value = address.query.get(name)?.trim() ?? '';
  return value === '' ? null : value;
};

/** 地址落到哪个工位。 */
export function resolveStationRoute(address: StationAddress): StationRoute {
  if (address.station === null) return { kind: 'home' };
  const entry = stationEntryOf(address.station);
  if (entry === undefined) return { kind: 'unknown', station: address.station };
  if (!entry.ready) return { kind: 'not-ready', entry };
  // 做好一个工位要在这里给它一支；没给的哪怕 ready 了也落到说明页，不错落到别的工位上
  switch (entry.id) {
    case 'pick':
      return { kind: 'pick', wave: textOf(address, 'wave') };
    case 'pack':
      return { kind: 'pack', request: textOf(address, 'request') };
    case 'receive':
      return { kind: 'receive', advice: textOf(address, 'advice') };
    case 'dispatch':
      return { kind: 'dispatch', request: textOf(address, 'request') };
    case 'putaway':
      return { kind: 'putaway' };
    default:
      return { kind: 'not-ready', entry };
  }
}
