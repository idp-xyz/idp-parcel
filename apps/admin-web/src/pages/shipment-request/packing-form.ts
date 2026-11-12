import type { PackingDraft } from './api';
import { emptyGoodsLine, nonBlankGoodsLineIndexes, type GoodsLineForm } from './goods-form';
import { decimalShape } from './form-shapes';

// 下单页的装箱矩阵(ADR-0373 决定三,票 declared-goods-lines/05):行 × 箱的数量,`matrix[箱下标][行下标]`,下标都是表单里的
// 下标(含空行),交给服务端时再换成去空行之后的行号(nonBlankGoodsLineIndexes)。小包下单页的多件委托与平台货件页共用。
//
// 形状规则跟服务端走——要么不报、报就报全:每箱至少装一行、每行各箱之和等于这一行的数量、报了分配的行要有数量。页面先按同一条
// 核一遍,免得一来一回才学到同一句话;服务端照样核,这里不是第二个权威。
//
// 格里填 0 当没装:服务端没有数量为 0 的分配,矩阵里却常有人顺手填 0,拒掉它只是多一道手续。

export type PackingMatrix = string[][];

export function packingCell(matrix: PackingMatrix, box: number, line: number): string {
  return matrix[box]?.[line] ?? '';
}

function grown(matrix: PackingMatrix, box: number, line: number): PackingMatrix {
  const next = matrix.map((row) => [...row]);
  while (next.length <= box) next.push([]);
  for (let at = 0; at <= box; at += 1) while (next[at].length <= line) next[at].push('');
  return next;
}

export function withPackingCell(matrix: PackingMatrix, box: number, line: number, value: string): PackingMatrix {
  const next = grown(matrix, box, line);
  next[box][line] = value;
  return next;
}

/** 去掉一箱:后面的箱上移,与箱子表同步。 */
export function withoutPackingBox(matrix: PackingMatrix, box: number): PackingMatrix {
  return matrix.filter((_, at) => at !== box);
}

/** 去掉一行货:各箱里这一列一起去掉,与货物行表同步。 */
export function withoutPackingLine(matrix: PackingMatrix, line: number): PackingMatrix {
  return matrix.map((row) => row.filter((_, at) => at !== line));
}

/**
 * 提交前收掉空行,矩阵同步去掉那几列:服务端的行号只数非空行,屏上留着空行,服务端说「第 2 行」时人对着的是第 3 张卡。
 * 一行都没填时留一张空卡,表单不至于空掉。
 */
export function withoutBlankGoodsLines(lines: GoodsLineForm[], matrix: PackingMatrix): { lines: GoodsLineForm[]; matrix: PackingMatrix } {
  const kept = nonBlankGoodsLineIndexes(lines);
  if (kept.length === lines.length) return { lines, matrix };
  return {
    lines: kept.length > 0 ? kept.map((index) => lines[index]) : [{ ...emptyGoodsLine }],
    matrix: matrix.map((row) => kept.map((index) => row[index] ?? '')),
  };
}

/**
 * 「同款每箱 N 件 × M 箱」:把一行货在第 fromBox 到第 toBox 箱(含两端,下标从 0)各填 perBox。只改这一列,别的行不动;
 * 原来有数的格照样覆盖——人按这个按钮就是要整列重填。
 */
export function fillSameEachBox(matrix: PackingMatrix, line: number, perBox: string, fromBox: number, toBox: number): PackingMatrix {
  if (toBox < fromBox) return matrix;
  const next = grown(matrix, toBox, line);
  for (let box = fromBox; box <= toBox; box += 1) next[box][line] = perBox;
  return next;
}

function cellQuantity(value: string): string | undefined {
  const text = value.trim();
  if (text === '' || (decimalShape(text) && !/[1-9]/.test(text))) return undefined;
  return text;
}

/** 矩阵里有没有填了的格;一格都没有就是整票清单,不报分配。 */
export function packingFilled(lines: GoodsLineForm[], matrix: PackingMatrix, boxCount: number): boolean {
  const used = nonBlankGoodsLineIndexes(lines);
  for (let box = 0; box < boxCount; box += 1) {
    if (used.some((line) => cellQuantity(packingCell(matrix, box, line)) !== undefined)) return true;
  }
  return false;
}

// 十进制串精确相加:装箱的数量可以是 0.5 千克这样的数,浮点加出 0.30000000000000004 就对不上了。
interface Scaled {
  digits: bigint;
  scale: number;
}

function scaledOf(value: string): Scaled | undefined {
  if (!decimalShape(value)) return undefined;
  const [whole, fraction = ''] = value.split('.');
  return { digits: BigInt(whole + fraction), scale: fraction.length };
}

function rescale(value: Scaled, scale: number): bigint {
  return value.digits * BigInt(10) ** BigInt(scale - value.scale);
}

function formatScaled(value: Scaled): string {
  if (value.scale === 0) return value.digits.toString();
  const text = value.digits.toString().padStart(value.scale + 1, '0');
  const whole = text.slice(0, text.length - value.scale);
  const fraction = text.slice(text.length - value.scale).replace(/0+$/, '');
  return fraction ? `${whole}.${fraction}` : whole;
}

function sumOf(values: Scaled[]): Scaled {
  const scale = Math.max(0, ...values.map((value) => value.scale));
  return { digits: values.reduce((total, value) => total + rescale(value, scale), BigInt(0)), scale };
}

function sameAmount(left: Scaled, right: Scaled): boolean {
  const scale = Math.max(left.scale, right.scale);
  return rescale(left, scale) === rescale(right, scale);
}

/** 一行货的装箱核对:declared 是这一行报的数量(没报为空串),packed 是各箱之和。 */
export interface PackingLineCheck {
  line: number;
  declared: string;
  packed: string;
  balanced: boolean;
}

export interface PackingCheck {
  /** 每个非空货物行一条,按表单次序。 */
  lines: PackingLineCheck[];
  /** 一行都没装的箱,下标从 0。 */
  emptyBoxes: number[];
}

/** 核装箱矩阵:逐行和对不对得上,哪几箱什么都没装。数不成形的格不计(另报)。 */
export function packingCheckOf(lines: GoodsLineForm[], matrix: PackingMatrix, boxCount: number): PackingCheck {
  const used = nonBlankGoodsLineIndexes(lines);
  const checks = used.map((line) => {
    const cells: Scaled[] = [];
    for (let box = 0; box < boxCount; box += 1) {
      const quantity = cellQuantity(packingCell(matrix, box, line));
      const scaled = quantity === undefined ? undefined : scaledOf(quantity);
      if (scaled) cells.push(scaled);
    }
    const packed = sumOf(cells);
    const declared = lines[line].quantity.trim();
    const declaredScaled = scaledOf(declared);
    return {
      line,
      declared,
      packed: formatScaled(packed),
      balanced: declaredScaled !== undefined && sameAmount(declaredScaled, packed),
    };
  });
  const emptyBoxes: number[] = [];
  for (let box = 0; box < boxCount; box += 1) {
    if (!used.some((line) => cellQuantity(packingCell(matrix, box, line)) !== undefined)) emptyBoxes.push(box);
  }
  return { lines: checks, emptyBoxes };
}

/** 填的不是十进制数的格,`[箱下标, 行下标]`,按箱的次序。 */
function malformedCellsOf(lines: GoodsLineForm[], matrix: PackingMatrix, boxCount: number): [number, number][] {
  const used = nonBlankGoodsLineIndexes(lines);
  const cells: [number, number][] = [];
  for (let box = 0; box < boxCount; box += 1) {
    for (const line of used) {
      const value = packingCell(matrix, box, line).trim();
      if (value && !decimalShape(value)) cells.push([box, line]);
    }
  }
  return cells;
}

function countedList(items: string[], limit: number, unit: string): string {
  return items.length > limit ? `${items.slice(0, limit).join('、')} 等 ${items.length} ${unit}` : items.join('、');
}

/**
 * 报了分配时,矩阵里还差什么——一句一事;报全了或整票清单(一格都没填)为空表。箱与行都按屏上的序号说(第几箱、第几行),即
 * 表单里的第几张,与货物行表、箱子表上的编号一致。check 可由调用方传入,免得同一份矩阵核两遍。
 */
export function packingProblemsOf(
  lines: GoodsLineForm[],
  matrix: PackingMatrix,
  boxCount: number,
  check: PackingCheck = packingCheckOf(lines, matrix, boxCount),
): string[] {
  if (!packingFilled(lines, matrix, boxCount)) return [];
  const problems: string[] = [];
  const malformed = malformedCellsOf(lines, matrix, boxCount).map(([box, line]) => `第 ${box + 1} 箱第 ${line + 1} 行`);
  if (malformed.length > 0) problems.push(`${countedList(malformed, 5, '格')}填的不是数字`);
  for (const line of check.lines) {
    if (line.balanced) continue;
    problems.push(
      line.declared
        ? `第 ${line.line + 1} 行报了 ${line.declared},各箱加起来是 ${line.packed}`
        : `第 ${line.line + 1} 行没填数量,报了装箱就要填`,
    );
  }
  if (check.emptyBoxes.length > 0) {
    const boxes = check.emptyBoxes.map((box) => `第 ${box + 1} 箱`);
    problems.push(`${countedList(boxes, 5, '箱')}什么都没装;报了装箱,每箱至少装一行`);
  }
  return problems;
}

/**
 * 两个下单页提交前对装箱矩阵的核对:数不成形的格各标一个键 `k{箱下标}.{行下标}`(格上标框),整体问题并成 `packing` 一句。
 */
export function packingErrorsOf(lines: GoodsLineForm[], matrix: PackingMatrix, boxCount: number): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const [box, line] of malformedCellsOf(lines, matrix, boxCount)) errors[`k${box}.${line}`] = '填数字';
  const problems = packingProblemsOf(lines, matrix, boxCount);
  if (problems.length > 0) errors.packing = problems.join(';');
  return errors;
}

/**
 * 各箱的装箱分配草案,按箱的次序;一格都没填时缺席(整票清单)。行号是去空行之后的第几行(从 1 起),与 goodsDraftOfLines
 * 交出的行次序一致。
 */
export function packingDraftsOf(lines: GoodsLineForm[], matrix: PackingMatrix, boxCount: number): PackingDraft[][] | undefined {
  if (!packingFilled(lines, matrix, boxCount)) return undefined;
  const used = nonBlankGoodsLineIndexes(lines);
  const drafts: PackingDraft[][] = [];
  for (let box = 0; box < boxCount; box += 1) {
    const allocations: PackingDraft[] = [];
    used.forEach((line, order) => {
      const quantity = cellQuantity(packingCell(matrix, box, line));
      if (quantity !== undefined) allocations.push({ line: order + 1, quantity });
    });
    drafts.push(allocations);
  }
  return drafts;
}

