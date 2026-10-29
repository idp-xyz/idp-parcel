// 预览答复里的问题 → 表单位置。
//
// 后端的问题坐标是（表、行、列）。表单生成工作簿时记下了每一行来自哪个控件（OriginIndex），这里查回去：能查到的标在控件上，
// 查不到的（整份层面的问题、模板版本、找不到行的）放总清单。位置的话用运营看得懂的路径（「费用项 2 · 条件 1 · 比较值」），
// 不出现表名和列键。

import type { PriceCardProblem } from '../api';
import { stepOfField } from './fields';
import type { Origin, OriginIndex, Target } from './workbook';
import type { StepId } from './model';

export interface LocatedProblem {
  problem: PriceCardProblem;
  step: StepId;
  /** 控件定位串（fields.ts）；只能定位到步骤时是 step.<步骤>。 */
  field: string;
  /** 位置的人话。 */
  where: string;
  /** 问题的话（中文部分）。 */
  text: string;
  /** 服务端话里夹带的英文原文（领域错误原文），单独收着，页面折叠显示。 */
  original?: string;
}

/**
 * 服务端的话多是中文白话；构造门拒收时会在中文后面附领域错误的英文原文（如「……立不住：parcel pricing: invalid rate entry」）。
 * 中文部分当话、英文部分收作原文，页面折叠显示：运营读中文，排查时原文还在。
 */
export function problemWords(message: string): { text: string; original?: string } {
  const cjk = /[\u3400-\u9fff]/;
  if (!cjk.test(message)) return { text: '没有通过检查', original: message };
  // 英文原文是一句带空格的纯 ASCII 话；「可取：G、KG」这类代码清单不算。
  const match = /^(.*?)[:：]\s*([A-Za-z][\x20-\x7e]*)$/s.exec(message);
  if (match !== null && match[1].trim() !== '' && /\s/.test(match[2].trim())) return { text: `${match[1].trim()}（原因见「原话」）`, original: match[2].trim() };
  return { text: message };
}

export interface MappedProblems {
  located: LocatedProblem[];
  /** 映射不到控件的问题：整份层面的、表层面又没有默认落点的。 */
  unlocated: PriceCardProblem[];
  /** 问题清单被截断的说明（`PROBLEMS_TRUNCATED`，问题多于 1000 条时才有）：不是一条要改的问题，放总清单顶部。 */
  truncated: string | null;
}

/** 问题清单被截断时服务端补在最前的问题码。 */
export const problemsTruncatedCode = 'PROBLEMS_TRUNCATED';
/** 整份内容超出技术上限（表数、行数、列数、总格数、请求体大小）时的问题码。 */
export const fileTooLargeCode = 'FILE_TOO_LARGE';

/**
 * 说不到格子的问题的话。超技术上限的话里点名的是工作簿的哪个上限，运营看不懂也没法照办，换成「太大，请拆分」，
 * 原话留作折叠的原文；其余照 problemWords。
 */
export function unlocatedWords(problem: PriceCardProblem): { text: string; original?: string } {
  if (problem.code === fileTooLargeCode) return { text: '这份价卡太大，请拆分成几张再提交。', original: problem.message };
  return problemWords(problem.message);
}

function locate(problem: PriceCardProblem, target: Target, path: string, named: boolean): LocatedProblem {
  const base = target.path ?? path;
  // 列对不上具体的格（行层面的拒收，列是各种标识列）时只说到行，焦点落在这一行的第一个控件。
  const where = !named || base === '' ? (named ? target.label : base || target.label) : `${base} · ${target.label}`;
  return { problem, step: stepOfField(target.field), field: target.field, where, ...problemWords(problem.message) };
}

/** 话里的生成标识换回它在表单里的名字，最长的先换，免得短标识是长标识的前缀时换坏。 */
function withNames(text: string, ids: Readonly<Record<string, string>>): string {
  return Object.keys(ids)
    .filter((id) => id !== '')
    .sort((left, right) => right.length - left.length)
    .reduce((current, id) => current.split(id).join(ids[id]), text);
}

export function mapProblems(problems: readonly PriceCardProblem[], origins: OriginIndex): MappedProblems {
  const located: LocatedProblem[] = [];
  const unlocated: PriceCardProblem[] = [];
  let truncated: string | null = null;
  const add = (entry: LocatedProblem) => {
    // 同一格被几行工作簿共用时（币种、重量单位、生效起点各出现在每张价表里）服务端会各报一遍，页面上只留一条。
    const text = withNames(entry.text, origins.ids);
    const duplicate = located.some((other) => other.field === entry.field && other.where === entry.where && other.text === text);
    if (!duplicate) located.push({ ...entry, text });
  };
  for (const problem of problems) {
    if (problem.code === problemsTruncatedCode) {
      truncated = problem.message;
      continue;
    }
    if (problem.sheet === '') {
      unlocated.push(problem);
      continue;
    }
    if (problem.sheet === 'card') {
      const target = origins.cardFields[problem.column];
      if (target === undefined) unlocated.push(problem);
      else add(locate(problem, target, '', true));
      continue;
    }
    const fallbackSheet = origins.sheets[problem.sheet];
    // 行号 = 数组下标 + 2；表层面的问题行号为 0。
    const origin = problem.row >= 2 ? origins.rows[problem.sheet]?.[problem.row - 2] : undefined;
    if (origin !== undefined) {
      const cell = origin.cells[problem.column];
      add(locate(problem, cell ?? origin.fallback, origin.path, cell !== undefined));
    } else if (fallbackSheet !== undefined) {
      add(locate(problem, fallbackSheet.target, fallbackSheet.path, problem.row === 0 && problem.column !== ''));
    } else {
      unlocated.push(problem);
    }
  }
  return { located, unlocated, truncated };
}

/** 各步骤上的问题条数，给步骤条画标记用。 */
export function problemCountsByStep(located: readonly LocatedProblem[]): Partial<Record<StepId, number>> {
  const counts: Partial<Record<StepId, number>> = {};
  for (const entry of located) counts[entry.step] = (counts[entry.step] ?? 0) + 1;
  return counts;
}
