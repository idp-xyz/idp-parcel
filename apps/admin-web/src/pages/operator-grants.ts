// 操作者授予自查答复的判读（ADR-0315）。`GET /operator-grants` 答这个会话的操作者此刻持有哪几格能力面授予与哪几种运营决定授予；
// 管理台据此不摆这个操作者做不了的写入口。纯函数，node:test 钉着；取数与钩子在 use-operator-grants.ts。只做类型导入，好让
// run-tests 的 CommonJS 发射直接加载本文件。
//
// **摆不摆只是呈现，不是裁决**：每个写口照旧在服务端判授予，拒绝仍是服务端的答案。所以这里只在「明确知道没有」时才收起入口——
// 答不出（未配置、拒绝、依赖故障、读不到、答复形状不对）一律当作不知道，入口照摆，让服务端去答。

import type { ApiResult } from './catalogue-api';

export interface OperatorGrantsResponseBody {
  outcome: string;
  grants: string[];
  decisions: string[];
}

/** 登记册配置写：ADR-0085 那一族登记与发布端点的能力面，原词取自 accessidentity.CapabilityRegistryConfigurationWrite。 */
export const registryWriteFace = 'REGISTRY_CONFIGURATION_WRITE';

export interface HeldGrants {
  faces: ReadonlySet<string>;
  decisions: ReadonlySet<string>;
}

/**
 * 判成持有的授予。只有业务成格且两个数组都在场才算拿到；其余各态一律 undefined——「不知道」，不折成「什么都没有」。
 */
export function heldGrantsOf(answer: ApiResult<OperatorGrantsResponseBody> | null): HeldGrants | undefined {
  if (answer?.kind !== 'outcome' || answer.body?.outcome !== 'OPERATOR_GRANTS_RESOLVED') return undefined;
  const { grants, decisions } = answer.body;
  if (!Array.isArray(grants) || !Array.isArray(decisions)) return undefined;
  return { faces: new Set(grants), decisions: new Set(decisions) };
}

/** 登记与发布入口摆不摆：明确知道没有登记册配置写授予才收起，不知道就摆。 */
export function mayWriteRegistry(held: HeldGrants | undefined): boolean {
  return held === undefined || held.faces.has(registryWriteFace);
}

/** 入口按授予收起时，原本指向它的文案改说这一句：说谁能用，不让人去找本页没有摆出的签或按钮。 */
export const hiddenEntryHint = '登记与发布入口只对持登记册配置写授予的操作者显示。';

/**
 * 空态与说明里指向写入口的那半句。入口摆着时照说它在哪（`pointer`）；入口按授予收起时换成 hiddenEntryHint——文案跟着入口走，
 * 否则查阅员会被指去一个看不见的「登记」签。
 */
export function entryHint(mayWrite: boolean, pointer: string): string {
  return mayWrite ? pointer : hiddenEntryHint;
}
