// 送站预报区与送站交接单区共用的筛选写法、送站预报的行转写与两本册读口的问题码（票 ps-external-mark-relations/13「做什么」的「查阅读口」）。
//
// 单独成模块而不写在页面里：最容易错的是「日」——读口只收瞬间，一日从哪一刻起算随看的人所在的时区，页面在这里把日子换成本地零点到次日
// 本地零点的左闭右开区间；换错一格，零点那一份就落进前一日。页面组件要 React 与 DOM，这几格是纯函数就能拿 node:test 钉住。

import { formatInstant } from '../catalogue-view';
import type { DeliveryPreAdviceRecord } from './delivery-scope-api';

/** 一本册列表认的那一对业务时刻键：预报是预计送达，交接单是开立。键名与 query_delivery_scope_records.go 同字。 */
export interface DeliveryScopeFilterKeys {
  from: string;
  until: string;
}

export const preAdviceFilterKeys: DeliveryScopeFilterKeys = { from: 'expectedArrivalFrom', until: 'expectedArrivalUntil' };
export const handoverNoteFilterKeys: DeliveryScopeFilterKeys = { from: 'openedFrom', until: 'openedUntil' };

/** 页面上人填的两格：节点与日子（`<input type="date">` 的 YYYY-MM-DD，空即不按日筛）。 */
export interface DeliveryScopeListFilter {
  node: string;
  day: string;
}

/**
 * 把 YYYY-MM-DD 换成那一日本地零点到次日本地零点（左闭右开）的两个瞬间。认不出的日子（含 2 月 30 日这种 Date 会悄悄进位的）答 null。
 */
export function localDayBoundsOf(day: string): { from: string; until: string } | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day);
  if (!match) return null;
  const [year, month, date] = [Number(match[1]), Number(match[2]) - 1, Number(match[3])];
  const start = new Date(year, month, date);
  if (start.getFullYear() !== year || start.getMonth() !== month || start.getDate() !== date) return null;
  return { from: start.toISOString(), until: new Date(year, month, date + 1).toISOString() };
}

/**
 * 拼列表路径：只带填了的格，节点去两端空白。日子认不出时答 null，页面不发请求——发半截条件，人会把「没按日筛的列表」当成「这一日的」。
 */
export function deliveryScopeListPathOf(
  path: string,
  keys: DeliveryScopeFilterKeys,
  filter: DeliveryScopeListFilter,
): string | null {
  const query = new URLSearchParams();
  const node = filter.node.trim();
  if (node !== '') query.set('node', node);
  if (filter.day !== '') {
    const bounds = localDayBoundsOf(filter.day);
    if (!bounds) return null;
    query.set(keys.from, bounds.from);
    query.set(keys.until, bounds.until);
  }
  const encoded = query.toString();
  return encoded === '' ? path : `${path}?${encoded}`;
}

export interface PreAdviceReviewRow {
  key: string;
  preAdviceId: string;
  values: Readonly<Record<string, string>>;
}

/**
 * 一份预报转写成预报区的一行。「未撤回」不写成「有效」：预报对一次送站算不算数，节点、时窗与号码要在收寄时逐条判（ADR-0212 决定二），
 * 列表上说不出。
 */
export function preAdviceRowOf(record: DeliveryPreAdviceRecord): PreAdviceReviewRow {
  return {
    key: `pre-advice:${record.preAdviceId}`,
    preAdviceId: record.preAdviceId,
    values: {
      preAdviceId: record.preAdviceId,
      customerAccount: record.customerAccount,
      node: record.node,
      expectedArrivalAt: formatInstant(record.expectedArrivalAt),
      numbers: `${record.customerParcelNumbers.length} 个`,
      deliveringParty: record.deliveringParty ?? '（没给交付方线索）',
      withdrawal: record.withdrawn
        ? `已撤回${record.withdrawnAt ? ` · ${formatInstant(record.withdrawnAt)}` : ''}`
        : '未撤回',
      formedAt: formatInstant(record.formedAt),
    },
  };
}

/** 两本册读口的 problem+json 码说明。 */
export function deliveryScopeReadProblemNote(code: string): string {
  switch (code) {
    case 'DELIVERY_PRE_ADVICE_NOT_VISIBLE':
      return '本租户查无这份预报（别的租户的同名预报也是这一句）；刷新列表再选。';
    case 'DELIVERY_HANDOVER_NOTE_NOT_VISIBLE':
      return '本租户查无这张交接单（别的租户的同名交接单也是这一句）；刷新列表再选。';
    case 'MALFORMED_REQUEST':
      return '筛选条件不是服务端接受的形状（节点不得空白，日子换成的两端须前早后晚）；请检查前端与服务端版本是否一致。';
    case 'METHOD_NOT_ALLOWED':
      return '端点不接受当前 HTTP 方法；请检查前端与服务端版本是否一致。';
    case 'INTAKE_FAILED':
      return '接入面未能形成操作者身份；服务端未读册。';
    case 'IDENTITY_DEPENDENCY_UNAVAILABLE':
      return '接入身份依赖读不动，可稍后重试。';
    case 'NO_ANSWER_FORMED':
      return '读册未形成答案，可稍后重试；这不是「册为空」。';
    default:
      return `服务端问题码：${code}`;
  }
}
