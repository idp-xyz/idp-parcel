import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  deliveryConditionDeclared,
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
  deepEqual(deliveryConditionRenderedPaths(root, draft, 'product'), deliveryConditionFieldPaths(root, draft, 'product'));
  deepEqual(deliveryConditionRenderedPaths(contractRoot, draft, 'contract'), deliveryConditionFieldPaths(contractRoot, draft, 'contract'));
});
