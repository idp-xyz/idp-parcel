import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { problemCodeOf, problemDetailOf } from './problem-body';

// 两种问题体并存（ADR-0140 决定三）：首方族与 PS 客户面命令口是 RFC 9457 问题详情，`code`/`detail` 在顶层；操作者族各口仍是
// {"error":{"code","detail"}}。读错一边，页面就把一个有名字的拒绝显示成「未知错误码」。
test('读得出问题详情顶层的 code 与 detail', () => {
  const body = {
    type: 'urn:idp-parcel:problem:MALFORMED_REQUEST',
    title: 'Malformed request',
    status: 400,
    code: 'MALFORMED_REQUEST',
    detail: 'parcels[0]: missing',
    requestId: 'r-1',
  };
  equal(problemCodeOf(body), 'MALFORMED_REQUEST');
  equal(problemDetailOf(body), 'parcels[0]: missing');
});

test('读得出旧形状 error 成员里的 code 与 detail', () => {
  const body = { error: { code: 'OPERATOR_NOT_GRANTED', detail: 'x' } };
  equal(problemCodeOf(body), 'OPERATOR_NOT_GRANTED');
  equal(problemDetailOf(body), 'x');
});

test('认不出的形状答 UNKNOWN，detail 不冒充', () => {
  equal(problemCodeOf(null), 'UNKNOWN');
  equal(problemCodeOf({ code: 7 }), 'UNKNOWN');
  equal(problemDetailOf({ code: 'X', detail: { nested: true } }), undefined);
});
