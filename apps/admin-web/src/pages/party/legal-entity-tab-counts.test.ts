import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import { revisionCountOf, type RevisionChainBody } from './legal-entity-tab-counts';

// 钉详情栏与对象页「资料版本」「登记记录」签上计数的取值（票 company-subject-ux/09）：只有答到了、且答的是所问这家公司，
// 才是修订链的条数；其余一律没有数，签上不显。

function chain(legalEntityId: string, count: number): ApiResult<RevisionChainBody> {
  return { kind: 'outcome', status: 200, body: { legalEntityId, revisions: Array.from({ length: count }, (_, index) => ({ revision: index + 1 })) } };
}

test('答到了且回显的是所问公司：数就是修订链的条数', () => {
  equal(revisionCountOf(chain('SYN-LE-01', 6), 'SYN-LE-01'), 6);
});

test('链上一条都没有是 0，不是没数——「没登记过」是事实，签上照显', () => {
  equal(revisionCountOf(chain('SYN-LE-01', 0), 'SYN-LE-01'), 0);
});

test('还没答：没数', () => {
  equal(revisionCountOf(null, 'SYN-LE-01'), undefined);
});

test('读不到（没到达 / 没形成答案 / 读口墙前）：没数，不拿 0 冒充', () => {
  equal(revisionCountOf({ kind: 'transport', message: '连不上' }, 'SYN-LE-01'), undefined);
  equal(revisionCountOf({ kind: 'noAnswer', status: 502, code: 'UPSTREAM' }, 'SYN-LE-01'), undefined);
  equal(revisionCountOf({ kind: 'notGranted' }, 'SYN-LE-01'), undefined);
  equal(revisionCountOf({ kind: 'unconfigured' }, 'SYN-LE-01'), undefined);
});

test('答的是另一家公司（换行时上一问的慢答案后到）：没数', () => {
  equal(revisionCountOf(chain('SYN-LE-HK-01', 3), 'SYN-LE-01'), undefined);
});
