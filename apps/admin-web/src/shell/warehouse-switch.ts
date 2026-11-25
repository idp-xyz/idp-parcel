// 仓配履约部署开关在管理台这一侧的判读（ADR-0375 决定四）。纯函数、不引 React，node:test 直接钉；取一次开关的钩子在
// use-warehouse-switch.ts。只做类型导入，好让 run-tests 的 CommonJS 发射直接加载本文件。
//
// 开关关着时，parcel-api 的所有仓配端点（含 `/v1/warehouse/`）在认证之前答同一份固定的 403 问题详情、码
// WAREHOUSE_FULFILLMENT_NOT_ENABLED：不读令牌、报文与册，对任何调用方、任何请求都一样——不带令牌、没有授予也是它。
// 所以一次探测的答复里：
//   - 恰是这一对（403 + 这个码）          → 明确知道关着；
//   - 业务结果、接入三格拒绝（未配置、需要登录、没有授予）或别的问题码
//                                       → 请求已经过了开关那一道，开着；
//   - 5xx、传输失败                      → 没有答出来，不知道。
//
// **摆不摆只是呈现，不是裁决**（口径同 `mayMakeDecision`）：服务端照旧在每个仓配口上判开关，这里只在「明确知道关着」时才
// 收起仓配入口；开着、不知道、还没答出，一律照旧摆。把「别的 403」「别的问题码」判成关着，会在代理或网关答了一个不相干的
// 403 时把一个开着的部署的仓配整个藏起来，代价远大于多摆一个入口。

import type { ApiResult } from '../pages/catalogue-api';

/** 开关关着时仓配各口答的码，原词取自 warehousehttp.CodeWarehouseFulfillmentNotEnabled；状态码恒为 403。 */
export const WAREHOUSE_NOT_ENABLED_CODE = 'WAREHOUSE_FULFILLMENT_NOT_ENABLED';
const WAREHOUSE_NOT_ENABLED_STATUS = 403;

/** 部署开关的环境变量名，原词取自 warehouseswitch.EnabledEnv；只在说明页的「技术详情」里露出，部署方照它去开。 */
export const WAREHOUSE_SWITCH_ENV = 'IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED';

export type WarehouseSwitch = 'enabled' | 'disabled' | 'unknown';

/** 把一次仓配读口的探测答复判成开关的三态。状态码与问题码一起对上才算关着，与 access-refusal 一向的判法相同。 */
export function warehouseSwitchOf(answer: ApiResult<unknown>): WarehouseSwitch {
  switch (answer.kind) {
    case 'outcome':
    case 'unconfigured':
    case 'loginRequired':
    case 'notGranted':
      return 'enabled';
    case 'callerProblem':
      return answer.status === WAREHOUSE_NOT_ENABLED_STATUS && answer.code === WAREHOUSE_NOT_ENABLED_CODE
        ? 'disabled'
        : 'enabled';
    case 'noAnswer':
    case 'transport':
      return 'unknown';
  }
}

/** 仓配入口摆不摆：只有明确知道开关关着才收起；还没答（undefined）、答不出、答出开着都摆。 */
export function warehouseOffered(state: WarehouseSwitch | undefined): boolean {
  return state !== 'disabled';
}
