import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import {
  decisionClearsRequest,
  decisionDraftOf,
  decisionNoteOf,
  directionBlockedReason,
  directionOptions,
  manualItemKeyOf,
  manualItemLineOf,
  manualResultBlockedReason,
  manualResultDraftOf,
  manualResultNoteOf,
  manualSummaryText,
  requestDetailRows,
  requestQueueRowOf,
  requestQueueViewState,
  requestSummaryText,
} from './post-intake-disposition';
import { mayMakeDecision, heldGrantsOf, postIntakeDispositionDecision, postIntakeManualItemDecision } from '../operator-grants';
import type { ApiResult, PostIntakeDispositionDecisionResponseBody, PostIntakeDispositionRequestRow, PostIntakeManualItemRow } from './api';

// 本文件钉收寄后处置页的呈现逻辑(UC-PS-006;ADR-0332、ADR-0341)。期望值取服务端响应的原词。

const moduleInfo = { title: '收寄后处置决定', owner: '小包托运(parcel-shipment)', source: 'docs/adr/0332' };

function requestOf(overrides: Partial<PostIntakeDispositionRequestRow> = {}): PostIntakeDispositionRequestRow {
  return { requestKey: 'RK-1', parcelId: 'PARCEL-1', origin: 'CUSTOMER_CHANNEL', recordedAt: '2026-10-27T09:00:00Z', currentVersion: 0, ...overrides };
}
function itemOf(overrides: Partial<PostIntakeManualItemRow> = {}): PostIntakeManualItemRow {
  return {
    parcelId: 'PARCEL-1', decisionVersion: 1, owner: 'NODE_OPERATIONS', action: 'HOLD_AT_CONTROL', direction: 'INTERCEPT',
    decidedAt: '2026-10-27T10:00:00Z', currentVersion: 1, supersededByLaterDecision: false, ...overrides,
  };
}

test('列表:去向集恰是五种;队列行带包裹、请求键与链尾版本;摘要说明截断', () => {
  deepEqual(directionOptions.map((o) => o.id), ['CONTINUE', 'INTERCEPT', 'REDIRECT', 'RETURN', 'TERMINATE_SERVICE']);
  const line = requestQueueRowOf(requestOf({ currentVersion: 2 }));
  equal(line.title, 'PARCEL-1');
  match(line.subtitle, /RK-1/);
  equal(line.statusText, '链尾 v2');
  equal(requestQueueRowOf(requestOf()).statusText, '尚无处置决定');
  equal(requestSummaryText({ outcome: 'POST_INTAKE_DISPOSITION_REQUESTS_LISTED', pendingCount: 5, truncated: true, requests: [requestOf()] }), '等回应 5 件(只列出前 1 件,回应之后其余的会补进来)');
});

test('列表:未保全的字段如实说未保全,不说没有请求方', () => {
  const rows = requestDetailRows(requestOf());
  match(rows.find((r) => r.label === '请求方')!.value, /未保全/);
});

test('列表:空清单是答案,读不到不画成空', () => {
  const empty = requestQueueViewState({ kind: 'outcome', status: 200, body: { outcome: 'POST_INTAKE_DISPOSITION_REQUESTS_LISTED', pendingCount: 0, truncated: false, requests: [] } }, () => undefined, moduleInfo);
  equal(empty.kind, 'empty');
  equal(requestQueueViewState({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }, () => undefined, moduleInfo).kind, 'error');
  equal(requestQueueViewState({ kind: 'unconfigured' } as never, () => undefined, moduleInfo).kind, 'unconfigured');
});

test('决定:草案回应请求、前一版本取链尾,只有改送带目的地', () => {
  const draft = decisionDraftOf(requestOf({ currentVersion: 3 }), 'INTERCEPT', ' 核对后拦截 ', '不该带');
  deepEqual(draft, { parcel: 'PARCEL-1', supersedes: 3, direction: 'INTERCEPT', requestKey: 'RK-1', basis: '核对后拦截' });
  equal(decisionDraftOf(requestOf(), 'REDIRECT', 'x', ' DEST-1 ').destination, 'DEST-1');
  ok(directionBlockedReason('REDIRECT', '  '));
  equal(directionBlockedReason('REDIRECT', 'D'), undefined);
  equal(directionBlockedReason('RETURN', ''), undefined);
});

test('决定:每格业务答案按词表呈现;前一份已变不清请求', () => {
  const decided: ApiResult<PostIntakeDispositionDecisionResponseBody> = { kind: 'outcome', status: 201, body: { outcome: 'DECIDED', decision: { version: 1, supersedes: 0, direction: 'INTERCEPT', source: 'CUSTOMER_REQUEST', responsibilityStart: '1', decidedAt: 't', ownerRequests: [{ owner: 'NODE_OPERATIONS', action: 'HOLD_AT_CONTROL' }] } } };
  const note = decisionNoteOf('决定', decided);
  equal(note.tone, 'outcome');
  match(note.text, /DECIDED/);
  match(note.text, /节点作业:在控制点扣留/);
  ok(decisionClearsRequest(decided));
  const moved: ApiResult<PostIntakeDispositionDecisionResponseBody> = { kind: 'outcome', status: 200, body: { outcome: 'PREDECESSOR_MOVED' } };
  match(decisionNoteOf('决定', moved).text, /重读清单/);
  ok(!decisionClearsRequest(moved));
  match(decisionNoteOf('决定', { kind: 'outcome', status: 200, body: { outcome: 'FINAL_OUTCOME_STANDS', finalBasis: 'FINAL-1' } }).text, /FINAL-1/);
  equal(decisionNoteOf('决定', { kind: 'notGranted' } as never).tone, 'refused');
  match(decisionNoteOf('决定', { kind: 'callerProblem', status: 404, code: 'POST_INTAKE_DISPOSITION_REQUEST_NOT_VISIBLE' }).text, /重读清单/);
});

test('登记:被取代的项带标志文案、已登记项带结果;键四段拼成', () => {
  const superseded = manualItemLineOf(itemOf({ currentVersion: 2, supersededByLaterDecision: true }));
  ok(superseded.superseded);
  match(superseded.supersededText, /链尾 v2/);
  equal(manualItemLineOf(itemOf()).supersededText, '');
  equal(manualItemKeyOf(itemOf()), 'PARCEL-1#1#NODE_OPERATIONS#HOLD_AT_CONTROL');
  const done = manualItemLineOf(itemOf({ result: { outcome: 'UNABLE', note: '现场找不到', handledBy: 'op-1', handledAt: '2026-10-27T11:00:00Z' } }));
  match(done.resultText, /无法处理 · 现场找不到/);
});

test('登记:说明必填;草案带四段键、结果与说明;重放与冲突如实呈现', () => {
  ok(manualResultBlockedReason('  '));
  equal(manualResultBlockedReason('已扣留'), undefined);
  deepEqual(manualResultDraftOf(itemOf(), 'HANDLED', ' 已扣留 '), { parcel: 'PARCEL-1', decisionVersion: 1, owner: 'NODE_OPERATIONS', action: 'HOLD_AT_CONTROL', outcome: 'HANDLED', note: '已扣留' });
  const item = itemOf({ result: { outcome: 'HANDLED', note: '已扣留', handledBy: 'op', handledAt: 't' } });
  match(manualResultNoteOf('登记', { kind: 'outcome', status: 201, body: { outcome: 'REGISTERED', item } }).text, /已登记/);
  match(manualResultNoteOf('登记', { kind: 'outcome', status: 200, body: { outcome: 'ALREADY_REGISTERED', item } }).text, /此前已登记/);
  match(manualResultNoteOf('登记', { kind: 'callerProblem', status: 409, code: 'X' }).text, /另一个结果/);
  match(manualResultNoteOf('登记', { kind: 'callerProblem', status: 404, code: 'POST_INTAKE_MANUAL_ITEM_NOT_VISIBLE' }).text, /重读清单/);
});

test('读回:按包裹摘要说名下项数与未登记数', () => {
  equal(manualSummaryText({ outcome: 'POST_INTAKE_MANUAL_ITEMS_LISTED', pendingCount: 1, truncated: false, items: [itemOf(), itemOf({ owner: 'NETWORK_ROUTING' })] }, 'PARCEL-1'), '包裹 PARCEL-1 名下共 2 项,其中 1 项尚未登记结果');
});

test('写入口按授予收起:明确没有才收起,不知道就摆;两种运营决定分开授予', () => {
  equal(mayMakeDecision(undefined, postIntakeDispositionDecision), true);
  const held = heldGrantsOf({ kind: 'outcome', status: 200, body: { outcome: 'OPERATOR_GRANTS_RESOLVED', grants: [], decisions: [postIntakeDispositionDecision] } });
  equal(mayMakeDecision(held, postIntakeDispositionDecision), true);
  equal(mayMakeDecision(held, postIntakeManualItemDecision), false);
});
