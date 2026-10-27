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
} from './control-release-refusals';
import type { ControlReleaseRefusalRow } from './api';

// 本包 @types/node 的 assert/strict 声明里没有 doesNotMatch;反向断言照本目录既有写法用 ok(!re.test())。
function doesNotMatch(value: string, pattern: RegExp): void {
  ok(!pattern.test(value), `不该匹配 ${pattern} 却匹配了:${value}`);
}

// 本文件钉释放未受理册查阅与处理页的呈现逻辑(票 docs-consistency/code/95;ADR-0293 决定一至三)。
// 期望值取服务端响应的原词与 ADR 原句,不从实现推。

const moduleInfo = {
  title: '释放拒收处理',
  owner: '小包托运(parcel-shipment)',
  source: 'docs/adr/0282',
};

function rowOf(overrides: Partial<ControlReleaseRefusalRow> = {}): ControlReleaseRefusalRow {
  return {
    shipmentRequestId: 'REQ-1',
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
test('动作集恰是在结算侧释放与标记已处理', () => {
  deepEqual(
    refusalActionOptions.map((option) => option.id),
    ['RELEASE_AT_SETTLEMENT', 'MARK_HANDLED'],
  );
  equal(refusalActionOptions[0].label, '核对后在结算侧释放');
  equal(refusalActionOptions[1].label, '标记已处理');
});

// Covers: 队列行的键由（委托、版本、控制关联）三段拼成,同一委托上不同占用不撞键;行文案带客户账户、被拒收的版本与登记时刻。
test('队列行的键逐段拼成,不同占用不撞键', () => {
  const first = rowOf();
  const second = rowOf({ submissionVersionId: 'REQ-1/v2', controlResultId: 'REQ-1/v2' });
  ok(refusalKeyOf(first) !== refusalKeyOf(second));
  const line = refusalQueueRowOf(first);
  equal(line.id, refusalKeyOf(first));
  equal(line.title, 'REQ-1');
  match(line.subtitle, /CUST-1/);
  match(line.subtitle, /REQ-1\/v1/);
  equal(line.statusText, '作用域已存下');
});

// Covers: ADR-0282 决定四——没存作用域的存量行如实标出,并说明「在结算侧释放」会再被提供方拒收、只剩标记已处理;不说「无作用域」当成没有问题。
test('没存作用域的存量行标明释放会再被拒收', () => {
  const legacy = rowOf({ scopeRecorded: false, occupationScope: undefined });
  equal(refusalQueueRowOf(legacy).statusText, '未存作用域(存量行)');
  const text = refusalScopeText(legacy);
  match(text, /未存下/);
  match(text, /再被提供方拒收/);
  match(text, /标记已处理/);
  equal(refusalScopeText(rowOf()), 'LE-1 / ACCT-1 / CNY');
});

// Covers: 命令草案只带三段标识、动作与依据;不带处理人(那是「谁在签」,由服务端从认证翻译)。依据去两端空白。
test('草案不带处理人,依据去两端空白', () => {
  const draft = handlingDraftOf(rowOf(), 'MARK_HANDLED', '  已在结算侧手工释放  ');
  deepEqual(draft, {
    shipmentRequestId: 'REQ-1',
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
    {
      kind: 'outcome',
      status: 200,
      body: { outcome: 'CONTROL_RELEASE_REFUSALS_LISTED', unhandledCount: 0, truncated: false, refusals: [] },
    },
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
    '未处理 2 笔',
  );
  const truncated = refusalSummaryText({
    outcome: 'CONTROL_RELEASE_REFUSALS_LISTED',
    unhandledCount: 250,
    truncated: true,
    refusals: [rowOf()],
  });
  match(truncated, /未处理 250 笔/);
  match(truncated, /只列出最早登记的 1 笔/);
});

// Covers: ADR-0022——每一格 outcome 都是形成了的业务答案,各说各的话:已处理说续办已交、已被处理说先到那一份、释放未收口说什么也没写可再试、
// 再被拒收带提供方原因并指向标记已处理。
test('每一格处理结果各说各的话', () => {
  const handled = refusalHandlingNoteOf('标记已处理', {
    kind: 'outcome',
    status: 201,
    body: { outcome: 'HANDLED', handlingKind: 'MARKED_HANDLED', handledAt: '2026-10-27T09:00:00Z' },
  });
  equal(handled.tone, 'outcome');
  match(handled.text, /HANDLED/);
  match(handled.text, /接受链/);

  const already = refusalHandlingNoteOf('标记已处理', {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'ALREADY_HANDLED', handlingKind: 'RELEASED_AT_SETTLEMENT', handledAt: '2026-10-27T08:00:00Z' },
  });
  equal(already.tone, 'outcome');
  match(already.text, /先到/);
  match(already.text, /RELEASED_AT_SETTLEMENT/);

  const notSettled = refusalHandlingNoteOf('核对后在结算侧释放', {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'RELEASE_NOT_SETTLED' },
  });
  equal(notSettled.tone, 'outcome');
  match(notSettled.text, /什么也没写/);
  match(notSettled.text, /稍后可再试/);

  const again = refusalHandlingNoteOf('核对后在结算侧释放', {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'RELEASE_REFUSED_AGAIN', reason: 'SA-RELEASE_REQUEST_NOT_ACCEPTED' },
  });
  equal(again.tone, 'outcome');
  match(again.text, /SA-RELEASE_REQUEST_NOT_ACCEPTED/);
  match(again.text, /什么也没写/);
  match(again.text, /标记已处理/);
});

// Covers: 失败各归其格——册上没有这一笔(404 统一不可见)是终局答案,不说稍后重试;未配置、需要登录、没有授予各报自己的停点;
// 5xx 说服务端没形成答案;传输层说请求可能没到。
test('失败各归其格', () => {
  const missing = refusalHandlingNoteOf('标记已处理', { kind: 'callerProblem', status: 404, code: 'SHIPMENT_REQUEST_NOT_VISIBLE' });
  equal(missing.tone, 'problem');
  match(missing.text, /册上已没有这一笔|不可见/);
  doesNotMatch(missing.text, /稍后重试/);

  equal(refusalHandlingNoteOf('标记已处理', { kind: 'unconfigured' }).tone, 'unconfigured');
  const login = refusalHandlingNoteOf('标记已处理', { kind: 'loginRequired' });
  ok(login.tone === 'refused' && login.kind === 'loginRequired');
  const grant = refusalHandlingNoteOf('标记已处理', { kind: 'notGranted' });
  ok(grant.tone === 'refused' && grant.kind === 'notGranted');
  match(grant.text, /403 OPERATOR_NOT_GRANTED/);

  const noAnswer = refusalHandlingNoteOf('标记已处理', { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' });
  match(noAnswer.text, /服务端未形成答案/);
  const transport = refusalHandlingNoteOf('标记已处理', { kind: 'transport', message: 'fetch failed' });
  match(transport.text, /未送达/);
});
