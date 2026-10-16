// 操作者绑定答复的判读（票 group-legal-entities-uiux/11）。`GET /operator-binding` 答这个会话的操作者在操作者册上绑的那一个
// 租户——`id_token` 里没有租户声明，绑定只登在服务端（ADR-0100 决定二）。纯函数，node:test 钉着；取数与钩子在
// use-operator-binding.ts。只做类型导入，好让 run-tests 的 CommonJS 发射直接加载本文件。

import type { ApiResult } from './catalogue-api';

export interface OperatorBindingResponseBody {
  outcome: string;
  tenantId: string;
}

/**
 * 判成当前集团（租户）标识。只有业务成格且带非空标识才算拿到；未配置、拒绝、依赖故障、读不到、成格却缺标识一律
 * undefined——顶栏作用域照旧显「由服务端按会话判定」，页头不显集团，不拿空串或猜的值顶上。
 */
export function boundTenantOf(answer: ApiResult<OperatorBindingResponseBody> | null): string | undefined {
  if (answer?.kind !== 'outcome' || answer.body.outcome !== 'OPERATOR_BINDING_RESOLVED') return undefined;
  const tenant = typeof answer.body.tenantId === 'string' ? answer.body.tenantId.trim() : '';
  return tenant === '' ? undefined : tenant;
}
