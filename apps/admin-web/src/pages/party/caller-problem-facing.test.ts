import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { callerProblemFacing, pointedFieldProblem } from './caller-problem-facing';

const scopeDetail = 'relationships[0].scope: party commercial: blank value: commercial scope reference';

test('点名到覆盖范围：拆出路径和原话', () => {
  deepEqual(pointedFieldProblem(scopeDetail), {
    paths: ['relationships[0].scope'],
    raw: 'party commercial: blank value: commercial scope reference',
  });
});

test('区间两格一条：拆成开始与结束', () => {
  deepEqual(pointedFieldProblem('relationships[0].effectiveStartsAt/effectiveEndsAt: 终点早于起点'), {
    paths: ['relationships[0].effectiveStartsAt', 'relationships[0].effectiveEndsAt'],
    raw: '终点早于起点',
  });
});

test('对不上路径的散文不拆', () => {
  equal(pointedFieldProblem('tenantId must not be carried in the payload'), null);
  equal(pointedFieldProblem('游标与本次的排序或筛选不符，请从第一页重取'), null);
});

test('点名到格子：页脚用格子的名字和译好的一句，原话不放在明面', () => {
  deepEqual(callerProblemFacing('MALFORMED_REQUEST', scopeDetail, () => '请求的格式不对', { 'relationships[0].scope': '覆盖范围' }), {
    lead: '覆盖范围：必填，不能留空',
    reason: null,
    technicalDetail: scopeDetail,
  });
});

test('点名到格子但没有名字：只留译好的一句', () => {
  equal(callerProblemFacing('MALFORMED_REQUEST', scopeDetail, () => '请求的格式不对').lead, '必填，不能留空');
});

test('对不上格子：仍是调用方式问题，散文原样放在原因里', () => {
  const detail = 'tenantId must not be carried in the payload';
  deepEqual(callerProblemFacing('MALFORMED_REQUEST', detail, (code) => `NOTE(${code})`), {
    lead: '调用方式问题：NOTE(MALFORMED_REQUEST)',
    reason: detail,
    technicalDetail: '',
  });
});
