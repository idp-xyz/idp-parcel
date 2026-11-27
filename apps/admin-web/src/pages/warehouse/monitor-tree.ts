// 仓库监控的对象树，以及一行怎么钻到下一层（纯 TypeScript，node:test 直接测）。
//
// 只列现有读口答得了的对象。没有列表读口的不进这棵树，去对应单据页看。
// 筛选只发该读口声明过的维：没声明的键整份请求会被退回。出库单点「拣货任务」要再读出库单详情，详情上有货主，才能再看这一格库存。
// 波次列表上已经带着任务，点「拣货任务」直接展开；任务上没有货主，这一层不再往下。

import { documentHash } from './document-address';

export interface MonitorNode {
  id: string;
  group: string;
  label: string;
  /** 这个对象没有列表读口，点开只说明。 */
  unavailable?: string;
  /** 读口按货主取，不填就不问，也不替租户挑一个货主。 */
  needsOwner?: boolean;
  /** 目录读口声明了仓。库存的仓是查询参数，页面另送，不走这里。 */
  warehouse?: boolean;
  /** 目录读口声明了货主。 */
  owner?: boolean;
  /** 目录读口声明的状态词表。不在词表里的不发。 */
  status?: readonly string[];
}

export const monitorNodes: readonly MonitorNode[] = [
  { id: 'pre-advice', group: '入库', label: '入库预报', needsOwner: true },
  { id: 'receipts', group: '入库', label: '收货记录', unavailable: '收货记录在入库预报那张单据上，没有单独的列表读口。' },
  { id: 'receipt-diff', group: '入库', label: '收货差异', unavailable: '收货差异记在收货那一笔上，没有单独的列表读口。' },
  { id: 'unclaimed', group: '入库', label: '待认领', warehouse: true, status: ['UNCLAIMED', 'CLAIMED', 'DISPOSED'] },
  {
    id: 'outbound',
    group: '出库',
    label: '出库单',
    warehouse: true,
    owner: true,
    status: ['RESERVED', 'PARTIALLY_RESERVED', 'REJECTED', 'PENDING_POLICY', 'HANDED_OFF', 'FORWARDED', 'SPLIT', 'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED', 'SHIPPED', 'CANCELLED', 'AWAITING_RETURN', 'PARTIALLY_CANCELLED'],
  },
  { id: 'waves', group: '出库', label: '波次', warehouse: true },
  { id: 'tasks', group: '出库', label: '拣货任务', unavailable: '拣货任务在波次上。在「波次」里双击一行，或在「出库单」里双击一张单。' },
  { id: 'parcels', group: '出库', label: '包裹', unavailable: '包裹在出库单那张单据上。从「出库单」打开那张单看。' },
  { id: 'cells', group: '库存', label: '库存格', needsOwner: true, warehouse: true },
  { id: 'cartons', group: '库存', label: '整箱', needsOwner: true, warehouse: true },
  { id: 'holds', group: '库存', label: '占用', unavailable: '占用在实时库存的占用视图里，监控这里不另问一遍。' },
  { id: 'movements', group: '库存', label: '流水', unavailable: '流水在实时库存的流水视图里，监控这里不另问一遍。' },
  { id: 'counts', group: '库内', label: '盘点', warehouse: true, owner: true, status: ['PENDING', 'COUNTING', 'COUNTED', 'VOID'] },
  { id: 'adjustments', group: '库内', label: '调整', warehouse: true, owner: true, status: ['PENDING', 'POSTED', 'REJECTED'] },
  { id: 'freezes', group: '库内', label: '冻结', warehouse: true, owner: true, status: ['FROZEN', 'UNFROZEN'] },
  { id: 'jobs', group: '库内', label: '移库与加工', unavailable: '移库、加工和换标在各自的单据页，没有一张合在一起的监控读口。' },
];

export interface MonitorLine {
  key: string;
  cells: string[];
  hash?: string;
  /** 往下看的按钮上的字。没有这一列就不画。 */
  drillLabel?: string;
  drillOutbound?: string;
  /** 点下去再按号读这一张波次的任务。列表不嵌每一条。 */
  drillWave?: string;
  drillStock?: { owner: string; warehouse: string; location: string; goods: string };
}

export interface MonitorOutbound {
  requestId: string;
  documentNumber?: string;
  ownerAccount: string;
  warehouseCode: string;
  status: string;
  dispatch: string;
}

export interface MonitorTask {
  taskId: string;
  locationCode: string;
  goodsCode: string;
  quantity: number;
  picked: number;
  status: string;
}

export const outboundHeaders = ['出库单', '货主', '仓', '状态', '发运'];
export const taskHeaders = ['任务', '库位', '商品', '应拣', '已拣', '状态'];
export const waveHeaders = ['波次', '仓', '出库单', '任务'];

/** 只留下这个对象声明过的筛选维。空着的维不发。 */
export function monitorFilters(node: MonitorNode, warehouse: string, owner: string, status: string): Record<string, readonly string[]> {
  const filters: Record<string, readonly string[]> = {};
  const warehouseCode = warehouse.trim();
  if (node.warehouse && warehouseCode !== '') filters.warehouseCode = [warehouseCode];
  const ownerAccount = owner.trim();
  if (node.owner && ownerAccount !== '') filters.ownerAccount = [ownerAccount];
  if (node.status?.includes(status)) filters.status = [status];
  return filters;
}

export function outboundLines(rows: readonly MonitorOutbound[]): MonitorLine[] {
  return rows.map((row) => ({
    key: row.requestId,
    cells: [row.documentNumber ?? row.requestId, row.ownerAccount, row.warehouseCode, row.status, row.dispatch],
    hash: documentHash('warehouse-outbound', [row.requestId]),
    drillOutbound: row.requestId,
    drillLabel: '拣货任务',
  }));
}

export function waveLines(
  rows: readonly {
    waveId: string;
    warehouseCode: string;
    requests: readonly string[];
    openTasks?: number;
    doneTasks?: number;
    shortTasks?: number;
    cancelledTasks?: number;
  }[],
): MonitorLine[] {
  return rows.map((row) => ({
    key: row.waveId,
    cells: [
      row.waveId,
      row.warehouseCode,
      String(row.requests.length),
      String((row.openTasks ?? 0) + (row.doneTasks ?? 0) + (row.shortTasks ?? 0) + (row.cancelledTasks ?? 0)),
    ],
    hash: documentHash('warehouse-waves', [row.waveId]),
    drillLabel: '拣货任务',
    drillWave: row.waveId,
  }));
}

/** 按号读到的波次任务。这一层没有货主，不再钻库存格。 */
export function waveTaskLines(tasks: readonly MonitorTask[]): MonitorLine[] {
  return tasks.map((task) => ({
    key: task.taskId,
    cells: [task.taskId, task.locationCode, task.goodsCode, String(task.quantity), String(task.picked), task.status],
  }));
}

export function taskLinesFromDetail(tasks: readonly MonitorTask[], owner: string, warehouse: string): MonitorLine[] {
  return tasks.map((task) => ({
    key: task.taskId,
    cells: [task.taskId, task.locationCode, task.goodsCode, String(task.quantity), String(task.picked), task.status],
    drillStock: owner.trim() === '' ? undefined : { owner: owner.trim(), warehouse, location: task.locationCode, goods: task.goodsCode },
    drillLabel: owner.trim() === '' ? undefined : '这一格',
  }));
}

export function monitorGroups(): Array<{ group: string; nodes: MonitorNode[] }> {
  const groups: Array<{ group: string; nodes: MonitorNode[] }> = [];
  for (const node of monitorNodes) {
    if (node.unavailable) continue;
    const found = groups.find((item) => item.group === node.group);
    if (found) found.nodes.push(node);
    else groups.push({ group: node.group, nodes: [node] });
  }
  return groups;
}
