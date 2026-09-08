import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import {
  contractChoiceKey,
  contractChoiceOf,
  emptySettlementPolicyDraft,
  methodCodesOf,
  settlementPolicyFieldPaths,
  settlementPolicyPayloadOf,
  withShellIntervalCopiedIntoPolicy,
  type SettlementPolicyDraft,
} from './settlement-policy-form';

// 票 admin-write-faces/15：结算政策逐字段表单的纯逻辑。钉的是「草稿 → 载荷」这一步与它对公共半边
// （publication-draft-flow）的两个承诺：载荷形状逐格镜像 Go 的 CommercialPublicationPayload.settlementPolicy
// （合同维是 objectId / version 两格，不是拼好的串），表单认领的 JSON 路径覆盖它自己能产出的每一条键；
// 外加合同「对象 + 版本一起选」与「方式下拉只吃服务端词表」两条选形。

const filled: SettlementPolicyDraft = {
  objectId: ' SYN-SETTLEMENT-PREPAID-01 ',
  version: 'v2',
  scope: 'SYN-SCOPE-01',
  effectiveStartsAt: '2026-10-01',
  effectiveEndsAt: '',
  method: 'PREPAID',
  legalEntity: 'SYN-LE-01 ',
  counterparty: 'SYN-ACCOUNT-01',
  contractObjectId: 'SYN-CONTRACT-01',
  contractVersion: ' v1',
  chargeScope: 'SYN-CHARGE-PREPAID',
  currency: ' cny ',
  policyEffectiveStartsAt: '2026-10-01T00:00:00Z',
  policyEffectiveEndsAt: '2026-12-31',
};

test('草稿组成载荷：kind 固定 SETTLEMENT_POLICY，各格去首尾空白，合同维是对象 + 版本两格，只到天的日期补成当天零点 UTC', () => {
  deepEqual(settlementPolicyPayloadOf(filled), {
    kind: 'SETTLEMENT_POLICY',
    objectId: 'SYN-SETTLEMENT-PREPAID-01',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    settlementPolicy: {
      method: 'PREPAID',
      legalEntity: 'SYN-LE-01',
      counterparty: 'SYN-ACCOUNT-01',
      contract: { objectId: 'SYN-CONTRACT-01', version: 'v1' },
      chargeScope: 'SYN-CHARGE-PREPAID',
      currency: 'cny',
      effectiveStartsAt: '2026-10-01T00:00:00Z',
      effectiveEndsAt: '2026-12-31T00:00:00Z',
    },
  });
});

// 币种收 ISO 代码串、存在性由构造门答（票面「选形与理由」）：表单只去空白，不改大小写、不查表——改了就是本地在裁。
test('币种原样送、方式原样送：未选方式是空串而不是缺键，服务端点名 settlementPolicy.method', () => {
  const payload = settlementPolicyPayloadOf({ ...filled, method: '', currency: 'usd' });
  equal(payload.settlementPolicy!.method, '');
  equal(payload.settlementPolicy!.currency, 'usd');
  ok('method' in payload.settlementPolicy!);
});

// 服务端按键在场与否分辨「没有上界」，空串会被当成一个填了空的时刻送进构造门；壳与正文两级同一条规矩。
test('区间上界留空即键缺席，不送空串；壳上不带指名引用', () => {
  const payload = settlementPolicyPayloadOf({ ...filled, policyEffectiveEndsAt: '  ' });
  ok(!('effectiveEndsAt' in payload));
  ok(!('effectiveEndsAt' in payload.settlementPolicy!));
  ok(!('references' in payload));

  const bounded = settlementPolicyPayloadOf({ ...filled, effectiveEndsAt: '2027-01-01' });
  equal(bounded.effectiveEndsAt, '2027-01-01T00:00:00Z');
});

// 伞票 07 硬句：表单不算摘要、不收也不送批准人；租户与录入者由操作者信封给。载荷里出现这些键会被服务端
// 按未知键拒，所以这里钉死它们一个都不在。本册特有的硬句一并钉：结算政策答「怎么结」不答「要不要接受前控制」，
// 载荷里没有任何控制字段。
test('载荷里没有身份、没有摘要，也没有接受前控制的字段', () => {
  const text = JSON.stringify(settlementPolicyPayloadOf(filled));
  for (const forbidden of ['tenant', 'submitter', 'approver', 'contentDigest', 'approval', 'canonicalization']) {
    ok(!text.includes(`"${forbidden}`), `载荷不该带 ${forbidden}`);
  }
  for (const control of ['preAcceptanceControl', 'control', 'requirement', 'jointPass', 'contractLabel']) {
    ok(!text.includes(`"${control}"`), `载荷不该带 ${control}`);
  }
});

// 公共半边把服务端点名、表单没认领的路径单列为「未认领」。表单自己能产出的每一条键都必须已认领，
// 否则本册自己的格出了问题会被显示成别处的问题。合同维两层要展开到 contract.objectId / contract.version——
// 服务端逐格问题就落在那两条路径上，不落在 contract 本身。
test('表单认领的 JSON 路径覆盖载荷能产出的每一条键（合同维展开到两格）', () => {
  const payload = settlementPolicyPayloadOf({ ...filled, effectiveEndsAt: '2027-01-01' });
  const produced: string[] = [];
  const walk = (prefix: string, value: unknown) => {
    if (value !== null && typeof value === 'object') {
      for (const [key, nested] of Object.entries(value as Record<string, unknown>)) {
        walk(prefix === '' ? key : `${prefix}.${key}`, nested);
      }
    } else {
      produced.push(prefix);
    }
  };
  for (const [key, value] of Object.entries(payload)) {
    if (key === 'kind') continue;
    walk(key, value);
  }
  for (const path of produced) {
    ok(settlementPolicyFieldPaths.includes(path), `路径 ${path} 未被表单认领`);
  }
  // 反向：认领表里没有载荷产不出的路径——多认领一条会把别处的问题吞进本表单。
  for (const path of settlementPolicyFieldPaths) {
    ok(produced.includes(path), `认领了载荷产不出的路径 ${path}`);
  }
});

// 合同从客户与合同目录选，对象 + 版本两格一起落：选项键把两格编成一个不歧义的串（不拿「/」拼——那是领域
// NewQualifiedVersionLabel 一处的事，表单不拼版本号），选回来按键查目录行取两格。
test('合同选项键 ↔ 目录行：两格一起进出，含「/」的标识也不歧义', () => {
  const contracts = [
    { objectId: 'SYN-CONTRACT-01', version: 'v1' },
    { objectId: 'SYN-CONTRACT-01', version: 'v2' },
    { objectId: 'a/b', version: 'c' },
    { objectId: 'a', version: 'b/c' },
  ];
  const keys = contracts.map(contractChoiceKey);
  equal(new Set(keys).size, contracts.length, '四条目录行要得到四个不同的键');
  deepEqual(contractChoiceOf(keys[1], contracts), { objectId: 'SYN-CONTRACT-01', version: 'v2' });
  deepEqual(contractChoiceOf(keys[2], contracts), { objectId: 'a/b', version: 'c' });
  deepEqual(contractChoiceOf(keys[3], contracts), { objectId: 'a', version: 'b/c' });
  equal(contractChoiceOf('not-a-key', contracts), null);
  equal(contractChoiceOf(contractChoiceKey({ objectId: 'SYN-CONTRACT-09', version: 'v1' }), contracts), null);
  // 手填的两格也编得出键，选单据此把「不在目录上的当前值」保留为一项。
  equal(contractChoiceKey({ objectId: 'SYN-CONTRACT-01', version: 'v2' }), keys[1]);
});

// 方式下拉只吃服务端词表（票 20：一口按 kind 答各册正文的封闭集）：本文件不内置 PREPAID / TERMS，只从答复里
// 取名为 method 的那一集；没有那一集就是没有，表单据此显占位、不自造码。
test('从词表答复里取 method 一集的码；缺席交回 null 而不是空数组', () => {
  deepEqual(
    methodCodesOf([
      { name: 'other', codes: ['X'] },
      { name: 'method', codes: ['PREPAID', 'TERMS'] },
    ]),
    ['PREPAID', 'TERMS'],
  );
  equal(methodCodesOf([]), null);
  equal(methodCodesOf([{ name: 'other', codes: ['X'] }]), null);
  deepEqual(methodCodesOf([{ name: 'method', codes: [] }]), []);
});

test('「区间从版本壳带入」只抄区间两格，范围不抄（壳范围是版本的、费用范围是另一种引用），其余各格不动', () => {
  const copied = withShellIntervalCopiedIntoPolicy({
    ...filled,
    effectiveEndsAt: '2027-01-01',
    chargeScope: 'SYN-CHARGE-PREPAID',
    policyEffectiveStartsAt: 'stale',
    policyEffectiveEndsAt: 'stale',
  });
  equal(copied.policyEffectiveStartsAt, '2026-10-01');
  equal(copied.policyEffectiveEndsAt, '2027-01-01');
  equal(copied.chargeScope, 'SYN-CHARGE-PREPAID');
  equal(copied.method, 'PREPAID');
  equal(copied.contractVersion, ' v1');
});

test('空草稿十四格全空：方式不预选', () => {
  const draft = emptySettlementPolicyDraft();
  equal(Object.keys(draft).length, 14);
  for (const [key, value] of Object.entries(draft)) equal(value, '', `${key} 不为空`);
});
