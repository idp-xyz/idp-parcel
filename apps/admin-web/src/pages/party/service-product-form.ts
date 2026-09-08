// 服务产品版本表单的纯逻辑（票 admin-write-faces/09；ADR-0101 决定八逐字段表单）。全部是纯函数，
// node:test 钉着；组件 ServiceProductPublicationForm.tsx 只负责摆，五步由 PublicationDraftFlow 走。
//
// **本册的载荷就是壳。** 服务产品版本没有正文——发布的是给合同、接单规则包、价格政策引用的版本身份；
// 产品属性与渠道映射走另一条登记路（register_products）。所以草稿只有壳四格、有效起止与引用表，
// 组出来的 CommercialPublicationPayload 没有任何正文格（与 Go 侧 publication_draft_payload.go 对本册
// 不加格是同一件事）。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。空字段、时刻格式、区间先后、引用键是
// 不是集合内的词，一律原样送上去让服务端逐格答；这里只做两件编码层的事：可缺的键缺席而不是空串，
// 以及引用表里同一个键填了两行时如实报出来——JSON 对象里放不下两格同名键，静默留一行就是丢输入。
//
// **表单不得替操作者拟引用键**（票 09 硬句）：`references` 是开放词汇，键从哪来由发布用例与领域答；
// 这里的引用表是「加一行」不是「从这几个里挑」，今天服务端没有词表读口，本文件也不内置一份。

import type { CommercialPublicationPayload } from './publication-draft-api';

/** 引用表的一行：被引对象类别（原词，操作者自填）→ 对象标识。 */
export interface ReferenceRowDraft {
  kind: string;
  objectId: string;
}

export interface ServiceProductDraft {
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  /** 留空即无上界（载荷里缺席，不送空串）。 */
  effectiveEndsAt: string;
  references: ReferenceRowDraft[];
}

export function emptyServiceProductDraft(): ServiceProductDraft {
  return { objectId: '', version: '', scope: '', effectiveStartsAt: '', effectiveEndsAt: '', references: [] };
}

export function emptyReferenceRow(): ReferenceRowDraft {
  return { kind: '', objectId: '' };
}

/** 两格都空的行是还没填的「加一行」，组载荷时跳过；填了任何一格就算一行，原样送上去。 */
export function isBlankReferenceRow(row: ReferenceRowDraft): boolean {
  return row.kind === '' && row.objectId === '';
}

/** 壳上各格在载荷里的 JSON 路径（服务端逐格问题按它点名）。 */
export const serviceProductShellPaths = [
  'objectId',
  'version',
  'scope',
  'effectiveStartsAt',
  'effectiveEndsAt',
] as const;

/** 引用表某一行的 JSON 路径：服务端对键与值的问题都记在 `references.<键>` 上。 */
export function referencePath(row: ReferenceRowDraft): string {
  return `references.${row.kind}`;
}

/**
 * 表单渲染了哪几条 JSON 路径：壳四格加有效起止，再加引用表每一行（按键）。流程组件拿它分辨
 * 「服务端点名的路径有没有格接住」，没接住的它自己单列。
 */
export function serviceProductFieldPaths(draft: ServiceProductDraft): string[] {
  const paths: string[] = [...serviceProductShellPaths];
  for (const row of draft.references) {
    if (!isBlankReferenceRow(row)) paths.push(referencePath(row));
  }
  return paths;
}

/**
 * 草稿 → 载荷。壳各格原样带；`effectiveEndsAt` 与 `references` 为空时**缺席**而不是空值——服务端按键
 * 在场与否分辨「没有」，空串与空对象在那边不是同一句话。引用表里两格全空的行跳过；半填的行照送，
 * 由服务端答哪一格立不住。同键两行时后一行覆盖前一行——那是 JSON 对象的形状所致，本模块用
 * `serviceProductLocalProblems` 把它报出来，流程组件在问题清零之前不放预览。
 */
export function serviceProductPayloadOf(draft: ServiceProductDraft): CommercialPublicationPayload {
  const payload: CommercialPublicationPayload = {
    kind: 'SERVICE_PRODUCT',
    objectId: draft.objectId,
    version: draft.version,
    scope: draft.scope,
    effectiveStartsAt: draft.effectiveStartsAt,
  };
  if (draft.effectiveEndsAt !== '') payload.effectiveEndsAt = draft.effectiveEndsAt;

  const references: Record<string, string> = {};
  let hasRows = false;
  for (const row of draft.references) {
    if (isBlankReferenceRow(row)) continue;
    references[row.kind] = row.objectId;
    hasRows = true;
  }
  if (hasRows) {
    // 线格式把键收窄到 CommercialObjectKindName 那个封闭集；表单按硬句不替操作者挑词，键原样送、集合外由
    // 服务端在 `references.<键>` 上逐格答。这里只是把开放词汇的对象放进收窄了的槽位，不是断言键一定在集合内。
    payload.references = references as CommercialPublicationPayload['references'];
  }
  return payload;
}

/**
 * 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。只有一种：引用表同一个键填了两行——
 * 载荷里放不下，留一行就是静默丢输入。空键、空值、集合外的键都不在这里判，那是服务端的话。
 */
export function serviceProductLocalProblems(draft: ServiceProductDraft): Record<string, string[]> {
  const seen = new Map<string, number>();
  for (const row of draft.references) {
    if (isBlankReferenceRow(row)) continue;
    seen.set(row.kind, (seen.get(row.kind) ?? 0) + 1);
  }
  const problems: Record<string, string[]> = {};
  for (const [kind, count] of seen) {
    if (count > 1) {
      problems[`references.${kind}`] = [`同一个被引对象类别填了 ${count} 行，载荷里只能有一格；删掉多出的行`];
    }
  }
  return problems;
}
