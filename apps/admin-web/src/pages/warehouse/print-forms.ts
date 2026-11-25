// 四张能用现有读口打出来的单。面单不在这里：面单文件不在这些读口里。
//
// 拣货单按库位排，打的是波次上的任务（应拣、已拣）。装箱单与箱唛打的是已经打包的包裹。
// 入库单打预报与已收。盘点单打范围；已经录入的库位带上实点，没有录入的只留库位，不编账面数。

import type { PrintSheet } from './print-sheet';

const stockWords: Record<string, string> = { GOOD: '良品', PENDING_INSPECTION: '待检', DEFECTIVE: '次品' };

export interface PickTaskLine {
  locationCode: string;
  goodsCode: string;
  unit: string;
  quantity: number;
  picked: number;
  requestId: string;
}

export function pickListSheet(waveId: string, tasks: readonly PickTaskLine[]): PrintSheet {
  const ordered = [...tasks].sort((left, right) => left.locationCode.localeCompare(right.locationCode) || left.goodsCode.localeCompare(right.goodsCode));
  return {
    title: `拣货单 ${waveId}`,
    sections: [
      {
        heading: '按库位',
        lines: [{ label: '波次', value: waveId }, { label: '格数', value: String(tasks.length) }],
        headers: ['库位', '商品', '单位', '应拣', '已拣', '出库单'],
        rows: ordered.map((task) => [task.locationCode, task.goodsCode, task.unit, String(task.quantity), String(task.picked), task.requestId]),
      },
    ],
  };
}

export interface PackParcelLine {
  sequence: number;
  goods: ReadonlyArray<{ goodsCode: string; unit: string; quantity: number }>;
  weight?: string;
  weightUnit?: string;
  size?: string;
}

export function packListSheet(number: string, parcels: readonly PackParcelLine[]): PrintSheet {
  return {
    title: `装箱单 ${number}`,
    sections: parcels.map((parcel) => ({
      heading: `包裹 ${parcel.sequence}`,
      lines: [
        { label: '出库单', value: number },
        { label: '重量', value: parcel.weight ? `${parcel.weight} ${parcel.weightUnit ?? ''}` : '没记' },
        { label: '尺寸', value: parcel.size && parcel.size.trim() !== '' ? parcel.size : '没记' },
      ],
      headers: ['商品', '单位', '数量'],
      rows: parcel.goods.map((line) => [line.goodsCode, line.unit, String(line.quantity)]),
    })),
  };
}

/** 箱唛：一只包裹一页，大字是单号和包裹序号，下面是装了什么。不是承运面单。 */
export function cartonMarkSheet(number: string, parcels: readonly PackParcelLine[]): PrintSheet {
  return {
    title: `箱唛 ${number}`,
    sections: parcels.map((parcel) => ({
      heading: `${number} · 包裹 ${parcel.sequence}`,
      lines: [{ label: '出库单', value: number }, { label: '包裹', value: String(parcel.sequence) }],
      headers: ['商品', '数量'],
      rows: parcel.goods.map((line) => [line.goodsCode, String(line.quantity)]),
    })),
  };
}

export interface InboundLine {
  goodsCode: string;
  unit: string;
  advised: number;
  received: number;
}

export function inboundSheet(number: string, warehouse: string, lines: readonly InboundLine[]): PrintSheet {
  return {
    title: `入库单 ${number}`,
    sections: [
      {
        heading: '预报与已收',
        lines: [{ label: '入库预报', value: number }, { label: '仓', value: warehouse }],
        headers: ['商品', '单位', '预报', '已收'],
        rows: lines.map((line) => [line.goodsCode, line.unit, String(line.advised), String(line.received)]),
      },
    ],
  };
}

export interface CountPrintLine {
  locationCode: string;
  goodsCode: string;
  quantity: number;
  stockState?: string;
  observed?: string;
}

export function countSheet(countId: string, warehouse: string, locations: readonly string[], lines: readonly CountPrintLine[]): PrintSheet {
  const counted = new Set(lines.map((line) => line.locationCode));
  return {
    title: `盘点单 ${countId}`,
    sections: [
      {
        heading: '范围',
        lines: [{ label: '盘点单', value: countId }, { label: '仓', value: warehouse }],
        headers: ['库位', '清点'],
        rows: locations.map((location) => [location, counted.has(location) ? '已录入' : '还没录入']),
      },
      {
        heading: '已录入的实点',
        lines: [],
        headers: ['库位', '商品或观察', '状态', '实点'],
        rows: lines.map((line) => [line.locationCode, line.observed && line.observed !== '' ? line.observed : line.goodsCode, line.stockState ? (stockWords[line.stockState] ?? line.stockState) : '', String(line.quantity)]),
      },
    ],
  };
}
