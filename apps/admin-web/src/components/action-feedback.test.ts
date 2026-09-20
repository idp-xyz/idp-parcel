import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { feedbackFor, pendingText } from './action-feedback';

// 钉的是票 admin-web-workspace-form/05 第 2 条：写动作的三态文案由一处算，动词取调用点已有的按钮文字。
// 期望值取票面原句——成功「已<动词>」、进行中「<动词>中…」、失败用 problem.detail 或传输错原文不改写——不从实现推。

// Covers: 结果尚未到 → 进行中，文案「<动词>中…」；pendingText 与 feedbackFor(null) 是同一句。
test('请求在途映到进行中，文案是动词加「中…」', () => {
  deepEqual(feedbackFor(null, '撤回'), { state: 'pending', text: '撤回中…' });
  equal(pendingText('提交'), '提交中…');
  equal(feedbackFor(null, '提交').text, pendingText('提交'));
});

// Covers: 2xx 形成了答案 → 「已<动词>」；body 里的业务答案不在这一句里，由页面自己渲。
test('形成了答案映到已答，文案是「已」加动词', () => {
  deepEqual(feedbackFor({ kind: 'outcome', status: 201, body: { outcome: 'WITHDRAWN' } }, '撤回'), {
    state: 'answered',
    text: '已撤回',
  });
  // 负向业务答案照样是「已答」：答案不是失败，状态码只说答案有没有形成（ADR-0022）。
  equal(feedbackFor({ kind: 'outcome', status: 200, body: { outcome: 'NOT_AUTHORIZED' } }, '撤回').state, 'answered');
});

// Covers: 4xx 带 detail → 失败文案就是 detail 原文，一个字不改、不加前后缀。
test('调用方问题带 detail 时失败文案是 detail 原文', () => {
  const detail = 'tenantId must not be carried in the payload; it is bound by the access channel';
  deepEqual(feedbackFor({ kind: 'callerProblem', status: 400, code: 'PAYLOAD_SHAPE_REJECTED', detail }, '登记'), {
    state: 'failed',
    text: detail,
  });
});

// Covers: 4xx 没有 detail（边界）→ 退到错误码加状态码，不替服务端补一句散文。
test('调用方问题无 detail 时退到错误码与状态码', () => {
  equal(
    feedbackFor({ kind: 'callerProblem', status: 409, code: 'REQUEST_CONFLICT' }, '提交').text,
    'REQUEST_CONFLICT（HTTP 409）',
  );
  // 只有空白的 detail 视同缺席：一串空格不是理由。
  equal(
    feedbackFor({ kind: 'callerProblem', status: 400, code: 'BAD_INPUT', detail: '   ' }, '提交').text,
    'BAD_INPUT（HTTP 400）',
  );
});

// Covers: 5xx 未形成答案 → 失败，文案是错误码加状态码。
test('服务端未形成答案映到失败，文案是错误码与状态码', () => {
  deepEqual(feedbackFor({ kind: 'noAnswer', status: 503, code: 'DEPENDENCY_UNAVAILABLE' }, '取消'), {
    state: 'failed',
    text: 'DEPENDENCY_UNAVAILABLE（HTTP 503）',
  });
});

// Covers: 传输错 → 失败文案是 message 原文；message 为空（边界）时说「请求未到达 parcel-api」，不给一句空话。
test('传输错的失败文案是原文，空 message 退到未到达', () => {
  equal(feedbackFor({ kind: 'transport', message: 'Failed to fetch' }, '拒绝').text, 'Failed to fetch');
  equal(feedbackFor({ kind: 'transport', message: '' }, '拒绝').text, '请求未到达 parcel-api');
  equal(feedbackFor({ kind: 'transport', message: '  ' }, '拒绝').state, 'failed');
});

// Covers: 接入渠道未配置 → 失败，文案是页面各处已在用的那一句（403 + 码），不另造措辞。
test('接入渠道未配置映到失败，文案带 403 与错误码原名', () => {
  deepEqual(feedbackFor({ kind: 'unconfigured' }, '登记'), {
    state: 'failed',
    text: '接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）',
  });
});
