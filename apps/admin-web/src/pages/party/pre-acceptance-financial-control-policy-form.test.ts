import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { PreAcceptanceFinancialControlPolicyRecord } from './api';
import {
  controlPolicyCodesOf,
  controlPolicyFieldPaths,
  controlPolicyLocalProblems,
  controlPolicyPayloadOf,
  controlRowHints,
  emptyControlItemDraft,
  emptyControlPolicyDraft,
  preAcceptanceFinancialControlPolicyDraftFromRecord,
  withControlRowAdded,
  withControlRowPatched,
  withControlRowRemoved,
  type ControlPolicyDraft,
} from './pre-acceptance-financial-control-policy-form';

// 票 admin-write-faces/13：接受前财务控制策略逐字段表单的纯逻辑。钉的是「草稿 → 载荷」这一步与它对公共半边
// （publication-draft-flow）的两个承诺：载荷形状逐格镜像 Go 的 CommercialPublicationPayload.preAcceptanceFinancialControlPolicy
// （父行一格 + 子表逐行五键，键名照批文），表单认领的 JSON 路径覆盖它自己能产出的每一条键（子表按行展开）；
// 外加「控制项可加行」「三个封闭集只吃服务端词表」「重复只提示、拒绝由服务端说」三条选形。

const filled: ControlPolicyDraft = {
  objectId: ' SYN-FIN-CONTROL-01 ',
  version: 'v2',
  scope: 'SYN-SCOPE-01',
  effectiveStartsAt: '2026-10-01T08:00',
  effectiveEndsAt: '',
  jointPassCondition: 'ALL_CONTROLS_PASS',
  controls: [
    { control: 'CREDIT_CHECK', chargeScope: ' charge-terms ', order: ' 2 ', onFailure: 'AUTHORIZED_DISPOSITION', responsibility: 'operator-legal-1' },
    { control: 'PREPAID_FREEZE', chargeScope: 'charge-prepaid', order: '1', onFailure: 'REJECT', responsibility: ' customer-1' },
  ],
};

// 时刻格是日期时间控件的本地墙钟，按显示时区换成 UTC，与登记表单同一条换算（票 party-commercial-uiux/10）。
test('草稿组成载荷：kind 固定 PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY，各格去首尾空白，控制项逐行五键、顺序是整数，本地墙钟按显示时区换成 UTC', () => {
  deepEqual(controlPolicyPayloadOf(filled, 'Asia/Shanghai'), {
    kind: 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY',
    objectId: 'SYN-FIN-CONTROL-01',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    preAcceptanceFinancialControlPolicy: {
      jointPassCondition: 'ALL_CONTROLS_PASS',
      controls: [
        { control: 'CREDIT_CHECK', chargeScope: 'charge-terms', order: 2, onFailure: 'AUTHORIZED_DISPOSITION', responsibility: 'operator-legal-1' },
        { control: 'PREPAID_FREEZE', chargeScope: 'charge-prepaid', order: 1, onFailure: 'REJECT', responsibility: 'customer-1' },
      ],
    },
  });
});

// 表单不排序：行序原样送，文档按判断顺序归一是服务端规范化的事（换行序摘要不变由 Go 侧测试钉）。
test('行序原样送，不在本地按顺序重排', () => {
  const payload = controlPolicyPayloadOf(filled);
  deepEqual(
    payload.preAcceptanceFinancialControlPolicy!.controls.map((row) => row.order),
    [2, 1],
  );
});

// 服务端按键在场与否分辨「没有上界」；壳上不带指名引用；零行仍送空数组——「至少一项」由构造门答，表单不拦。
test('区间上界留空即键缺席；壳上不带指名引用；零行仍送 controls: []', () => {
  const payload = controlPolicyPayloadOf({ ...filled, controls: [] });
  ok(!('effectiveEndsAt' in payload));
  ok(!('references' in payload));
  deepEqual(payload.preAcceptanceFinancialControlPolicy!.controls, []);
  ok('controls' in payload.preAcceptanceFinancialControlPolicy!);

  const bounded = controlPolicyPayloadOf({ ...filled, effectiveEndsAt: '2027-01-01T00:00' }, 'Asia/Shanghai');
  equal(bounded.effectiveEndsAt, '2026-12-31T16:00:00Z');
});

// 票 party-commercial-uiux/10：时刻格换不成时刻是本地问题，落在那一格路径上，挡住预览；空着不是。
test('时刻格换不成时刻才是本地问题，落在该格路径上', () => {
  deepEqual(controlPolicyLocalProblems({ ...filled, effectiveStartsAt: '2026-10-01', effectiveEndsAt: 'tomorrow' }, 'Asia/Shanghai'), {
    effectiveStartsAt: ['时间格式不对，请用日期时间选择器重新选'],
    effectiveEndsAt: ['时间格式不对，请用日期时间选择器重新选'],
  });
  deepEqual(controlPolicyLocalProblems(filled, 'Asia/Shanghai'), {});
  deepEqual(controlPolicyLocalProblems(emptyControlPolicyDraft(), 'Asia/Shanghai'), {});
});

// 三个封闭集的码原样送（只去空白）：未选是空串而不是缺键，服务端点名那一格；表单不改大小写、不查表——改了就是本地在裁。
test('未选的共同通过条件 / 种类 / 处置是空串而不是缺键', () => {
  const payload = controlPolicyPayloadOf({
    ...filled,
    jointPassCondition: '',
    controls: [{ ...filled.controls[0], control: '', onFailure: ' reject ' }],
  });
  equal(payload.preAcceptanceFinancialControlPolicy!.jointPassCondition, '');
  ok('jointPassCondition' in payload.preAcceptanceFinancialControlPolicy!);
  equal(payload.preAcceptanceFinancialControlPolicy!.controls[0].control, '');
  equal(payload.preAcceptanceFinancialControlPolicy!.controls[0].onFailure, 'reject');
});

// 判断顺序是整数格：空即缺席（服务端把缺席读成 0、点名 .order），编不进 JSON 整数的文本也缺席且记进本地问题——
// 那不是替服务端判，是根本组不出那份载荷（判据同 credit-policy-form 的额度格）。零与负数照发，让服务端说「须从 1 起」。
test('顺序留空即键缺席；非整数文本缺席并记进本地问题；零与负数照发', () => {
  const draft: ControlPolicyDraft = {
    ...filled,
    controls: [
      { ...filled.controls[0], order: '' },
      { ...filled.controls[1], order: '1.5' },
      { ...emptyControlItemDraft(), order: 'two' },
      { ...emptyControlItemDraft(), order: '0' },
      { ...emptyControlItemDraft(), order: '-3' },
    ],
  };
  const rows = controlPolicyPayloadOf(draft).preAcceptanceFinancialControlPolicy!.controls;
  ok(!('order' in rows[0]));
  ok(!('order' in rows[1]));
  ok(!('order' in rows[2]));
  equal(rows[3].order, 0);
  equal(rows[4].order, -3);

  const problems = controlPolicyLocalProblems(draft);
  deepEqual(Object.keys(problems).sort(), [
    'preAcceptanceFinancialControlPolicy.controls[1].order',
    'preAcceptanceFinancialControlPolicy.controls[2].order',
  ]);
  deepEqual(controlPolicyLocalProblems(filled), {});
});

// 伞票 07 硬句：表单不算摘要、不收也不送批准人；租户与录入者由操作者信封给。本册特有：「无控制」那一格属合同声明
// （ADR-0115 Decision 一），本正文的载荷里没有合同层那些键（requirement / notApplicableBasis / inapplicabilityBasis / policy）。
test('载荷里没有身份、没有摘要，也没有合同层声明的键', () => {
  const text = JSON.stringify(controlPolicyPayloadOf(filled));
  for (const forbidden of ['tenant', 'submitter', 'approver', 'contentDigest', 'approval', 'canonicalization']) {
    ok(!text.includes(`"${forbidden}`), `载荷不该带 ${forbidden}`);
  }
  for (const contractLevel of ['requirement', 'notApplicableBasis', 'inapplicabilityBasis', 'policy', 'preAcceptanceControl"']) {
    ok(!text.includes(`"${contractLevel}`), `载荷不该带合同层的 ${contractLevel}`);
  }
});

// 公共半边把服务端点名、表单没认领的路径单列为「未认领」。表单自己能产出的每一条键都必须已认领，子表按行展开到
// controls[i].<键>；反向只许多认领行本身（controls[i]）——服务端在五格各自立得住而行拼不成时点名整行，载荷产不出它。
test('表单认领的 JSON 路径覆盖载荷能产出的每一条键（子表按行展开）；多认领的只有行本身', () => {
  const draft = { ...filled, effectiveEndsAt: '2027-01-01T00:00' };
  const payload = controlPolicyPayloadOf(draft);
  const produced: string[] = [];
  const walk = (prefix: string, value: unknown) => {
    if (Array.isArray(value)) {
      value.forEach((item, index) => walk(`${prefix}[${index}]`, item));
    } else if (value !== null && typeof value === 'object') {
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
  const claimed = controlPolicyFieldPaths(draft);
  for (const path of produced) {
    ok(claimed.includes(path), `路径 ${path} 未被表单认领`);
  }
  const rowOnly = claimed.filter((path) => !produced.includes(path));
  deepEqual(rowOnly, [
    'preAcceptanceFinancialControlPolicy.controls[0]',
    'preAcceptanceFinancialControlPolicy.controls[1]',
  ]);
  // 认领表随行数走：删一行少一组路径。
  ok(!controlPolicyFieldPaths({ ...draft, controls: [draft.controls[0]] }).some((path) => path.includes('controls[1]')));
});

// 三个下拉只吃服务端词表（票 20：一口按 kind 答各册正文的封闭集，集合名就是载荷里那格的键名）：本文件不内置
// ALL_CONTROLS_PASS / PREPAID_FREEZE / REJECT，只按名取集；没有那一集就是没有，表单据此显占位、不自造码。
test('从词表答复里按名取一集的码；缺席交回 null 而不是空数组', () => {
  const sets = [
    { name: 'jointPassCondition', codes: ['ALL_CONTROLS_PASS'] },
    { name: 'control', codes: ['PREPAID_FREEZE', 'CREDIT_CHECK'] },
    { name: 'onFailure', codes: [] },
  ];
  deepEqual(controlPolicyCodesOf(sets, 'jointPassCondition'), ['ALL_CONTROLS_PASS']);
  deepEqual(controlPolicyCodesOf(sets, 'control'), ['PREPAID_FREEZE', 'CREDIT_CHECK']);
  deepEqual(controlPolicyCodesOf(sets, 'onFailure'), []);
  equal(controlPolicyCodesOf(sets, 'method'), null);
  equal(controlPolicyCodesOf([], 'control'), null);
});

// 票 13「表单可以在提交前提示重复，但拒绝的话由服务端说」：提示只是提示——不进 problems、不挡预览；两行抢同一个判断
// 顺序、同一范围上同一种控制两行各提示一句；格没填全的行不参与比对（空值不算撞）。
test('重复提示：顺序撞了、（种类 × 范围）撞了各一句；没填全的行不参与；无重复即无提示', () => {
  deepEqual(controlRowHints(filled), []);
  const sameOrder = { ...filled, controls: [filled.controls[0], { ...filled.controls[1], order: '2' }] };
  const orderHints = controlRowHints(sameOrder);
  equal(orderHints.length, 1);
  ok(orderHints[0].includes('判断顺序 2'), orderHints[0]);
  ok(orderHints[0].includes('第 1、2 行'), orderHints[0]);
  ok(orderHints[0].includes('服务端'), orderHints[0]);

  const sameKey = {
    ...filled,
    controls: [filled.controls[0], { ...filled.controls[1], control: 'CREDIT_CHECK', chargeScope: 'charge-terms' }],
  };
  const keyHints = controlRowHints(sameKey);
  equal(keyHints.length, 1);
  ok(keyHints[0].includes('charge-terms'), keyHints[0]);
  ok(keyHints[0].includes('CREDIT_CHECK'), keyHints[0]);

  const blanks = { ...filled, controls: [emptyControlItemDraft(), emptyControlItemDraft()] };
  deepEqual(controlRowHints(blanks), []);
});

test('加行追加一行空控制项；删行按下标去掉那一行；改行只动那一行', () => {
  const added = withControlRowAdded(filled);
  equal(added.controls.length, 3);
  deepEqual(added.controls[2], emptyControlItemDraft());
  deepEqual(added.controls.slice(0, 2), filled.controls);

  const removed = withControlRowRemoved(filled, 0);
  deepEqual(removed.controls, [filled.controls[1]]);
  deepEqual(withControlRowRemoved(filled, 5).controls, filled.controls);

  const patched = withControlRowPatched(filled, 1, { responsibility: 'customer-2' });
  equal(patched.controls[1].responsibility, 'customer-2');
  equal(patched.controls[1].control, 'PREPAID_FREEZE');
  deepEqual(patched.controls[0], filled.controls[0]);
  equal(patched.objectId, filled.objectId);
});

// 首发共同通过条件只有一值，仍不预选：那是租户说出来的，不是产品替它默认的（ADR-0115 Decision 三）。空草稿带一行空
// 控制项——那是可填的格，不是值。
test('空草稿：壳五格与共同通过条件全空、不预选；带一行空控制项，五格全空', () => {
  const draft = emptyControlPolicyDraft();
  equal(Object.keys(draft).length, 7);
  for (const [key, value] of Object.entries(draft)) {
    if (key === 'controls') continue;
    equal(value, '', `${key} 不为空`);
  }
  equal(draft.controls.length, 1);
  equal(Object.keys(draft.controls[0]).length, 5);
  for (const [key, value] of Object.entries(draft.controls[0])) equal(value, '', `controls[0].${key} 不为空`);
});

// Covers: 票 party-commercial-uiux/14——基于此版本发新版本：对象标识、版本适用范围、共同通过条件与控制项（按判断顺序一行，
// 顺序数转文本）照抄；版本号与版本的生效区间留空，哪怕记录上有终止时刻。
test('基于此版本预填：抄对象、适用范围、共同通过条件与控制项，留版本号与生效区间；顺序 0 不折成缺席', () => {
  const record: PreAcceptanceFinancialControlPolicyRecord = {
    objectId: 'SYN-FIN-CONTROL-01',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    status: 'PUBLISHED',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    effectiveEndsAt: '2026-12-31T00:00:00Z',
    publishedAt: '2026-01-01T00:00:00Z',
    contentRegistered: true,
    content: {
      jointPassCondition: 'ALL_CONTROLS_PASS',
      registeredAt: '2026-01-01T00:00:00Z',
      controls: [
        { control: 'PREPAID_FREEZE', chargeScope: 'charge-freight', order: 0, onFailure: 'REJECT', responsibility: 'operator-legal-1' },
        { control: 'CREDIT_CHECK', chargeScope: 'charge-terms', order: 2, onFailure: 'AUTHORIZED_DISPOSITION', responsibility: 'customer-1' },
      ],
    },
  };
  const draft = preAcceptanceFinancialControlPolicyDraftFromRecord(record);
  deepEqual(draft, {
    ...emptyControlPolicyDraft(),
    objectId: 'SYN-FIN-CONTROL-01',
    scope: 'SYN-SCOPE-01',
    jointPassCondition: 'ALL_CONTROLS_PASS',
    controls: [
      { control: 'PREPAID_FREEZE', chargeScope: 'charge-freight', order: '0', onFailure: 'REJECT', responsibility: 'operator-legal-1' },
      { control: 'CREDIT_CHECK', chargeScope: 'charge-terms', order: '2', onFailure: 'AUTHORIZED_DISPOSITION', responsibility: 'customer-1' },
    ],
  });
  equal(draft.version, '');
  equal(draft.effectiveStartsAt, '');
  equal(draft.effectiveEndsAt, '');

  // 抄回去的控制项原样出现在载荷里：由草稿组载荷的那条路径能把它们拼回记录上的样子。
  const payload = controlPolicyPayloadOf({ ...draft, version: 'v3', effectiveStartsAt: '2027-01-01T00:00' }, 'UTC');
  deepEqual(payload.preAcceptanceFinancialControlPolicy?.controls, record.content?.controls);
});

// Covers: 票 party-commercial-uiux/14——读面上可缺的正文：contentRegistered 为假时（响应里没有 content 键）只还原壳那几格，
// 共同通过条件留空、控制项沿用空草稿预开的一行空行；记录给了空数组也不崩、不硬塞，同样是那一行空行。
test('基于此版本预填：正文未登记只还原壳；控制项为空数组沿用预开的一行空行', () => {
  const shell: PreAcceptanceFinancialControlPolicyRecord = {
    objectId: 'SYN-FIN-CONTROL-02',
    version: 'v1',
    scope: 'SYN-SCOPE-02',
    status: 'PUBLISHED',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    publishedAt: '2026-01-01T00:00:00Z',
    contentRegistered: false,
  };
  deepEqual(preAcceptanceFinancialControlPolicyDraftFromRecord(shell), {
    ...emptyControlPolicyDraft(),
    objectId: 'SYN-FIN-CONTROL-02',
    scope: 'SYN-SCOPE-02',
  });

  // contentRegistered 为假时即便响应带了 content 也不还原：是否在册只认这个布尔。
  const stray = preAcceptanceFinancialControlPolicyDraftFromRecord({
    ...shell,
    content: { jointPassCondition: 'ALL_CONTROLS_PASS', registeredAt: '2026-01-01T00:00:00Z', controls: [] },
  });
  equal(stray.jointPassCondition, '');

  const noControls = preAcceptanceFinancialControlPolicyDraftFromRecord({
    ...shell,
    contentRegistered: true,
    content: { jointPassCondition: 'ALL_CONTROLS_PASS', registeredAt: '2026-01-01T00:00:00Z', controls: [] },
  });
  equal(noControls.jointPassCondition, 'ALL_CONTROLS_PASS');
  deepEqual(noControls.controls, [emptyControlItemDraft()]);
});
