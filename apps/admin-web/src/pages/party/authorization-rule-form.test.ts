import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import { payloadKey } from './publication-draft-flow';
import {
  authorizationRuleFieldPaths,
  emptyAuthorizationRuleDraft,
  emptyCancellationRowDraft,
  partyOptionsOf,
  payloadOf,
  type AuthorizationRuleDraft,
} from './authorization-rule-form';

// 本文件钉的是授权规则表单草稿 → 载荷那一层纯函数（票 admin-write-faces/17）：
//   1. 正文只有取消授权目录一节，键名 = Go `AuthorizationRuleBodyPayload` 的 json 标签（authorizationRule.cancellationAuthority[{party, rule}]）；
//   2. 行照原样送、**不代判**：没选请求方、空规则、同一请求方两行、零行都原样上送，答回来的是服务端的拒绝；
//   3. 请求方下拉的码只从词表读口来（票 20），403 是可辨的「未配置」显占位，这里没有内置码、没有默认选中。
// 表单不算摘要、不裁任何门（伞票 07 硬句）：这里没有任何领域校验。

function filled(over: Partial<AuthorizationRuleDraft> = {}): AuthorizationRuleDraft {
  return {
    ...emptyAuthorizationRuleDraft(),
    objectId: 'SYN-AUTH-CANCEL-02',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    rows: [
      { party: 'CUSTOMER', rule: 'SYN-RULE-CANCEL-CUSTOMER-01' },
      { party: 'OPERATIONS', rule: 'SYN-RULE-CANCEL-OPS-01' },
    ],
    ...over,
  };
}

test('齐全的草稿组出的载荷键名镜像 Go 侧 json 标签，目录行照原样', () => {
  deepEqual(payloadOf(filled()), {
    kind: 'AUTHORIZATION_RULE',
    objectId: 'SYN-AUTH-CANCEL-02',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    authorizationRule: {
      cancellationAuthority: [
        { party: 'CUSTOMER', rule: 'SYN-RULE-CANCEL-CUSTOMER-01' },
        { party: 'OPERATIONS', rule: 'SYN-RULE-CANCEL-OPS-01' },
      ],
    },
  });
});

test('生效止点留空不进载荷，填了就进；载荷里没有身份也没有摘要', () => {
  const open = payloadOf(filled());
  equal('effectiveEndsAt' in open, false);
  equal(payloadOf(filled({ effectiveEndsAt: '2027-01-01T00:00:00Z' })).effectiveEndsAt, '2027-01-01T00:00:00Z');
  for (const forbidden of ['tenant', 'submitter', 'contentDigest', 'approval', 'references']) {
    equal(forbidden in open, false, `${forbidden} 不该在载荷里`);
  }
});

test('没选请求方、空规则、同一请求方两行、零行都原样上送——表单不代判、不补默认', () => {
  const undecided = payloadOf(filled({ rows: [emptyCancellationRowDraft()] }));
  deepEqual(undecided.authorizationRule?.cancellationAuthority, [{ party: '', rule: '' }]);

  const duplicated = payloadOf(
    filled({ rows: [{ party: 'CUSTOMER', rule: 'A' }, { party: 'CUSTOMER', rule: 'B' }] }),
  );
  equal(duplicated.authorizationRule?.cancellationAuthority.length, 2);

  const none = payloadOf(filled({ rows: [] }));
  deepEqual(none.authorizationRule, { cancellationAuthority: [] });
});

test('空草稿只有一行空行，没有任何一格带默认值', () => {
  const draft = emptyAuthorizationRuleDraft();
  deepEqual(draft.rows, [{ party: '', rule: '' }]);
  for (const [name, value] of Object.entries(draft)) {
    if (name === 'rows') continue;
    equal(value, '', `${name} 不该有默认值`);
  }
});

test('换一行的请求方或规则就是另一份载荷（同键判据与流程状态机一致）', () => {
  const base = payloadKey(payloadOf(filled()));
  equal(payloadKey(payloadOf(filled())), base);
  notEqual(payloadKey(payloadOf(filled({ rows: [{ party: 'CUSTOMER', rule: 'SYN-RULE-CANCEL-CUSTOMER-02' }] }))), base);
});

test('表单认领的路径覆盖壳、目录节与每一行的两格', () => {
  const paths = authorizationRuleFieldPaths(filled());
  for (const expected of [
    'objectId',
    'version',
    'scope',
    'effectiveStartsAt',
    'effectiveEndsAt',
    'authorizationRule',
    'authorizationRule.cancellationAuthority',
    'authorizationRule.cancellationAuthority[0]',
    'authorizationRule.cancellationAuthority[0].party',
    'authorizationRule.cancellationAuthority[0].rule',
    'authorizationRule.cancellationAuthority[1].party',
    'authorizationRule.cancellationAuthority[1].rule',
  ]) {
    ok(paths.includes(expected), `缺 ${expected}`);
  }
  ok(!paths.includes('authorizationRule.cancellationAuthority[2]'), '多认领了不存在的行');
});

test('请求方下拉：读到词表就是服务端码 × 本页中文，顺序照服务端，集外原样', () => {
  const state = partyOptionsOf(
    {
      kind: 'outcome',
      status: 200,
      body: {
        outcome: 'PUBLICATION_VOCABULARY_LISTED',
        kind: 'AUTHORIZATION_RULE',
        sets: [{ name: 'party', codes: ['CUSTOMER', 'OPERATIONS', 'NEW_PARTY'] }],
      },
    },
    { CUSTOMER: '客户', OPERATIONS: '运营' },
  );
  deepEqual(state, {
    kind: 'options',
    options: [
      { value: 'CUSTOMER', label: '客户' },
      { value: 'OPERATIONS', label: '运营' },
      { value: 'NEW_PARTY', label: 'NEW_PARTY' },
    ],
  });
});

test('请求方下拉：未读到是 loading，403 是可辨的未配置，别的失败与没有 party 集都是读不到——三态都没有内置码', () => {
  deepEqual(partyOptionsOf(null, {}), { kind: 'loading' });
  deepEqual(partyOptionsOf({ kind: 'unconfigured' }, {}), { kind: 'unconfigured' });
  deepEqual(partyOptionsOf({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }, {}), { kind: 'unavailable' });
  deepEqual(partyOptionsOf({ kind: 'transport', message: 'down' }, {}), { kind: 'unavailable' });
  deepEqual(
    partyOptionsOf(
      { kind: 'outcome', status: 200, body: { outcome: 'PUBLICATION_VOCABULARY_LISTED', kind: 'AUTHORIZATION_RULE', sets: [] } },
      { CUSTOMER: '客户' },
    ),
    { kind: 'unavailable' },
  );
});
