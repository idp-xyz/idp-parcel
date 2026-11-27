// 操作者的姓名怎么示、怎么报（纯 TypeScript，node:test 直接测）。
//
// 各册上记操作者的是「发行方#sub」这个串，人认不出是谁；登了姓名的示姓名，没登的示 sub 那一截。
// 姓名只是给人认的：判「是不是同一个人」「是不是我」永远用标识，不用姓名——两个人可以同名。

/** 标识「发行方#sub」到姓名。只有登了姓名的人在里面。 */
export type OperatorNames = ReadonlyMap<string, string>;

export const noOperatorNames: OperatorNames = new Map();

/** 与后端 accessidentity.MaxOperatorNameLength 同值；超了后端答坏报文，这里先拦住。 */
export const maxOperatorNameLength = 60;

export function operatorNamesOf(rows: ReadonlyArray<{ operator: string; name: string }>): OperatorNames {
  return new Map(rows.map((row) => [row.operator, row.name]));
}

/** 标识里 sub 那一截：发行方每个人都一样、又长，界面上不摆；没有井号的原样摆。 */
export function operatorSubjectOf(operator: string): string {
  const mark = operator.lastIndexOf('#');
  return mark < 0 || mark === operator.length - 1 ? operator : operator.slice(mark + 1);
}

/** 一位操作者示成什么：登了姓名示姓名，没登示 sub。没有人（空）答 null，由调用的地方说「还没人领」之类。 */
export function operatorLabel(operator: string | undefined, names: OperatorNames = noOperatorNames): string | null {
  if (!operator) return null;
  return names.get(operator) ?? operatorSubjectOf(operator);
}

/** 姓名这一格填得对不对：空是「不登/删掉」，算对；超长不对。答 null 即可以提交。 */
export function operatorNameProblem(draft: string): string | null {
  return [...draft.trim()].length > maxOperatorNameLength ? `姓名最多 ${maxOperatorNameLength} 个字。` : null;
}

/** 报文：去掉首尾空白；空了不带 name——不带才是删掉，空串后端不收。 */
export function operatorNamePayload(issuer: string, subject: string, draft: string): { issuer: string; subject: string; name?: string } {
  const name = draft.trim();
  return name === '' ? { issuer, subject } : { issuer, subject, name };
}

const outcomeNotes: Record<string, string> = {
  OPERATOR_NAME_RECORDED: '姓名已登上。',
  OPERATOR_NAME_CLEARED: '姓名已删掉，界面上重新示他的主体标识。',
  OPERATOR_NOT_FOUND: '本租户的操作者册上没有这个人，姓名没有登上。',
};

export function operatorNameOutcomeNote(outcome: string): { done: boolean; text: string } {
  const text = outcomeNotes[outcome];
  if (text === undefined) return { done: false, text: `后端答了一个这里不认识的结果（${outcome}），没有当成登上了。` };
  return { done: outcome !== 'OPERATOR_NOT_FOUND', text };
}
