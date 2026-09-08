import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { payloadKey } from './publication-draft-flow';
import {
  emptyReferenceRow,
  emptyServiceProductDraft,
  isBlankReferenceRow,
  serviceProductFieldPaths,
  serviceProductLocalProblems,
  serviceProductPayloadOf,
  type ServiceProductDraft,
} from './service-product-form';

// 本文件钉的是服务产品版本表单纯逻辑的四条边（票 admin-write-faces/09）：
//   1. 载荷就是壳：组出来的对象没有任何正文格，kind 钉死 SERVICE_PRODUCT；
//   2. 可缺的键缺席而不是空值：上界留空、引用表全空时载荷里没有那两个键；
//   3. 本地只判编码层的问题（同键两行），空字段 / 时刻格式 / 集合外的键一律零命中——那是服务端的话；
//   4. 表单认领的 JSON 路径随引用表的行而变，服务端点名的每一行都有格接住。

function draft(over: Partial<ServiceProductDraft> = {}): ServiceProductDraft {
  return {
    objectId: 'SYN-PROD-CN-SG-EXPRESS',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-08-01T00:00:00Z',
    effectiveEndsAt: '',
    references: [],
    ...over,
  };
}

test('载荷就是壳：只有 kind 与壳各格，没有正文格', () => {
  const payload = serviceProductPayloadOf(draft());
  deepEqual(payload, {
    kind: 'SERVICE_PRODUCT',
    objectId: 'SYN-PROD-CN-SG-EXPRESS',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-08-01T00:00:00Z',
  });
  ok(!('creditPolicy' in payload), '本册载荷不该长出别册的正文格');
  ok(!('effectiveEndsAt' in payload), '上界留空时键缺席，不送空串');
  ok(!('references' in payload), '引用表为空时键缺席，不送空对象');
});

test('上界与引用表填了才进载荷；两格全空的行跳过，半填的行照送', () => {
  const payload = serviceProductPayloadOf(
    draft({
      effectiveEndsAt: '2027-08-01T00:00:00Z',
      references: [
        { kind: 'CUSTOMER_CONTRACT', objectId: 'SYN-CONTRACT-01' },
        emptyReferenceRow(),
        { kind: 'ACCEPTANCE_RULE_PACKAGE', objectId: '' },
      ],
    }),
  );
  equal(payload.effectiveEndsAt, '2027-08-01T00:00:00Z');
  deepEqual(payload.references, { CUSTOMER_CONTRACT: 'SYN-CONTRACT-01', ACCEPTANCE_RULE_PACKAGE: '' });
  ok(isBlankReferenceRow(emptyReferenceRow()));
  ok(!isBlankReferenceRow({ kind: '', objectId: 'x' }), '填了任何一格就不是空行');
});

test('引用键原样送、不代填也不筛：集合外的键与空键都是服务端的话', () => {
  const payload = serviceProductPayloadOf(
    draft({ references: [{ kind: 'NOT_A_KIND', objectId: 'x' }, { kind: '', objectId: 'y' }] }),
  );
  deepEqual(payload.references, { NOT_A_KIND: 'x', '': 'y' });
  deepEqual(serviceProductLocalProblems(draft({ references: [{ kind: 'NOT_A_KIND', objectId: 'x' }] })), {});
});

test('本地只判编码层：空字段与坏时刻零命中，同键两行按路径报出', () => {
  deepEqual(serviceProductLocalProblems(emptyServiceProductDraft()), {});
  deepEqual(serviceProductLocalProblems(draft({ effectiveStartsAt: 'yesterday', objectId: '' })), {});

  const duplicated = draft({
    references: [
      { kind: 'CUSTOMER_CONTRACT', objectId: 'a' },
      { kind: 'PRICE_RULE', objectId: 'p' },
      { kind: 'CUSTOMER_CONTRACT', objectId: 'b' },
    ],
  });
  const problems = serviceProductLocalProblems(duplicated);
  deepEqual(Object.keys(problems), ['references.CUSTOMER_CONTRACT']);
  equal(problems['references.CUSTOMER_CONTRACT']?.length, 1);
  ok(problems['references.CUSTOMER_CONTRACT']?.[0].includes('2 行'));
});

test('认领的 JSON 路径 = 壳五格 + 引用表每一非空行', () => {
  deepEqual(serviceProductFieldPaths(emptyServiceProductDraft()), [
    'objectId',
    'version',
    'scope',
    'effectiveStartsAt',
    'effectiveEndsAt',
  ]);
  const paths = serviceProductFieldPaths(
    draft({ references: [{ kind: 'CUSTOMER_CONTRACT', objectId: 'c' }, emptyReferenceRow(), { kind: '', objectId: 'v' }] }),
  );
  deepEqual(paths.slice(5), ['references.CUSTOMER_CONTRACT', 'references.']);
});

test('载荷键随任一格变：换引用或换上界都是另一份，预览过的不再算数', () => {
  const base = payloadKey(serviceProductPayloadOf(draft()));
  equal(payloadKey(serviceProductPayloadOf(draft())), base);
  ok(payloadKey(serviceProductPayloadOf(draft({ effectiveEndsAt: '2027-01-01T00:00:00Z' }))) !== base);
  ok(payloadKey(serviceProductPayloadOf(draft({ references: [{ kind: 'CUSTOMER_CONTRACT', objectId: 'c' }] }))) !== base);
  // 加一行全空的引用不改载荷：还没填的「加一行」不是内容。
  equal(payloadKey(serviceProductPayloadOf(draft({ references: [emptyReferenceRow()] }))), base);
});
