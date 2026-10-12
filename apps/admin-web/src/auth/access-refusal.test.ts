import { afterEach, test } from 'node:test';
import { deepEqual, equal, match } from 'node:assert/strict';
import {
  accessRefusalHeadline,
  accessRefusalOf,
  isOperatorRefusal,
  operatorRefusalSentence,
} from './access-refusal';
import { exchangeMasterData, postMasterData } from '../pages/catalogue-api';
import { listShipmentRequestViews } from '../pages/shipment-request/api';
import { listTrackingProjections } from '../pages/visibility/api';

// 票 operator-channel/07「三态文案」的判据那一半：接入那一格按状态码与错误码原名一起判成三格（ADR-0100 决定四），
// 别的 401 / 403 不冒充这三格。各 fetch 出口共用 accessRefusalOf，下面再逐个出口钉一遍「判法相同」——新增出口时
// 把它的读与写加进 everyExit。

// Covers: 三格各自的状态码 + 码名。
test('三格各按自己的状态码与码名判出', () => {
  deepEqual(accessRefusalOf(403, 'ACCESS_CHANNEL_NOT_CONFIGURED'), { kind: 'unconfigured' });
  deepEqual(accessRefusalOf(401, 'OPERATOR_CREDENTIAL_REJECTED'), { kind: 'loginRequired' });
  deepEqual(accessRefusalOf(403, 'OPERATOR_NOT_GRANTED'), { kind: 'notGranted' });
});

// Covers: 只对上一半（码名或状态码）不算；别族的拒绝码（集成客户端那一族）不算。
test('码名与状态码只对上一半、或别的 401 / 403，都不冒充这三格', () => {
  equal(accessRefusalOf(403, 'OPERATOR_CREDENTIAL_REJECTED'), null);
  equal(accessRefusalOf(401, 'OPERATOR_NOT_GRANTED'), null);
  equal(accessRefusalOf(401, 'ACCESS_CHANNEL_NOT_CONFIGURED'), null);
  equal(accessRefusalOf(403, 'INTEGRATION_CLIENT_NOT_GRANTED'), null);
  equal(accessRefusalOf(401, 'UNKNOWN'), null);
  equal(accessRefusalOf(503, 'OPERATOR_NOT_GRANTED'), null);
});

// Covers: 未配置那一句与各页早已在用的句子逐字相同；两格操作者答复的整句带续办。
test('标题句带状态码与码名原名；操作者两格的整句带续办', () => {
  equal(accessRefusalHeadline('unconfigured'), '接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）');
  equal(accessRefusalHeadline('loginRequired'), '需要登录（401 OPERATOR_CREDENTIAL_REJECTED）');
  equal(accessRefusalHeadline('notGranted'), '没有授予（403 OPERATOR_NOT_GRANTED）');
  match(operatorRefusalSentence('loginRequired', '预览'), /^预览：需要登录（401 OPERATOR_CREDENTIAL_REJECTED）。.*重新登录后再试/);
  match(operatorRefusalSentence('notGranted'), /^没有授予（403 OPERATOR_NOT_GRANTED）。.*要管授予的人去登记授予/);
});

// Covers: 守卫只认两格操作者答复——未配置各页有自己那段说明，不经它。
test('isOperatorRefusal 只认需要登录与没有授予', () => {
  equal(isOperatorRefusal({ kind: 'loginRequired' }), true);
  equal(isOperatorRefusal({ kind: 'notGranted' }), true);
  equal(isOperatorRefusal({ kind: 'unconfigured' }), false);
  equal(isOperatorRefusal({ kind: 'callerProblem' }), false);
  equal(isOperatorRefusal({ kind: 'outcome' }), false);
});

const realFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = realFetch;
});

function answerWith(status: number, code: string): void {
  globalThis.fetch = (async () => new Response(JSON.stringify({ error: { code } }), { status })) as typeof fetch;
}

const everyExit: Array<[string, () => Promise<{ kind: string }>]> = [
  ['目录读口', () => exchangeMasterData('/pricing-price-cards')],
  ['登记写口', () => postMasterData('/pricing-price-card-registrations', {})],
  ['委托查阅', () => listShipmentRequestViews()],
  ['全程追踪', () => listTrackingProjections()],
];

// Covers: 每个出口都把三格判成同一格，别的 401 / 403 仍交回调用方问题，不被吞进三格。
test('各出口对三格判法相同；别的 401 / 403 照旧是调用方问题', async () => {
  for (const [exit, call] of everyExit) {
    answerWith(403, 'ACCESS_CHANNEL_NOT_CONFIGURED');
    equal((await call()).kind, 'unconfigured', exit);
    answerWith(401, 'OPERATOR_CREDENTIAL_REJECTED');
    equal((await call()).kind, 'loginRequired', exit);
    answerWith(403, 'OPERATOR_NOT_GRANTED');
    equal((await call()).kind, 'notGranted', exit);
    answerWith(401, 'SOMETHING_ELSE');
    equal((await call()).kind, 'callerProblem', exit);
    answerWith(403, 'SOMETHING_ELSE');
    equal((await call()).kind, 'callerProblem', exit);
  }
});
