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
}

export interface MappedProblems {
  located: LocatedProblem[];
  /** 映射不到控件的问题：整份层面的、表层面又没有默认落点的。 */
  unlocated: PriceCardProblem[];
}

function locate(problem: PriceCardProblem, target: Target, path: string): LocatedProblem {
  return {
    problem,
    step: stepOfField(target.field),
    field: target.field,
    where: (target.path ?? path) === '' ? target.label : `${target.path ?? path} · ${target.label}`,
  };
}

function cellOf(origin: Origin, column: string): Target {
  return origin.cells[column] ?? origin.fallback;
}

export function mapProblems(problems: readonly PriceCardProblem[], origins: OriginIndex): MappedProblems {
  const located: LocatedProblem[] = [];
  const unlocated: PriceCardProblem[] = [];
  for (const problem of problems) {
    if (problem.sheet === '') {
      unlocated.push(problem);
      continue;
    }
    if (problem.sheet === 'card') {
      const target = origins.cardFields[problem.column];
      if (target === undefined) unlocated.push(problem);
      else located.push(locate(problem, target, ''));
      continue;
    }
    const fallbackSheet = origins.sheets[problem.sheet];
    // 行号 = 数组下标 + 2；表层面的问题行号为 0。
    const origin = problem.row >= 2 ? origins.rows[problem.sheet]?.[problem.row - 2] : undefined;
    if (origin !== undefined) {
      located.push(locate(problem, cellOf(origin, problem.column), origin.path));
    } else if (fallbackSheet !== undefined) {
      located.push(locate(problem, fallbackSheet.target, fallbackSheet.path));
    } else {
      unlocated.push(problem);
    }
  }
  return { located, unlocated };
}

/** 控件 → 该控件上的问题话，给输入框显示用。 */
export function problemsByField(located: readonly LocatedProblem[]): Map<string, string[]> {
  const byField = new Map<string, string[]>();
  for (const entry of located) {
    byField.set(entry.field, [...(byField.get(entry.field) ?? []), entry.problem.message]);
  }
  return byField;
}

/** 各步骤上的问题条数，给步骤条画标记用。 */
export function problemCountsByStep(located: readonly LocatedProblem[]): Partial<Record<StepId, number>> {
  const counts: Partial<Record<StepId, number>> = {};
  for (const entry of located) counts[entry.step] = (counts[entry.step] ?? 0) + 1;
  return counts;
}
