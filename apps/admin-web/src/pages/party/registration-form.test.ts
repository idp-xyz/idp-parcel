import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';
import { effectiveDraftOf, registrationLanded, revisionOf, suggestedNextRevision } from './registration-form';

// 本文件钉的是参与方模块各登记表单共用的规则（票 admin-web-group-legal-entities/13 第 1 / 3 条抬出）：修订号格的编码
// 层判据、建议值何时顶进草稿、登记册答什么才算落地。各份表单（法人 / 参与方 / 关系 / 停用……）同一判，不各留一份。

// Covers: 修订号只收正整数原文——零、负数、小数、空、非数字都编不出，交回 undefined 由各表单的问题表拦住不送。
test('修订号只编正整数', () => {
  equal(revisionOf('1'), 1);
  equal(revisionOf('12'), 12);
  equal(revisionOf('0'), undefined);
  equal(revisionOf('-1'), undefined);
  equal(revisionOf('1.5'), undefined);
  equal(revisionOf(''), undefined);
  equal(revisionOf('x'), undefined);
});

// Covers: 修订号周围的空白不构成第二个值——它编成的是数，" 3 " 与 "3" 是同一个修订号；这是身份串「不裁空白」判据
// 的唯一例外，理由在 business-party-form.ts 文件头。
test('修订号允许首尾空白', () => {
  equal(revisionOf(' 3 '), 3);
  equal(revisionOf('\t7\n'), 7);
  equal(revisionOf('   '), undefined);
});

// Covers: 「最新修订加一」只此一份（票 14 第 2 条）：标识在已取回的册里 → 该行 revision + 1；不在册 / 册没取到（null）/
// 标识空 → 1；空数组是「册上确实没有」，也是 1。键按 keyOf 从行里取、与 id 逐字比——裁不裁空白是各册包装的事，共用体不替
// 它裁，所以 "A " 找不到 "A"；行的形状只要求有 revision，取键字段各册自定。
test('建议修订号取该标识最新修订加一，键由调用方取', () => {
  interface Row {
    code: string;
    revision: number;
  }
  const rows: Row[] = [
    { code: 'A', revision: 3 },
    { code: 'B', revision: 7 },
  ];
  const codeOf = (row: Row) => row.code;
  equal(suggestedNextRevision(rows, 'A', codeOf), 4);
  equal(suggestedNextRevision(rows, 'B', codeOf), 8);
  equal(suggestedNextRevision(rows, 'A ', codeOf), 1);
  equal(suggestedNextRevision(rows, 'C', codeOf), 1);
  equal(suggestedNextRevision(rows, '', codeOf), 1);
  equal(suggestedNextRevision([], 'A', codeOf), 1);
  equal(suggestedNextRevision(null, 'A', codeOf), 1);
});

// Covers: 操作者没改过修订号时建议值顶进草稿（其余格不动）；改过就用草稿原值，建议再变也不覆盖——「用建议值」复位
// 之前建议只显不占格。建议只是省一次翻册，连续性仍由服务端判，所以顶进去的也只是个待送的字符串。
test('建议值只在未手改时顶进草稿', () => {
  const draft = { partyId: 'SYN-PARTY-01', revision: '', basis: 'b' };
  deepEqual(effectiveDraftOf(draft, false, 4), { partyId: 'SYN-PARTY-01', revision: '4', basis: 'b' });
  deepEqual(effectiveDraftOf({ ...draft, revision: '9' }, true, 4), { partyId: 'SYN-PARTY-01', revision: '9', basis: 'b' });
  deepEqual(effectiveDraftOf({ ...draft, revision: '9' }, false, 4).revision, '4');
});

// Covers: 只有登记册答这一口的「已落册」线上名才算落地、才触发读签重取；答别的（已在册 / 冲突 / 未受理）都没写进去，
// 未配置 / 调用方问题 / 未形成答案 / 没到达更没有——重取只会让人以为写进去了。
test('登记册答落册线上名才算落地', () => {
  const answered = (outcome: string): ApiResult<RegistrationResponseBody> => ({
    kind: 'outcome',
    status: 201,
    body: { outcome },
  });
  equal(registrationLanded(answered('REGISTERED'), 'REGISTERED'), true);
  equal(registrationLanded(answered('DEACTIVATED'), 'DEACTIVATED'), true);
  equal(registrationLanded(answered('REGISTERED'), 'DEACTIVATED'), false);
  equal(registrationLanded(answered('NOT_ACCEPTED'), 'REGISTERED'), false);
  equal(registrationLanded({ kind: 'unconfigured' }, 'REGISTERED'), false);
  equal(registrationLanded({ kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' }, 'REGISTERED'), false);
  equal(registrationLanded({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }, 'REGISTERED'), false);
  equal(registrationLanded({ kind: 'transport', message: 'ECONNREFUSED' }, 'REGISTERED'), false);
});
