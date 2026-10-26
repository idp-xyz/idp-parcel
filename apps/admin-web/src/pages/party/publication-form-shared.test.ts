import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { wallMomentOf, wallMomentProblem } from './publication-form-shared';

// 票 party-commercial-uiux/10：发布表单的时刻按操作者显示时区的墙钟填，送出 UTC。
test('本地墙钟按显示时区换成 UTC：上海当天零点是前一天 16 点 UTC', () => {
  equal(wallMomentOf('2026-10-01T00:00', 'Asia/Shanghai'), '2026-09-30T16:00:00Z');
  equal(wallMomentOf('2026-10-01T09:30:15', 'Asia/Singapore'), '2026-10-01T01:30:15Z');
  equal(wallMomentOf('2026-10-01T00:00', 'UTC'), '2026-10-01T00:00:00Z');
});

test('留空交空串，由服务端点名必填或由调用方不带选填键', () => {
  equal(wallMomentOf('', 'Asia/Shanghai'), '');
  equal(wallMomentOf('   ', 'Asia/Shanghai'), '');
  equal(wallMomentProblem('', 'Asia/Shanghai'), null);
});

test('换不出的文本原样交出，并在本地点名挡住预览', () => {
  equal(wallMomentOf('2026-13-01T00:00', 'Asia/Shanghai'), '2026-13-01T00:00');
  equal(wallMomentProblem('2026-13-01T00:00', 'Asia/Shanghai'), '时间格式不对，请用日期时间选择器重新选');
  equal(wallMomentProblem('2026-10-01', 'Asia/Shanghai'), '时间格式不对，请用日期时间选择器重新选');
  equal(wallMomentProblem('2026-10-01T08:00', 'Asia/Shanghai'), null);
});
