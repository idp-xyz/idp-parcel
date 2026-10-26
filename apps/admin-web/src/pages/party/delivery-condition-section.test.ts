import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { DeliveryConditionRecord } from './api';
import {
  deliveryConditionDeclared,
  deliveryConditionDraftFromRecord,
  deliveryConditionFieldPaths,
  deliveryConditionPayloadOf,
  deliveryConditionRenderedPaths,
  emptyDeliveryConditionDraft,
  methodLinesOf,
} from './delivery-condition-section';

// 钉票 admin-write-faces/25 ① 表单一节的纯逻辑：一格没填整节不声明、填任一格即声明；方式多行文本一行一项、空行不算；合同层
// tightens 永远送（空着也送）、产品层不带 tightens 键；载荷里没有身份也没有摘要；认领路径随方式行数与层长。

test('一格都没填就是不声明；填任一格（含只填合同层 tightens）就是声明', () => {
  const empty = emptyDeliveryConditionDraft();
  equal(deliveryConditionDeclared(empty, 'product'), false);
  equal(deliveryConditionDeclared(empty, 'contract'), false);
  equal(deliveryConditionDeclared({ ...empty, methodsText: '  \n ' }, 'product'), false, '只有空白也不算填');
  equal(deliveryConditionDeclared({ ...empty, proofOfDeliveryRule: 'RULE/proof-1' }, 'product'), true);
  equal(deliveryConditionDeclared({ ...empty, tightensVersion: 'v1' }, 'contract'), true);
  equal(deliveryConditionDeclared({ ...empty, tightensVersion: 'v1' }, 'product'), false, '产品层不看 tightens 两格');
});

test('方式多行文本一行一项：去首尾空白、空行不算项、不去重（重复由服务端答）', () => {
  deepEqual(methodLinesOf('METHOD/in-person\n  METHOD/locker  \n\nMETHOD/in-person\r\n'), [
    'METHOD/in-person',
    'METHOD/locker',
    'METHOD/in-person',
  ]);
  deepEqual(methodLinesOf(''), []);
});

test('产品层载荷三格、不带 tightens 键；合同层 tightens 两格永远在场（空着也送）', () => {
  const draft = {
    ...emptyDeliveryConditionDraft(),
    methodsText: 'METHOD/safe-drop\nMETHOD/in-person',
    recipientScopeRule: ' RULE/recipient-scope-1 ',
    proofOfDeliveryRule: 'RULE/proof-1',
  };
  const product = deliveryConditionPayloadOf(draft, 'product');
  deepEqual(product, {
    methods: ['METHOD/safe-drop', 'METHOD/in-person'],
    recipientScopeRule: 'RULE/recipient-scope-1',
    proofOfDeliveryRule: 'RULE/proof-1',
  });
  equal('tightens' in product, false, '产品层不得带 tightens 键——带了是类别错误，也不该由表单制造');

  const contract = deliveryConditionPayloadOf(draft, 'contract');
  deepEqual(contract.tightens, { objectId: '', version: '' }, '合同层空着也送，让服务端逐格点名');
  const named = deliveryConditionPayloadOf({ ...draft, tightensObjectId: 'product-1', tightensVersion: 'v1' }, 'contract');
  deepEqual(named.tightens, { objectId: 'product-1', version: 'v1' });
  for (const forbidden of ['tenant', 'submitter', 'approver', 'contentDigest', 'approval']) {
    equal(forbidden in named, false, `载荷里不得有 ${forbidden}`);
  }
});

test('认领路径随方式行数与层长；渲染表与认领表一致', () => {
  const root = 'serviceProduct.deliveryConditions';
  const draft = { ...emptyDeliveryConditionDraft(), methodsText: 'METHOD/a\n\nMETHOD/b' };
  deepEqual(deliveryConditionFieldPaths(root, draft, 'product'), [
    root,
    `${root}.recipientScopeRule`,
    `${root}.proofOfDeliveryRule`,
    `${root}.methods[0]`,
    `${root}.methods[1]`,
  ]);
  const contractRoot = 'customerContract.deliveryConditions';
  const contractPaths = deliveryConditionFieldPaths(contractRoot, emptyDeliveryConditionDraft(), 'contract');
  deepEqual(contractPaths, [
    contractRoot,
    `${contractRoot}.recipientScopeRule`,
    `${contractRoot}.proofOfDeliveryRule`,
    `${contractRoot}.tightens`,
    `${contractRoot}.tightens.objectId`,
    `${contractRoot}.tightens.version`,
  ]);
  // 渲染表按 JSX 顺序、认领表按载荷顺序，两表各写各的（票 26 ②），比的是同一个集合，不比顺序。
  const sorted = (paths: string[]) => [...paths].sort();
  deepEqual(sorted(deliveryConditionRenderedPaths(root, draft, 'product')), sorted(deliveryConditionFieldPaths(root, draft, 'product')));
  deepEqual(sorted(deliveryConditionRenderedPaths(contractRoot, draft, 'contract')), sorted(deliveryConditionFieldPaths(contractRoot, draft, 'contract')));
});

// Covers: 票 party-commercial-uiux/14——基于此版本发新版本：目录行上的交付条件还原成这一节的草稿，合同与产品两张表单共用。
// 方式集合还原成一行一项的文本，两条规则引用与合同层所收紧的产品版本照抄，声明时刻不进草稿；键缺席即空草稿（这一版没有交付条件）。
test('目录行的交付条件还原成这一节的草稿：方式一行一项、规则引用与 tightens 照抄、键缺席即空草稿', () => {
  const record: DeliveryConditionRecord = {
    tightens: { objectId: 'SYN-PROD-01', version: 'v2' },
    methods: ['METHOD/safe-drop', 'METHOD/in-person'],
    recipientScopeRule: 'RULE/recipient-scope-1',
    proofOfDeliveryRule: 'RULE/proof-1',
    declaredAt: '2026-01-01T00:00:00Z',
  };
  const restored = deliveryConditionDraftFromRecord(record);
  deepEqual(restored, {
    methodsText: 'METHOD/safe-drop\nMETHOD/in-person',
    recipientScopeRule: 'RULE/recipient-scope-1',
    proofOfDeliveryRule: 'RULE/proof-1',
    tightensObjectId: 'SYN-PROD-01',
    tightensVersion: 'v2',
  });
  // 还原的反向就是组载荷：同一份记录经草稿再组出来，方式的集合与顺序、两条规则引用、tightens 都与记录一致。
  deepEqual(deliveryConditionPayloadOf(restored, 'contract'), {
    methods: record.methods,
    recipientScopeRule: record.recipientScopeRule,
    proofOfDeliveryRule: record.proofOfDeliveryRule,
    tightens: record.tightens,
  });

  // 产品层的记录没有 tightens 键：两格空着；方式为空数组还原成空文本，不凭空造一项。
  const product = deliveryConditionDraftFromRecord({ ...record, tightens: undefined, methods: [] });
  deepEqual(product, { ...emptyDeliveryConditionDraft(), recipientScopeRule: 'RULE/recipient-scope-1', proofOfDeliveryRule: 'RULE/proof-1' });
  deepEqual(deliveryConditionPayloadOf(product, 'product').methods, []);

  deepEqual(deliveryConditionDraftFromRecord(undefined), emptyDeliveryConditionDraft());
  equal(deliveryConditionDeclared(deliveryConditionDraftFromRecord(undefined), 'contract'), false, '空草稿不声明，整节不进载荷');
});
