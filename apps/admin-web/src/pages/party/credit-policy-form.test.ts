import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import {
  creditPolicyFieldPaths,
  creditPolicyLocalProblems,
  creditPolicyPayloadOf,
  emptyCreditPolicyDraft,
  normalizeMoment,
  type CreditPolicyDraft,
} from './credit-policy-form';

// 本文件钉的是信用政策表单纯逻辑的三条边界（票 admin-write-faces/16）：
//   1. 额度两格**都照发**：零金额是「授予零信用」不折成缺席；两格都填、都空也照发，恰一由服务端裁；
//   2. 本地只拦「编不进 JSON 类型」的格（整数格填了非整数），空字段、区间先后一律送上去让服务端答；
//   3. 草稿 → 载荷时可缺的键**缺席而不是空串**，必填的键空着也送（服务端逐格点名，表单只呈现）。

function draft(over: Partial<CreditPolicyDraft> = {}): CreditPolicyDraft {
  return {
    objectId: 'CP-SYN-1',
    version: 'v1',
    scope: 'tenant',
    effectiveStartsAt: '2026-09-01',
    effectiveEndsAt: '',
    legalEntity: 'LE-1',
    authorityLevel: 'L1',
    chargeType: 'FREIGHT',
    limitMinor: '',
    limitRatioBasisPoints: '',
    bodyEffectiveStartsAt: '2026-09-01',
    bodyEffectiveEndsAt: '',
    ...over,
  };
}

// Covers: 零金额进载荷是 0 不是缺席（读面 creditLimitCell 同一判据）；比例格空着即缺席。
test('零金额照发为 0，不折成缺席', () => {
  const payload = creditPolicyPayloadOf(draft({ limitMinor: '0' }));
  equal(payload.creditPolicy?.limitMinor, 0);
  ok(!('limitRatioBasisPoints' in payload.creditPolicy!));
  deepEqual(creditPolicyLocalProblems(draft({ limitMinor: '0' })), {});
});

// Covers: 两格都填照发两格、都空照发零格——恰一由服务端裁，表单不替它挑（票面「选形与理由」）。
test('额度两格都填照发、都空照发，不在本地裁恰一', () => {
  const both = creditPolicyPayloadOf(draft({ limitMinor: '100000', limitRatioBasisPoints: '2500' }));
  equal(both.creditPolicy?.limitMinor, 100000);
  equal(both.creditPolicy?.limitRatioBasisPoints, 2500);
  deepEqual(creditPolicyLocalProblems(draft({ limitMinor: '100000', limitRatioBasisPoints: '2500' })), {});

  const neither = creditPolicyPayloadOf(draft());
  ok(!('limitMinor' in neither.creditPolicy!));
  ok(!('limitRatioBasisPoints' in neither.creditPolicy!));
  deepEqual(creditPolicyLocalProblems(draft()), {});
});

// Covers: 本地只拦编不进 JSON 整数的文本（小数、字母、超出安全整数范围），且拦在那一格自己的路径上；
// 拦住的格不进载荷。
test('整数格填了编不进类型的文本才是本地问题，落在该格路径上', () => {
  const decimal = creditPolicyLocalProblems(draft({ limitMinor: '12.5' }));
  ok(decimal['creditPolicy.limitMinor']?.length === 1);
  ok(!('creditPolicy.limitRatioBasisPoints' in decimal));
  ok(!('limitMinor' in creditPolicyPayloadOf(draft({ limitMinor: '12.5' })).creditPolicy!));

  const letters = creditPolicyLocalProblems(draft({ limitRatioBasisPoints: 'abc' }));
  ok(letters['creditPolicy.limitRatioBasisPoints']?.length === 1);

  const huge = creditPolicyLocalProblems(draft({ limitMinor: '99999999999999999999' }));
  ok(huge['creditPolicy.limitMinor']?.length === 1);

  deepEqual(creditPolicyLocalProblems(draft({ limitMinor: ' -7 ', limitRatioBasisPoints: '+0' })), {});
});

// Covers: 空字段不是本地问题——空草稿零本地问题，载荷里必填键空着照送，服务端逐格点名。
test('空字段不在本地拦：空草稿零本地问题，载荷照送空串', () => {
  deepEqual(creditPolicyLocalProblems(emptyCreditPolicyDraft()), {});
  const payload = creditPolicyPayloadOf(emptyCreditPolicyDraft());
  equal(payload.kind, 'CREDIT_POLICY');
  equal(payload.objectId, '');
  equal(payload.creditPolicy?.legalEntity, '');
  equal(payload.creditPolicy?.effectiveStartsAt, '');
});

// Covers: 可缺的键缺席而不是空串（壳与正文的区间上界），两端空白去掉，日期只到天补成当天零点 UTC。
test('草稿到载荷：可缺键缺席、去空白、日期补成零点 UTC', () => {
  const payload = creditPolicyPayloadOf(
    draft({
      objectId: ' CP-SYN-1 ',
      effectiveStartsAt: ' 2026-09-01 ',
      bodyEffectiveStartsAt: '2026-09-01T08:00:00+08:00',
    }),
  );
  deepEqual(payload, {
    kind: 'CREDIT_POLICY',
    objectId: 'CP-SYN-1',
    version: 'v1',
    scope: 'tenant',
    effectiveStartsAt: '2026-09-01T00:00:00Z',
    creditPolicy: {
      legalEntity: 'LE-1',
      authorityLevel: 'L1',
      chargeType: 'FREIGHT',
      effectiveStartsAt: '2026-09-01T08:00:00+08:00',
    },
  });
  ok(!('effectiveEndsAt' in payload));
  ok(!('references' in payload));

  const bounded = creditPolicyPayloadOf(draft({ effectiveEndsAt: '2026-12-31', bodyEffectiveEndsAt: '2026-12-31' }));
  equal(bounded.effectiveEndsAt, '2026-12-31T00:00:00Z');
  equal(bounded.creditPolicy?.effectiveEndsAt, '2026-12-31T00:00:00Z');
  equal(normalizeMoment(' 2026-08-01 '), '2026-08-01T00:00:00Z');
});

// Covers: 表单认领的路径覆盖载荷能发出的每一条键与服务端会点名的 `creditPolicy.limit`——漏一条，那一格的
// 问题就会掉到「未认领」里而不是挂在格旁。
test('认领路径覆盖载荷全部键与 creditPolicy.limit', () => {
  const claimed = new Set<string>(creditPolicyFieldPaths);
  for (const path of [
    'kind',
    'objectId',
    'version',
    'scope',
    'effectiveStartsAt',
    'effectiveEndsAt',
    'creditPolicy.legalEntity',
    'creditPolicy.authorityLevel',
    'creditPolicy.chargeType',
    'creditPolicy.limit',
    'creditPolicy.limitMinor',
    'creditPolicy.limitRatioBasisPoints',
    'creditPolicy.effectiveStartsAt',
    'creditPolicy.effectiveEndsAt',
  ]) {
    ok(claimed.has(path), path);
  }
});
