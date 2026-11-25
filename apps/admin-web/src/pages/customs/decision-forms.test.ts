import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { buildGuidedPayload, initialGuidedValues } from '../../components/guided-form/guided-forms';
import { customsDecisionEndpoints, type CustomsDecisionKind } from './decisions';
import { customsDecisionForm, customsDecisionPrefill } from './decision-forms';

test('每个清关运营决定口都有逐格表单，空表交不出去', () => {
  for (const kind of Object.keys(customsDecisionEndpoints) as CustomsDecisionKind[]) {
    const definition = customsDecisionForm(kind);
    equal(definition.fields.length > 0, true, kind);
    const built = buildGuidedPayload(definition, initialGuidedValues(definition.fields));
    equal(built.ok, false, kind);
  }
});

test('建案把包裹号拆成数组、角色拆成行', () => {
  const definition = customsDecisionForm('case-establishment');
  const built = buildGuidedPayload(definition, {
    ...initialGuidedValues(definition.fields),
    jurisdiction: 'CN',
    direction: 'EXPORT',
    procedure: 'PROC',
    obligation: 'OBL',
    parcels: 'P1\nP2',
    roles: [{ role: 'DECLARANT', party: 'PARTY', authority: 'AUTH' }],
  });
  equal(built.ok, true);
  if (!built.ok) return;
  deepEqual(built.payload.parcels, ['P1', 'P2']);
  deepEqual(built.payload.roles, [{ role: 'DECLARANT', party: 'PARTY', authority: 'AUTH' }]);
});

test('不是放行结果时不带 release；附条件放行缺条件交不出去', () => {
  const definition = customsDecisionForm('external-result-entry');
  const base = {
    ...initialGuidedValues(definition.fields),
    sourceId: 'S',
    layer: 'REGULATORY_RECEIPT',
    role: 'CUSTOMS',
    rawSemantics: '原文',
    claimedVersion: 'V',
    attempt: '1',
    scope: 'UNIT',
    occurredAt: '2026-11-13T01:00:00Z',
    original: 'O',
  };
  const receipt = buildGuidedPayload(definition, base);
  equal(receipt.ok, true);
  if (receipt.ok) equal(receipt.payload.release, undefined);

  const conditional = buildGuidedPayload(definition, {
    ...base,
    layer: 'RELEASE_RESULT',
    'release.kind': 'CONDITIONAL',
    'release.authority': 'AUTH',
  });
  equal(conditional.ok, false);
});

test('结果层不是放行结果却填了放行内容：交不出去，不替人把放行内容去掉', () => {
  const definition = customsDecisionForm('external-result-entry');
  const built = buildGuidedPayload(definition, {
    ...initialGuidedValues(definition.fields),
    sourceId: 'S',
    layer: 'BUSINESS_ACCEPTANCE',
    role: 'CUSTOMS',
    rawSemantics: '原文',
    claimedVersion: 'V',
    attempt: '1',
    scope: 'UNIT',
    occurredAt: '2026-11-13T01:00:00Z',
    original: 'O',
    'release.kind': 'FULL',
    'release.authority': 'AUTH',
  });
  equal(built.ok, false);
  if (!built.ok) equal(built.formError?.includes('结果层不是放行结果'), true);
});

test('完整放行带条件交不出去；时刻缺秒或缺时区交不出去', () => {
  const definition = customsDecisionForm('external-result-entry');
  const base = {
    ...initialGuidedValues(definition.fields),
    sourceId: 'S',
    layer: 'RELEASE_RESULT',
    role: 'CUSTOMS',
    rawSemantics: '原文',
    claimedVersion: 'V',
    attempt: '1',
    scope: 'UNIT',
    occurredAt: '2026-11-13T01:00:00Z',
    original: 'O',
    'release.kind': 'FULL',
    'release.authority': 'AUTH',
  };
  equal(buildGuidedPayload(definition, base).ok, true);
  equal(buildGuidedPayload(definition, { ...base, 'release.condition': '补税后放行' }).ok, false);
  for (const occurredAt of ['2026-11-13T01:00:00', '2026-11-13T01:00Z', '2026-11-13 01:00:00Z']) {
    const built = buildGuidedPayload(definition, { ...base, occurredAt });
    equal(built.ok, false, occurredAt);
  }
  equal(buildGuidedPayload(definition, { ...base, occurredAt: '2026-11-13T09:00:00+08:00' }).ok, true);
});

test('限制哪几步是多选，交出去的是封闭词的数组', () => {
  const definition = customsDecisionForm('regulatory-restriction');
  const base = { ...initialGuidedValues(definition.fields), restrictionId: 'R', decision: 'SRC' };
  equal(buildGuidedPayload(definition, base).ok, false);
  const built = buildGuidedPayload(definition, { ...base, constrains: ['FINAL_DELIVERY', 'OUTBOUND_RELEASE'] });
  equal(built.ok, true);
  if (built.ok) deepEqual(built.payload.constrains, ['OUTBOUND_RELEASE', 'FINAL_DELIVERY']);
});

test('列表行按口带标识：清关单行带进提交申报与关闭', () => {
  deepEqual(customsDecisionPrefill({ key: 'case:1', values: { id: 'CASE-1' } }), {
    'declaration-submission': { caseId: 'CASE-1' },
    'case-closure': { caseId: 'CASE-1' },
  });
});

test('报关批次行把批次带成放行检查与海关结果的申报范围，监管程序带成边界', () => {
  const prefill = customsDecisionPrefill({ key: 'unit:U', values: { id: 'UNIT-1', caseId: 'CASE-1', procedure: 'PROC' } });
  deepEqual(prefill['declaration-submission'], { unitId: 'UNIT-1', caseId: 'CASE-1', procedure: 'PROC' });
  deepEqual(prefill['release-gate-verification'], { scope: 'UNIT-1', boundary: 'PROC' });
  deepEqual(prefill['external-result-entry'], { scope: 'UNIT-1' });
});

test('舱单行只带舱单号与承运商进新版本口，版本与范围要人填', () => {
  deepEqual(
    customsDecisionPrefill({ key: 'manifest:CX:M1', values: { manifest: 'M1', carrier: 'CX', version: 'v1', scope: 'S' } }),
    { 'manifest-revision-entry': { manifestId: 'M1', carrier: 'CX' } },
  );
});

test('海关结果行不带任何格：带进更正对象会把新结果交成更正', () => {
  deepEqual(customsDecisionPrefill({ key: 'result:S', values: { source: 'SRC', layerCode: 'RELEASE_RESULT' } }), {});
});

test('占位的横线与并成一格的多项不带', () => {
  deepEqual(customsDecisionPrefill({ key: 'unit:U', values: { id: 'UNIT-1', caseId: '—', procedure: 'A\nB' } })['declaration-submission'], {
    unitId: 'UNIT-1',
  });
});
