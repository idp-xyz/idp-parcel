// 授权规则发布表单的纯逻辑（票 admin-write-faces/17）：草稿形状、草稿 → 载荷、表单认领的 JSON 路径、请求方下拉的选项
// 来源判读。全部是纯函数，node:test 钉着；组件 AuthorizationRulePublicationForm.tsx 只负责摆。写法照 customer-contract-form.ts。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。取消授权目录一行两格（请求方 / 规则引用）照草稿原样送
// 上去：没选请求方、没填规则、同一请求方写了两行、一行都没有——都由服务端答（逐格问题或预览上的`未受理`带成因），
// 表单不代判也不静默补齐、不给默认不预选。请求方的封闭集从服务端词表读口取（票 20），这里没有任何一份内置的码。

import type { ApiResult } from '../catalogue-api';
import {
  vocabularyOptions,
  type CommercialPublicationPayload,
  type PublicationVocabularyResponseBody,
  type VocabularyOption,
} from './publication-draft-api';

/** 目录的一行草稿：`party` 是请求方原词（`''` = 还没选），`rule` 是规则引用串。 */
export interface CancellationRowDraft {
  party: string;
  rule: string;
}

export interface AuthorizationRuleDraft {
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  rows: CancellationRowDraft[];
}

export function emptyCancellationRowDraft(): CancellationRowDraft {
  return { party: '', rule: '' };
}

export function emptyAuthorizationRuleDraft(): AuthorizationRuleDraft {
  return {
    objectId: '',
    version: '',
    scope: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    rows: [emptyCancellationRowDraft()],
  };
}

/**
 * 草稿 → 载荷。可缺的壳格留空即不进载荷（Go 侧 omitempty 同义）；目录一节永远在场、行照原样——零行也送空数组，
 * 「一行都没有」由服务端答缺件，不是表单替人决定的事。载荷里没有身份也没有摘要。
 */
export function payloadOf(draft: AuthorizationRuleDraft): CommercialPublicationPayload {
  const payload: CommercialPublicationPayload = {
    kind: 'AUTHORIZATION_RULE',
    objectId: draft.objectId,
    version: draft.version,
    scope: draft.scope,
    effectiveStartsAt: draft.effectiveStartsAt,
    authorizationRule: {
      cancellationAuthority: draft.rows.map((row) => ({ party: row.party, rule: row.rule })),
    },
  };
  if (draft.effectiveEndsAt !== '') payload.effectiveEndsAt = draft.effectiveEndsAt;
  return payload;
}

const shellPaths = ['objectId', 'version', 'scope', 'effectiveStartsAt', 'effectiveEndsAt'] as const;

/**
 * 表单渲染了哪几条 JSON 路径（流程组件把服务端点名的其余路径单独列出）。目录按行数长：服务端对整行与行里每一格
 * 都可能点名。
 */
export function authorizationRuleFieldPaths(draft: AuthorizationRuleDraft): string[] {
  const paths: string[] = [...shellPaths, 'authorizationRule', 'authorizationRule.cancellationAuthority'];
  draft.rows.forEach((_, index) => {
    const row = `authorizationRule.cancellationAuthority[${index}]`;
    paths.push(row, `${row}.party`, `${row}.rule`);
  });
  return paths;
}

/**
 * 请求方下拉的选项来源，由词表读口的答复判读：读到了就是服务端码 × 本页中文词表（vocabularyOptions，集外原样）；
 * 403 是可辨的「未配置」——下拉显占位、不内置任何码顶替；别的失败与「答复里没有 `party` 集」都是读不到，同样只显占位。
 * 选项里没有任何一项带「选中」：不给默认、不预选归表单硬句。
 */
export type PartyOptionsState =
  | { kind: 'loading' }
  | { kind: 'options'; options: VocabularyOption[] }
  | { kind: 'unconfigured' }
  | { kind: 'unavailable' };

export function partyOptionsOf(
  answer: ApiResult<PublicationVocabularyResponseBody> | null,
  labels: Record<string, string>,
): PartyOptionsState {
  if (answer === null) return { kind: 'loading' };
  if (answer.kind === 'unconfigured') return { kind: 'unconfigured' };
  if (answer.kind !== 'outcome') return { kind: 'unavailable' };
  const party = answer.body.sets.find((set) => set.name === 'party');
  if (!party) return { kind: 'unavailable' };
  return { kind: 'options', options: vocabularyOptions(party.codes, labels) };
}
