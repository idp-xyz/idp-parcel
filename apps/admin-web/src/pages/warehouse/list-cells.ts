// 仓配列表的排版规矩里不带界面的那一半（纯 TypeScript，node:test 直接测）：列按内容定宽、时刻的短写法、编码换名称。
// 单元格长什么样在 wms-ui 的 WmsTwoLine。

import { formatInstant } from '../moment';
import type { ListColumn } from '../../templates/ListPageTemplate';

/** 让一列缩到内容那么宽：表是撑满的，不写宽度的列会平分多出来的空白，几个短值之间就隔出一大片。 */
export const fitContentClass = 'w-px whitespace-nowrap';

/** 末尾那一列空列的标识。它吃掉右边多出来的宽度，各数据列才能挨在一起从左往右读。 */
export const fillerColumnId = 'wms-filler';

/**
 * 各列按内容定宽、靠左排紧，末尾补一列空列占剩下的宽度。屏幕越宽，一行里各个值之间的距离不变，
 * 眼睛不用横扫整屏去找「下一步」。
 */
export function packColumns<Row>(columns: ReadonlyArray<ListColumn<Row>>): ListColumn<Row>[] {
  const packed = columns.map((column) => ({
    ...column,
    className: column.className ? `${fitContentClass} ${column.className}` : fitContentClass,
  }));
  return [...packed, { id: fillerColumnId, header: '', className: 'w-full', render: () => null }];
}

/**
 * 列表里时刻的短写法：`MM-DD HH:mm`，不是今年的带上年份。到秒与偏移是悬停与单据页里的事——列表上要的是一眼
 * 看出哪一天、几点。认不出的串原样交回。
 */
export function shortInstant(value: string | undefined, now: Date): string {
  if (!value) return '';
  const full = formatInstant(value, { precision: 'minute' });
  if (full === value) return value;
  const thisYear = formatInstant(now.toISOString(), { precision: 'day' }).slice(0, 4);
  return full.startsWith(`${thisYear}-`) ? full.slice(5) : full;
}

/**
 * 编码对应的名称。名册里没有这个编码、或登记时没留名称，答空串：单元格这时只摆编码，不编一个名字。
 */
export function nameOf(names: ReadonlyMap<string, string>, code: string): string {
  return names.get(code)?.trim() ?? '';
}
