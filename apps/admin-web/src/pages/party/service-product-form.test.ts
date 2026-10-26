import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ServiceProductRecord } from './api';
import { emptyDeliveryConditionDraft } from './delivery-condition-section';
import { payloadKey } from './publication-draft-flow';
import {
  emptyReferenceRow,
  emptyServiceProductDraft,
  isBlankReferenceRow,
  serviceProductDraftFromRecord,
  serviceProductFieldPaths,
  serviceProductLocalProblems,
  serviceProductPayloadOf,
  type ServiceProductDraft,
} from './service-product-form';

// 本文件钉的是服务产品版本表单纯逻辑的四条边（票 admin-write-faces/09）：
//   1. 载荷是壳：不声明交付条件时组出来的对象没有任何正文格，kind 钉死 SERVICE_PRODUCT；
//   2. 可缺的键缺席而不是空值：上界留空、引用表全空时载荷里没有那两个键；
//   3. 本地只判编码层的问题（同键两行、时刻格换不成时刻），空字段 / 集合外的键一律零命中——那是服务端的话；
//   4. 表单认领的 JSON 路径随引用表的行而变，服务端点名的每一行都有格接住。
// 第五条自票 admin-write-faces/25 起：产品层交付条件一节填了才带 `serviceProduct.deliveryConditions`，不带 tightens 键。

function draft(over: Partial<ServiceProductDraft> = {}): ServiceProductDraft {
  return {
    objectId: 'SYN-PROD-CN-SG-EXPRESS',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-08-01T00:00',
    effectiveEndsAt: '',
    references: [],
    deliveryConditions: emptyDeliveryConditionDraft(),
    ...over,
  };
}

test('载荷是壳：不声明交付条件时只有 kind 与壳各格，没有正文格', () => {
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
  ok(!('serviceProduct' in payload), '交付条件一格都没填时本册正文格缺席，载荷仍是壳');
});

test('交付条件填了任一格整节进 serviceProduct.deliveryConditions；产品层不带 tightens 键', () => {
  const declared = serviceProductPayloadOf(
    draft({
      deliveryConditions: {
        ...emptyDeliveryConditionDraft(),
        methodsText: 'METHOD/safe-drop\nMETHOD/in-person\n',
        recipientScopeRule: 'RULE/recipient-scope-1',
        proofOfDeliveryRule: 'RULE/proof-1',
      },
    }),
  );
  deepEqual(declared.serviceProduct, {
    deliveryConditions: {
      methods: ['METHOD/safe-drop', 'METHOD/in-person'],
      recipientScopeRule: 'RULE/recipient-scope-1',
      proofOfDeliveryRule: 'RULE/proof-1',
    },
  });
  // 只填一格也算声明：空格照送，由服务端点名——表单不替操作者判「这算不算填了」。
  const partial = serviceProductPayloadOf(draft({ deliveryConditions: { ...emptyDeliveryConditionDraft(), proofOfDeliveryRule: 'RULE/proof-1' } }));
  deepEqual(partial.serviceProduct?.deliveryConditions, { methods: [], recipientScopeRule: '', proofOfDeliveryRule: 'RULE/proof-1' });
});

test('上界与引用表填了才进载荷；两格全空的行跳过，半填的行照送', () => {
  const payload = serviceProductPayloadOf(
    draft({
      effectiveEndsAt: '2027-08-01T00:00',
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

// Covers: 票 party-commercial-uiux/10——时刻格是日期时间控件交出的本地墙钟，按显示时区换成 UTC；只有空白的止点同空着。
test('时刻格的本地墙钟按显示时区换成 UTC', () => {
  const payload = serviceProductPayloadOf(draft({ effectiveStartsAt: ' 2026-08-01T08:00 ', effectiveEndsAt: '2027-08-01T00:00:30' }), 'Asia/Shanghai');
  equal(payload.effectiveStartsAt, '2026-08-01T00:00:00Z');
  equal(payload.effectiveEndsAt, '2027-07-31T16:00:30Z');
  ok(!('effectiveEndsAt' in serviceProductPayloadOf(draft({ effectiveEndsAt: '  ' }), 'Asia/Shanghai')));
});

// Covers: 票 party-commercial-uiux/10——时刻格换不成时刻是本地问题，落在那一格路径上，挡住预览；空着不是。
test('时刻格换不成时刻才是本地问题，落在该格路径上', () => {
  deepEqual(serviceProductLocalProblems(draft({ effectiveStartsAt: 'yesterday', effectiveEndsAt: '2027-08-01' }), 'Asia/Shanghai'), {
    effectiveStartsAt: ['时间格式不对，请用日期时间选择器重新选'],
    effectiveEndsAt: ['时间格式不对，请用日期时间选择器重新选'],
  });
  deepEqual(serviceProductLocalProblems(draft(), 'Asia/Shanghai'), {});
  deepEqual(serviceProductLocalProblems(emptyServiceProductDraft(), 'Asia/Shanghai'), {});
});

test('本地只判编码层：空字段零命中，同键两行按路径报出', () => {
  deepEqual(serviceProductLocalProblems(emptyServiceProductDraft()), {});
  deepEqual(serviceProductLocalProblems(draft({ objectId: '' })), {});

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

test('认领的 JSON 路径 = 壳五格 + 引用表每一非空行 + 交付条件一节', () => {
  const sectionPaths = [
    'serviceProduct.deliveryConditions',
    'serviceProduct.deliveryConditions.recipientScopeRule',
    'serviceProduct.deliveryConditions.proofOfDeliveryRule',
  ];
  deepEqual(serviceProductFieldPaths(emptyServiceProductDraft()), [
    'objectId',
    'version',
    'scope',
    'effectiveStartsAt',
    'effectiveEndsAt',
    ...sectionPaths,
  ]);
  const paths = serviceProductFieldPaths(
    draft({ references: [{ kind: 'CUSTOMER_CONTRACT', objectId: 'c' }, emptyReferenceRow(), { kind: '', objectId: 'v' }] }),
  );
  deepEqual(paths.slice(5), ['references.CUSTOMER_CONTRACT', 'references.', ...sectionPaths]);
  const withMethods = serviceProductFieldPaths(draft({ deliveryConditions: { ...emptyDeliveryConditionDraft(), methodsText: 'a\nb' } }));
  deepEqual(withMethods.slice(-2), ['serviceProduct.deliveryConditions.methods[0]', 'serviceProduct.deliveryConditions.methods[1]']);
});

test('载荷键随任一格变：换引用或换上界都是另一份，预览过的不再算数', () => {
  const base = payloadKey(serviceProductPayloadOf(draft()));
  equal(payloadKey(serviceProductPayloadOf(draft())), base);
  ok(payloadKey(serviceProductPayloadOf(draft({ effectiveEndsAt: '2027-01-01T00:00' }))) !== base);
  ok(payloadKey(serviceProductPayloadOf(draft({ references: [{ kind: 'CUSTOMER_CONTRACT', objectId: 'c' }] }))) !== base);
  // 加一行全空的引用不改载荷：还没填的「加一行」不是内容。
  equal(payloadKey(serviceProductPayloadOf(draft({ references: [emptyReferenceRow()] }))), base);
});

// 目录里一版服务产品的读面行：必填字段写全，可缺的键由各用例按需加。
function productRecord(over: Partial<ServiceProductRecord> = {}): ServiceProductRecord {
  return {
    objectId: 'SYN-PROD-CN-SG-EXPRESS',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    status: 'PUBLISHED',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    effectiveEndsAt: '2026-12-31T00:00:00Z',
    publishedAt: '2026-01-01T00:00:00Z',
    form: 'EXPRESS',
    deliveryConditions: {
      methods: ['METHOD/safe-drop', 'METHOD/in-person'],
      recipientScopeRule: 'RULE/recipient-scope-1',
      proofOfDeliveryRule: 'RULE/proof-1',
      declaredAt: '2026-01-01T00:00:00Z',
    },
    ...over,
  };
}

// Covers: 票 party-commercial-uiux/14——基于此版本发新版本：对象标识、版本适用范围与产品层交付条件照抄；版本号与有效区间留空
// （抄上一版的区间会让新旧两版重叠，解析时答「适用冲突」）；指名引用表读面没有，留空；服务形态不在本册载荷里，不抄。
test('基于此版本预填：抄对象、适用范围与交付条件，留版本号、区间与引用表', () => {
  deepEqual(serviceProductDraftFromRecord(productRecord()), {
    ...emptyServiceProductDraft(),
    objectId: 'SYN-PROD-CN-SG-EXPRESS',
    scope: 'SYN-SCOPE-01',
    deliveryConditions: {
      methodsText: 'METHOD/safe-drop\nMETHOD/in-person',
      recipientScopeRule: 'RULE/recipient-scope-1',
      proofOfDeliveryRule: 'RULE/proof-1',
      tightensObjectId: '',
      tightensVersion: '',
    },
  });
});

// Covers: 票 party-commercial-uiux/14——还原是组载荷的反向：人补上版本号与起点后，载荷里的交付条件与记录所读一致，产品层
// 不带 tightens 键，上一版的止点没有被带过来。
test('基于此版本预填：人补齐留空的格后，载荷里的交付条件与记录一致', () => {
  const payload = serviceProductPayloadOf(
    { ...serviceProductDraftFromRecord(productRecord()), version: 'v3', effectiveStartsAt: '2027-01-01T00:00' },
    'Asia/Shanghai',
  );
  deepEqual(payload, {
    kind: 'SERVICE_PRODUCT',
    objectId: 'SYN-PROD-CN-SG-EXPRESS',
    version: 'v3',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-12-31T16:00:00Z',
    serviceProduct: {
      deliveryConditions: {
        methods: ['METHOD/safe-drop', 'METHOD/in-person'],
        recipientScopeRule: 'RULE/recipient-scope-1',
        proofOfDeliveryRule: 'RULE/proof-1',
      },
    },
  });
});

// Covers: 票 party-commercial-uiux/14——读面上可缺的键缺席时不崩也不硬塞：交付条件键缺席即这一版没有，草稿是空的一节、
// 载荷仍是壳；服务形态与止点缺席同样不影响；声明了但方式为空数组的一节照样还原（空方式集合不折成缺席）。
test('基于此版本预填：可缺的键缺席时不崩也不硬塞', () => {
  const bare = serviceProductDraftFromRecord(
    productRecord({ deliveryConditions: undefined, form: undefined, effectiveEndsAt: undefined }),
  );
  deepEqual(bare, { ...emptyServiceProductDraft(), objectId: 'SYN-PROD-CN-SG-EXPRESS', scope: 'SYN-SCOPE-01' });
  ok(!('serviceProduct' in serviceProductPayloadOf(bare)), '没有交付条件就不带本册正文格');

  const noMethods = serviceProductDraftFromRecord(
    productRecord({
      deliveryConditions: { methods: [], recipientScopeRule: 'RULE/r', proofOfDeliveryRule: 'RULE/p', declaredAt: '2026-01-01T00:00:00Z' },
    }),
  );
  equal(noMethods.deliveryConditions.methodsText, '');
  equal(noMethods.deliveryConditions.recipientScopeRule, 'RULE/r');
  deepEqual(serviceProductPayloadOf(noMethods).serviceProduct?.deliveryConditions, {
    methods: [],
    recipientScopeRule: 'RULE/r',
    proofOfDeliveryRule: 'RULE/p',
  });
});
