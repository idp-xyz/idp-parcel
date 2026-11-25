import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import type { ApiResult } from '../pages/catalogue-api';
import {
  WAREHOUSE_NOT_ENABLED_CODE,
  WAREHOUSE_SWITCH_ENV,
  warehouseOffered,
  warehouseSwitchOf,
  type WarehouseSwitch,
} from './warehouse-switch';

// 本文件钉的是仓配部署开关的判读，纯函数、不碰网络：期望值取服务端的原话（开关关着时仓配各口在认证之前答 403 + 固定问题码，
// 别的答复都说明请求已过了开关那一道），不从实现推。判读只在「明确知道关着」时才收起仓配入口。探针发出去的请求与真实答复的形状，
// 在 warehouse-switch-probe.test.ts 里钉。

const notEnabled: ApiResult<unknown> = { kind: 'callerProblem', status: 403, code: 'WAREHOUSE_FULFILLMENT_NOT_ENABLED' };

// Covers: 判读的每一格——业务结果、接入三格拒绝、别的 403 码、别的 4xx、关着、5xx、传输失败。
test('只有 403 加固定问题码判成关着；业务结果与各种拒绝判成开着；没答出来判成不知道', () => {
  const cells: ReadonlyArray<readonly [string, ApiResult<unknown>, WarehouseSwitch]> = [
    ['业务结果', { kind: 'outcome', status: 200, body: { outcome: 'WAREHOUSES_LISTED', warehouses: [] } }, 'enabled'],
    ['接入渠道未配置', { kind: 'unconfigured' }, 'enabled'],
    ['需要登录', { kind: 'loginRequired' }, 'enabled'],
    ['没有授予', { kind: 'notGranted' }, 'enabled'],
    ['别的 403 问题码', { kind: 'callerProblem', status: 403, code: 'INTAKE_FAILED' }, 'enabled'],
    ['403 认不出问题体', { kind: 'callerProblem', status: 403, code: 'UNKNOWN' }, 'enabled'],
    ['别的 4xx', { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' }, 'enabled'],
    ['开关关着', notEnabled, 'disabled'],
    ['固定问题码配了别的状态码', { kind: 'callerProblem', status: 404, code: WAREHOUSE_NOT_ENABLED_CODE }, 'enabled'],
    ['5xx', { kind: 'noAnswer', status: 503, code: 'NO_ANSWER_FORMED' }, 'unknown'],
    ['5xx 认不出问题体', { kind: 'noAnswer', status: 502, code: 'UNKNOWN' }, 'unknown'],
    ['传输失败', { kind: 'transport', message: 'Failed to fetch' }, 'unknown'],
  ];
  for (const [name, answer, want] of cells) {
    equal(warehouseSwitchOf(answer), want, name);
  }
});

// Covers: 摆不摆只在明确知道关着时才收起；还没答出（undefined）、答不出、答出开着都摆。
test('仓配入口只在明确知道关着时才不摆', () => {
  equal(warehouseOffered(undefined), true);
  equal(warehouseOffered('unknown'), true);
  equal(warehouseOffered('enabled'), true);
  equal(warehouseOffered('disabled'), false);
});

// 问题码与开关名是服务端与部署清单的原词：拼错一个字，关着的部署认不出自己关着，说明页也会把部署方指到不存在的开关上。
test('问题码与开关名等于服务端原词', () => {
  equal(WAREHOUSE_NOT_ENABLED_CODE, 'WAREHOUSE_FULFILLMENT_NOT_ENABLED');
  equal(WAREHOUSE_SWITCH_ENV, 'IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED');
});
