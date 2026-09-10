import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import {
  dispositionChoiceOptions,
  dispositionCommandNoteOf,
  dispositionQueueRowOf,
  dispositionQueueViewState,
  restrictedItemText,
} from './authorized-disposition';
import type { AuthorizedDispositionQueueEntry } from './api';

// 本包 @types/node 的 assert/strict 声明里没有 doesNotMatch;反向断言照本目录既有写法用 ok(!re.test())。
function doesNotMatch(value: string, pattern: RegExp): void {
  ok(!pattern.test(value), `不该匹配 ${pattern} 却匹配了:${value}`);
}

// 本文件钉授权处置页的三态与决定集(票 sa-preacceptance-policy-view/05;ADR-0132 决定一、二)。
// 期望值取服务端响应的原词与 ADR 原句,不从实现推。

// 不叫 module:测试编成 CommonJS 后 module 是模块对象,同名常量会在加载期撞成语法错。
const moduleInfo = {
  title: '授权处置',
  owner: '小包托运(parcel-shipment)',
  source: 'docs/domain/parcel-shipment/CONTEXT.md「授权处置」',
};

function entryOf(overrides: Partial<AuthorizedDispositionQueueEntry> = {}): AuthorizedDispositionQueueEntry {
  return {
    shipmentRequestId: 'REQ-1',
    customerAccountId: 'CUST-1',
    source: 'source-a',
    sourceRequestKey: 'key-1',
    state: 'SUBMITTED',
    submissionVersionId: 'REQ-1/v1',
    declaredParcelCount: 2,
    submittedAt: '2026-09-01T08:00:00Z',
    controlResultId: 'FCR-1',
    restrictedItems: [
      {
        kind: 'CREDIT_CHECK',
        order: 2,
        basis: 'CREDIT_LIMIT_EXCEEDED',
        failureDisposition: 'AUTHORIZED_DISPOSITION',
        responsibility: 'PARTY-FIN-1',
      },
    ],
    ...overrides,
  };
}

// Covers: ADR-0132 决定一——去向封闭两值,「放行」不在集内;决定集里没有任何一格能被读成通过。
test('决定集恰是拒绝与交客户补充两去向,没有通过 / 放行 / 批准', () => {
  deepEqual(
    dispositionChoiceOptions.map((option) => option.id),
    ['REJECT', 'CUSTOMER_SUPPLEMENT'],
  );
  for (const option of dispositionChoiceOptions) {
    doesNotMatch(option.label, /通过|放行|批准/);
  }
  equal(dispositionChoiceOptions.find((option) => option.id === 'REJECT')?.label, '拒绝');
  equal(
    dispositionChoiceOptions.find((option) => option.id === 'CUSTOMER_SUPPLEMENT')?.label,
    '交客户补充',
  );
});

// Covers: 三态之一「空」——服务端作答且 entries 为空数组才是空队列,措辞说「空队列是答案」,不说缺陷。
test('空队列是答案不是缺陷', () => {
  const state = dispositionQueueViewState(
    { kind: 'outcome', status: 200, body: { outcome: 'DISPOSITION_QUEUE_LISTED', entries: [] } },
    () => {},
    moduleInfo,
  );
  ok(state.kind === 'empty');
  match(state.title ?? '', /等待授权处置/);
  match(state.description ?? '', /空队列是答案/);
  match(state.description ?? '', /不是缺陷/);
});

// Covers: 三态之二「有项」——有行即 ready;队列行显委托标识、客户账户、包裹数与受限项数,
// 受限项文案逐字带服务端原词(种类、受限原因、失败处置、责任引用),不自造译法。
test('有项时队列行与受限项文案带服务端原词', () => {
  const entry = entryOf();
  const state = dispositionQueueViewState(
    { kind: 'outcome', status: 200, body: { outcome: 'DISPOSITION_QUEUE_LISTED', entries: [entry] } },
    () => {},
    moduleInfo,
  );
  equal(state.kind, 'ready');

  const row = dispositionQueueRowOf(entry);
  equal(row.id, 'REQ-1');
  equal(row.title, 'REQ-1');
  match(row.subtitle, /CUST-1/);
  match(row.subtitle, /2 件/);
  equal(row.statusText, '1 项受限');
  equal(row.badData, false);

  const line = restrictedItemText(entry.restrictedItems[0]);
  match(line, /CREDIT_CHECK/);
  match(line, /第 2 项/);
  match(line, /CREDIT_LIMIT_EXCEEDED/);
  match(line, /AUTHORIZED_DISPOSITION/);
  match(line, /进入授权处置/);
  match(line, /PARTY-FIN-1/);
});

// Covers: 受限项上两格采用引用缺席时不补「无」——没采用过就是没采用过,行里不出现这两段。
test('采用引用缺席时受限项文案不写无', () => {
  const line = restrictedItemText({ kind: 'PREPAID_FREEZE', order: 1 });
  match(line, /PREPAID_FREEZE/);
  doesNotMatch(line, /失败处置/);
  doesNotMatch(line, /责任/);
  doesNotMatch(line, /无/);
});

// Covers: restrictedItems 为空数组是坏数据的可观察征兆(读面头注),队列行标出来而不是当成没有受限项。
test('受限项为空数组的行标为坏数据征兆', () => {
  const row = dispositionQueueRowOf(entryOf({ restrictedItems: [] }));
  equal(row.badData, true);
  match(row.statusText, /受限项为空/);
});

// Covers: 三态之三「处置停点」——写面挂 UnconfiguredIntake(ADR-0085 / ADR-0055),403 译成未配置停点:
// 原词直显、点名 PAR-INT-01、不退化成「稍后重试」、不造采信身份。
test('未配置档的处置停点原词直显,不说稍后重试', () => {
  const note = dispositionCommandNoteOf('拒绝', { kind: 'unconfigured' });
  equal(note.tone, 'unconfigured');
  match(note.text, /403 ACCESS_CHANNEL_NOT_CONFIGURED/);
  match(note.text, /PAR-INT-01/);
  match(note.text, /「拒绝」/);
  doesNotMatch(note.text, /稍后重试/);
  doesNotMatch(note.text, /通过|放行/);
});

// Covers: 命令成立时按 outcome 词表呈现,并把去向原词与中文并列;补偿续办引用有才显。
test('已记录的处置带去向原词与之后的委托状态', () => {
  const note = dispositionCommandNoteOf('交客户补充', {
    kind: 'outcome',
    status: 201,
    body: {
      outcome: 'RECORDED',
      requestState: 'SUBMITTED',
      dispositionChoice: 'CUSTOMER_SUPPLEMENT',
    },
  });
  equal(note.tone, 'outcome');
  match(note.text, /已记录\(RECORDED\)/);
  match(note.text, /交客户补充\(CUSTOMER_SUPPLEMENT\)/);
  match(note.text, /SUBMITTED/);
  doesNotMatch(note.text, /补偿/);
});

// Covers: 未获授权是确定的业务答案(不是未决);授权规则未登记是未配置态,措辞指向登记参数而不是重试。
test('未获授权与授权规则未登记各说各的恢复动作', () => {
  const refused = dispositionCommandNoteOf('拒绝', {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'NOT_AUTHORIZED' },
  });
  match(refused.text, /未获授权\(NOT_AUTHORIZED\)/);
  match(refused.text, /不是未决/);

  const unregistered = dispositionCommandNoteOf('拒绝', {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'AUTHORITY_RULES_NOT_CONFIGURED' },
  });
  match(unregistered.text, /授权规则未登记\(AUTHORITY_RULES_NOT_CONFIGURED\)/);
  match(unregistered.text, /PAR-COM-14/);
  match(unregistered.text, /登记/);
});

// Covers: 任务已完结带既有决定,版本已换代带当前版本——处置人要知道输给了什么、该按哪一版重读。
test('任务已完结与版本已换代带出各自的凭据', () => {
  const closed = dispositionCommandNoteOf('拒绝', {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'TASK_ALREADY_CLOSED', decisionKind: 'ACCEPTED', requestState: 'ACCEPTED' },
  });
  match(closed.text, /任务已完结\(TASK_ALREADY_CLOSED\)/);
  match(closed.text, /ACCEPTED/);

  const superseded = dispositionCommandNoteOf('拒绝', {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'VERSION_SUPERSEDED', currentVersion: 'REQ-1/v2' },
  });
  match(superseded.text, /版本已换代\(VERSION_SUPERSEDED\)/);
  match(superseded.text, /REQ-1\/v2/);
});

// Covers: 其余 4xx / 5xx / 传输层各自分格,不与业务答案混;5xx 说服务端未形成答案。
test('调用方问题、服务端未形成答案与未送达分三格', () => {
  const caller = dispositionCommandNoteOf('拒绝', { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
  equal(caller.tone, 'problem');
  match(caller.text, /HTTP 400/);
  match(caller.text, /MALFORMED_REQUEST/);

  const noAnswer = dispositionCommandNoteOf('拒绝', { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' });
  equal(noAnswer.tone, 'problem');
  match(noAnswer.text, /未形成答案/);

  const transport = dispositionCommandNoteOf('拒绝', { kind: 'transport', message: 'Failed to fetch' });
  equal(transport.tone, 'problem');
  match(transport.text, /Failed to fetch/);
});
