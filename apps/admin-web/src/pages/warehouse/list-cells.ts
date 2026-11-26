// 仓配列表的排版规矩里不带界面的那一半（纯 TypeScript，node:test 直接测）：列按内容定宽、时刻的短写法、编码换名称。
// 单元格长什么样在 wms-ui 的 WmsTwoLine。

import { formatInstant } from '../moment';
import type { ListColumn } from '../../templates/ListPageTemplate';

/** 让一列缩到内容那么宽：表是撑满的，不写宽度的列会平分多出来的空白，几个短值之间就隔出一大片。 */
export const fitContentClass = 'w-px whitespace-nowrap';

const flexShares = ['w-full', 'w-1/2', 'w-1/3'] as const;

/**
 * 吃宽度的列：多出来的宽度给它，`count` 列都是这一种就平分。各列的百分比加起来正好一整份才分得匀——都写成一整份的话，
 * 浏览器把宽度全给排在前面的那一列。`max-w-0` 让格里的长文字跟着列宽截断，而不是把列撑开；`min-w` 是窗口窄时它
 * 最少留多宽，再窄就由表自己横向滚。只备了三档：点名超过三列时前三列把宽度分完，其余的停在最小宽度——真有四列
 * 自由文字的表，先想想是不是该收一列进单据页。
 */
export function flexColumnClass(count: number): string {
  const share = flexShares[Math.min(Math.max(count, 1), flexShares.length) - 1];
  return `${share} max-w-0 min-w-[11rem]`;
}

/** 末尾那一列空列的标识。没有哪一列吃宽度时，由它占掉右边多出来的宽度。 */
export const fillerColumnId = 'wms-filler';

/**
 * 各列按内容定宽、挨着排。`flex` 点名的列是放自由文字的（地址、明细），多出来的宽度给它们：屏幕宽了是这些字
 * 露出来得更全，不是各列之间的空白变大，排在它们后面的数字与动作贴在右边、每一行都在同一处。
 * 没有点名时末尾补一列空列占剩下的宽度。
 */
export function packColumns<Row>(columns: ReadonlyArray<ListColumn<Row>>, flex: readonly string[] = []): ListColumn<Row>[] {
  const flexing = columns.filter((column) => flex.includes(column.id)).length;
  const packed = columns.map((column) => {
    const sizing = flex.includes(column.id) ? flexColumnClass(flexing) : fitContentClass;
    return { ...column, className: column.className ? `${sizing} ${column.className}` : sizing };
  });
  if (flexing > 0) return packed;
  return [...packed, { id: fillerColumnId, header: '', className: 'w-full', render: () => null }];
}

/**
 * 仓配列表那块表面的样子，交给列表模板的 surfaceClassName。三件事：
 * - 表面最宽到 120rem，超宽屏上多出来的留在表外，不把一行拉到两千多像素宽；
 * - 行与行之间一道比表框淡的细线：行宽了没有线眼睛会串行，线和表框一样重又显得满屏是线。颜色取正文色的一成不到，
 *   三套主题下都比各自的边框色淡；主题色是裸 CSS 变量，淡一档只能用 color-mix 写，`/NN` 生不出样式；
 * - 表头一条与表面分得开的底、字用正文色，和表体分开。底色是往表面色里掺一点正文色：现成的悬停色在深色主题下
 *   和表面只差几级灰，看不出是一条带；掺出来的在三套主题下都差出同样的一档。它是实色，吸顶时盖得住滚过去的行。
 *
 * - 一行里各格的第一行字落在同一条基线上。有的格是两行（名称加编码），有的只有一行（发运、件数）或是一枚徽章、
 *   一个钮：都按居中摆的话，单行的值会悬在邻格两行字的中间，横着读一行时视线要上下跳。
 *
 * 这些类是从表面往里指的，格里要是再套一张表也会被指到；往别的页上用之前先看那一页的格里有没有。
 */
export const wmsSurfaceClass = [
  'max-w-[120rem]',
  '[&_tbody_td]:align-baseline',
  '[&_tbody_td]:border-b',
  '[&_tbody_td]:border-[color:color-mix(in_srgb,var(--idpxyz-text)_8%,transparent)]',
  '[&_tbody_tr:last-child_td]:border-b-0',
  '[&_thead_th]:bg-[color-mix(in_srgb,var(--idpxyz-text)_7%,var(--idpxyz-sidebar))]',
  '[&_thead_th]:text-idpxyz-text',
].join(' ');

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
