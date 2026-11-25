// 一张出库单此刻该做的那一步（纯 TypeScript，node:test 直接测）。列表每行最右与单据页头区的主动作读同一份，
// 两处不各写一套判断。

import { dispatchStationHash, packStationHash, pickStationHash } from './station/stations';

export type OutboundNextStep =
  /** 在出库单上就地做：放波次。 */
  | { kind: 'wave'; label: string }
  /** 要去另一页做：现场还没搬进全屏的几步在旧的仓内作业台，代理仓的在代理仓页。 */
  | { kind: 'go'; label: string; moduleId: string }
  /** 要去全屏的工位做（不带外壳，进去前要先问有没有没提交的输入）：地址已经写好，拣货带着这张单所在的波次，复核打包带着它的出库请求号。 */
  | { kind: 'station'; label: string; hash: string }
  /** 等人拿主意，页面上没有一个按钮能替他定：说清在等什么。 */
  | { kind: 'wait'; label: string };

const workstation = 'warehouse-workstation';

/**
 * 没有下一步的状态（已出库、已取消、已拒单、已拆单）答 null。代理仓的单打包后不经自营仓的出库确认，
 * 由代理仓回报交接，所以它的已打包也指到代理仓页。
 *
 * 已放波次的去拣货工位：知道这张单的波次（单据页读得到）就带上，只做那个波次；不知道（列表行没有这一格）就是本仓全部波次。
 * 已拣货与已复核的去复核打包工位：带着这张单的出库请求号，进去就直接做这一张；没给请求号就进工位等人扫单号。
 */
export function outboundNextStep(status: string, agentWarehouse: boolean, wave?: string, requestId?: string): OutboundNextStep | null {
  switch (status) {
    case 'RESERVED':
      return { kind: 'wave', label: '放波次' };
    case 'PARTIALLY_RESERVED':
      return { kind: 'wait', label: '库存不够，等补货或拆单' };
    case 'PENDING_POLICY':
      return { kind: 'wait', label: '等登记缺货处置' };
    case 'ALLOCATED':
      return { kind: 'station', label: '去拣货', hash: pickStationHash(wave) };
    case 'PICKED':
      return { kind: 'station', label: '去复核', hash: packStationHash(requestId) };
    case 'CHECKED':
      return { kind: 'station', label: '去打包', hash: packStationHash(requestId) };
    case 'PACKED':
      return agentWarehouse
        ? { kind: 'go', label: '回录代理仓交接', moduleId: 'warehouse-agents' }
        : { kind: 'station', label: '去发货', hash: dispatchStationHash(requestId) };
    case 'AWAITING_RETURN':
      return { kind: 'go', label: '去回库', moduleId: workstation };
    case 'HANDED_OFF':
      return { kind: 'go', label: '转出代理仓', moduleId: 'warehouse-agents' };
    case 'FORWARDED':
      return { kind: 'go', label: '回录代理仓结果', moduleId: 'warehouse-agents' };
    default:
      return null;
  }
}

/**
 * 还说得上「占没占够」的状态：拣完之前。拣出去的那部分预占随拣货放开，拣完之后预占是 0，那不是缺货；
 * 取消、拒单的单预占也是 0，同样不是缺货。
 */
export function outboundReservationOpen(status: string): boolean {
  return status === 'RESERVED' || status === 'PARTIALLY_RESERVED' || status === 'PENDING_POLICY' || status === 'ALLOCATED';
}

/** 不会再往下走的状态：队列上不算在等。 */
export function outboundSettled(status: string): boolean {
  return status === 'SHIPPED' || status === 'CANCELLED' || status === 'REJECTED' || status === 'SPLIT';
}
