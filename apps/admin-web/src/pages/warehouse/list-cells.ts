// 仓配列表的排版规矩里不带界面的那一半（纯 TypeScript，node:test 直接测）：列宽、时刻的短写法、编码换名称、商品行的摘要。
// 单元格长什么样在 wms-ui 的 WmsTwoLine。
//
// 列表摆在工作台模板上（templates/WorkbenchPageTemplate）。列宽的规矩：值短的列（单号、状态、数字、动作）缩到内容那么宽，
// 放自由文字的列（地址、商品）不写宽度、去吃多出来的宽度——屏幕宽了是这些字露出来得更全，不是各列之间的空白变大；
// 排在它们后面的数字与动作贴在右边，每一行都在同一处。

import { formatInstant } from '../moment';

/**
 * 交给工作台列的 `width`：让一列缩到内容那么宽。表是撑满的，不写宽度的列会分走多出来的空白；写成很小的百分比，
 * 浏览器就只给它内容要的宽度。格里的内容自己不能折行（见 wms-ui 的 WmsTwoLine 与各页的 `whitespace-nowrap`）。
 */
export const fitColumnWidth = '1%';

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

export interface LinesSummary {
  /** 第一种货与它的数量。没有商品行时是空串。 */
  first: string;
  /** 其余还有几种；只有一种时是空串。 */
  rest: string;
  /** 每一种货与数量，一行一种，给悬停用。 */
  all: string;
}

/** 一张单的商品行在列表一格里的写法：摆出第一种，其余的说还有几种，全部的在悬停里。 */
export function linesSummary(lines: ReadonlyArray<{ goodsCode: string; quantity: number }>): LinesSummary {
  const written = lines.map((line) => `${line.goodsCode} × ${line.quantity}`);
  return {
    first: written[0] ?? '',
    rest: written.length > 1 ? `另 ${written.length - 1} 种` : '',
    all: written.join('\n'),
  };
}

/**
 * 「已等」那一列的排序落到读口的受理时刻上。等得久的在前（降序）就是受理早的在前，方向是反的：表头上的箭头说的是
 * 「已等」这个数的大小，读口排的是时刻。
 */
export function waitedSortParam(dir: 1 | -1): string {
  return dir === -1 ? 'receivedAt' : '-receivedAt';
}
