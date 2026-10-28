import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import {
  handlingDraftOf,
  refusalActionOptions,
  refusalHandlingNoteOf,
  refusalKeyOf,
  refusalQueueRowOf,
  refusalQueueViewState,
  refusalScopeText,
  refusalSummaryText,
  refusalTechnicalRows,
} from './control-release-refusals';
import type { ControlReleaseRefusalRow } from './api';

// 本包 @types/node 的 assert/strict 声明里没有 doesNotMatch;反向断言照本目录既有写法用 ok(!re.test())。
function doesNotMatch(value: string, pattern: RegExp): void {
  ok(!pattern.test(value), `不该匹配 ${pattern} 却匹配了:${value}`);
}

// 本文件钉资金释放待处理页的呈现逻辑(票 docs-consistency/code/95;ADR-0293 决定一至三)。
// 屏上是运营的话:结果码与标识只进技术详情。

const moduleInfo = {
  title: '资金释放待处理',
  owner: '小包托运(parcel-shipment)',
  source: 'docs/adr/0282',
};

function rowOf(overrides: Partial<ControlReleaseRefusalRow> = {}): ControlReleaseRefusalRow {
  return {
    shipmentRequestId: 'SHR-REQ1',
    submissionVersionId: 'REQ-1/v1',
    controlResultId: 'REQ-1/v1',
    customerAccountId: 'CUST-1',
    scopeRecorded: true,
    occupationScope: { legalEntity: 'LE-1', settlementAccount: 'ACCT-1', currency: 'CNY' },
    reason: 'SA-RELEASE_REQUEST_NOT_ACCEPTED',
    recordedAt: '2026-10-26T09:00:00Z',
    ...overrides,
  };
}

// Covers: ADR-0293 决定二——处理动作恰是两种,没有第三种;「标记已处理」不冒充释放。
test('动作集恰是核对后释放与标记已处理', () => {
  deepEqual(
    refusalActionOptions.map((option) => option.id),
    ['RELEASE_AT_SETTLEMENT', 'MARK_HANDLED'],
  );
  equal(refusalActionOptions[0].label, '核对后释放');
  equal(refusalActionOptions[1].label, '标记已处理');
});

// Covers: 队列行的键由(委托、版本、控制关联)三段拼成,同一委托上不同占用不撞键;行显去前缀的运单号与客户,版本进技术详情。
test('队列行的键逐段拼成,不同占用不撞键;版本与标识进技术详情', () => {
  const first = rowOf();
  const second = rowOf({ submissionVersionId: 'REQ-1/v2', controlResultId: 'REQ-1/v2' });
  ok(refusalKeyOf(first) !== refusalKeyOf(second));
  const line = refusalQueueRowOf(first);
  equal(line.id, refusalKeyOf(first));
  equal(line.title, 'REQ1');
  equal(line.subtitle, '客户 CUST-1');
  doesNotMatch(line.subtitle, /REQ-1\/v1/);
  equal(line.statusText, '待处理');
  deepEqual(refusalTechnicalRows(first).map((row) => row.label), ['委托标识', '客户账户', '被拒收的提交版本', '控制关联', '结算侧原因码']);
  equal(refusalTechnicalRows(first)[2].value, 'REQ-1/v1');
});

// Covers: ADR-0282 决定四——没存作用域的存量行如实标出,并说明「核对后释放」会再被结算侧拒绝、只剩标记已处理;不说「无」当成没有问题。
test('没记下占用位置的老数据标明释放会再被拒绝', () => {
  const legacy = rowOf({ scopeRecorded: false, occupationScope: undefined });
  equal(refusalQueueRowOf(legacy).statusText, '老数据,只能手工处理');
  const text = refusalScopeText(legacy);
  match(text, /没有记下/);
  match(text, /再被结算侧拒绝/);
  match(text, /标记已处理/);
  equal(refusalScopeText(rowOf()), 'LE-1 / ACCT-1 / CNY');
});

// Covers: 命令草案只带三段标识、动作与依据;不带处理人(那是「谁在签」,由服务端从认证翻译)。依据去两端空白。
test('草案不带处理人,依据去两端空白', () => {
  const draft = handlingDraftOf(rowOf(), 'MARK_HANDLED', '  已在结算侧手工释放  ');
  deepEqual(draft, {
    shipmentRequestId: 'SHR-REQ1',
    submissionVersionId: 'REQ-1/v1',
    controlResultId: 'REQ-1/v1',
    action: 'MARK_HANDLED',
    basis: '已在结算侧手工释放',
  });
  ok(!('handler' in draft));
});

// Covers: 三态之「空」——服务端作答且清单为空才是空队列,措辞说「没有积压」是答案,不说缺陷。
test('空清单是答案不是缺陷', () => {
  const state = refusalQueueViewState(
    { kind: 'outcome', status: 200, body: { outcome: 'CONTROL_RELEASE_REFUSALS_LISTED', unhandledCount: 0, truncated: false, refusals: [] } },
    () => {},
    moduleInfo,
  );
  ok(state.kind === 'empty');
  match(state.description ?? '', /没有积压/);
});

// Covers: 页头的计数与截断说明:计数是未处理的全部行数;清单被页大小截断时说清只列了最早的几笔。
test('计数与截断如实说明', () => {
  equal(
    refusalSummaryText({ outcome: 'CONTROL_RELEASE_REFUSALS_LISTED', unhandledCount: 2, truncated: false, refusals: [rowOf(), rowOf()] }),
    '待处理 2 笔',
  );
  const truncated = refusalSummaryText({ outcome: 'CONTROL_RELEASE_REFUSALS_LISTED', unhandledCount: 250, truncated: true, refusals: [rowOf()] });
  match(truncated, /待处理 250 笔/);
  match(truncated, /先列出最早登记的 1 笔/);
});

// Covers: ADR-0022——每一格 outcome 都是形成了的业务答案,各说各的话且不带码:已处理说系统接着处理、已被处理说先到那一份、
// 释放未收口说什么也没记可再试、再被拒绝指向标记已处理;结果码在技术详情。
test('每一格处理结果各说各的话,码进技术详情', () => {
  const handled = refusalHandlingNoteOf('标记已处理', {
    kind: 'outcome',
    status: 201,
    body: { outcome: 'HANDLED', handlingKind: 'MARKED_HANDLED', handledAt: '2026-10-27T09:00:00Z' },
  });
  equal(handled.tone, 'outcome');
  match(handled.text, /^已处理\(标记已处理 · 2026-10-27 09:00:00 UTC\)。系统会接着处理这票运单/);
  doesNotMatch(handled.text, /HANDLED|MARKED_HANDLED/);
  equal(handled.technical.find((row) => row.label === '结果码')?.value, 'HANDLED');

  const already = refusalHandlingNoteOf('标记已处理', {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'ALREADY_HANDLED', handlingKind: 'RELEASED_AT_SETTLEMENT', handledAt: '2026-10-27T08:00:00Z' },
  });
  match(already.text, /刚被别人处理了\(结算侧已释放/);
  doesNotMatch(already.text, /RELEASED_AT_SETTLEMENT/);

  const notSettled = refusalHandlingNoteOf('核对后释放', { kind: 'outcome', status: 200, body: { outcome: 'RELEASE_NOT_SETTLED' } });
  match(notSettled.text, /什么也没记/);
  match(notSettled.text, /稍后再试/);

  const again = refusalHandlingNoteOf('核对后释放', { kind: 'outcome', status: 200, body: { outcome: 'RELEASE_REFUSED_AGAIN', reason: 'SA-RELEASE_REQUEST_NOT_ACCEPTED' } });
  match(again.text, /又一次拒绝了释放/);
  match(again.text, /标记已处理/);
  doesNotMatch(again.text, /SA-RELEASE_REQUEST_NOT_ACCEPTED/);
});

// Covers: 失败各归其格——册上没有这一笔(404 统一不可见)说刷新清单,不说稍后重试;未配置、需要登录、没有授予各报自己的停点;
// 5xx 说没有处理成功;传输层说没送到;状态码与码名只在技术详情。
test('失败各归其格,码进技术详情', () => {
  const missing = refusalHandlingNoteOf('标记已处理', { kind: 'callerProblem', status: 404, code: 'SHIPMENT_REQUEST_NOT_VISIBLE' });
  equal(missing.tone, 'problem');
  match(missing.text, /已经没有这一笔/);
  doesNotMatch(missing.text, /404|NOT_VISIBLE|稍后再试/);
  equal(missing.technical.find((row) => row.label === 'HTTP 状态')?.value, '404');

  equal(refusalHandlingNoteOf('标记已处理', { kind: 'unconfigured' }).tone, 'unconfigured');
  const login = refusalHandlingNoteOf('标记已处理', { kind: 'loginRequired' });
  ok(login.tone === 'refused' && login.kind === 'loginRequired');
  const grant = refusalHandlingNoteOf('标记已处理', { kind: 'notGranted' });
  ok(grant.tone === 'refused' && grant.kind === 'notGranted');
  doesNotMatch(grant.text, /403|OPERATOR_NOT_GRANTED/);
  equal(grant.technical.find((row) => row.label === '问题码')?.value, 'OPERATOR_NOT_GRANTED');

  match(refusalHandlingNoteOf('标记已处理', { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }).text, /没有处理成功/);
  match(refusalHandlingNoteOf('标记已处理', { kind: 'transport', message: 'fetch failed' }).text, /没有送到服务器/);
});
