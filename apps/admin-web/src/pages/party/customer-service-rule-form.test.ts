import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import {
  emptyClaimDeadlineDraft,
  emptyMinimumMaterialsDraft,
  emptyServiceRuleDraft,
  serviceRuleCodesOf,
  serviceRuleFieldPaths,
  serviceRuleLocalProblems,
  serviceRulePayloadOf,
  withClaimDeadlineRowAdded,
  withClaimDeadlineRowPatched,
  withClaimDeadlineRowRemoved,
  withMinimumMaterialsRowAdded,
  withMinimumMaterialsRowPatched,
  withMinimumMaterialsRowRemoved,
  type ServiceRuleDraft,
} from './customer-service-rule-form';

// 票 admin-write-faces/18：客户服务规则逐字段表单的纯逻辑。钉的是「草稿 → 载荷」这一步与它对公共半边
// （publication-draft-flow）的两个承诺：载荷形状逐格镜像 Go 的 CommercialPublicationPayload.customerServiceRule
// （适用对象两键恰一 + 父行两格 + 两张子表逐行，键名照批文），表单认领的 JSON 路径覆盖它自己能产出的每一条键
// （子表按行展开、材料清单按项展开）；外加「适用对象二选一控件」「期限种类只吃服务端词表」「两张子表可加行」三条选形。

const filled: ServiceRuleDraft = {
  objectId: ' SYN-CSR-01 ',
  version: 'v2',
  scope: 'SYN-SCOPE-01',
  effectiveStartsAt: '2026-10-01',
  effectiveEndsAt: '',
  appliesTo: 'serviceProduct',
  appliesToId: ' SYN-PRODUCT-01 ',
  responsible: 'operator-1 ',
  ruleScope: ' SYN-SCOPE-01',
  claimDeadlines: [
    { kind: 'CONCLUSION_REVIEW', startEvent: ' event-conclusion-notified ', days: ' 15 ', calendar: 'calendar-cn' },
    { kind: 'FIRST_CLAIM', startEvent: 'event-delivered', days: '30', calendar: ' calendar-cn ' },
  ],
  minimumMaterials: [
    { claimKind: ' claim-loss ', materials: ' material-photo \n\n material-invoice\n' },
  ],
};

test('草稿组成载荷：kind 固定 CUSTOMER_SERVICE_RULE，适用对象同一个值送进正文与壳上的指名引用，各格去首尾空白，期限行四键 days 是整数，材料一行一项，只到天的日期补成当天零点 UTC', () => {
  deepEqual(serviceRulePayloadOf(filled), {
    kind: 'CUSTOMER_SERVICE_RULE',
    objectId: 'SYN-CSR-01',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    references: { SERVICE_PRODUCT: 'SYN-PRODUCT-01' },
    customerServiceRule: {
      serviceProduct: 'SYN-PRODUCT-01',
      responsible: 'operator-1',
      scope: 'SYN-SCOPE-01',
      claimDeadlines: [
        { kind: 'CONCLUSION_REVIEW', startEvent: 'event-conclusion-notified', days: 15, calendar: 'calendar-cn' },
        { kind: 'FIRST_CLAIM', startEvent: 'event-delivered', days: 30, calendar: 'calendar-cn' },
      ],
      minimumMaterials: [{ claimKind: 'claim-loss', materials: ['material-photo', 'material-invoice'] }],
    },
  });
});

// 适用对象恰一由服务端裁（票 18）：表单只把二选一控件的取值折成两键之一。选客户合同即 customerContract + 壳上
// CUSTOMER_CONTRACT；未选即两键都缺席、壳上不带引用——服务端点名 customerServiceRule.applicability；选了却没填标识，
// 正文键在场为空串（服务端同样答 applicability），壳上不带一个空引用。
test('适用对象：选合同送 customerContract 与壳上 CUSTOMER_CONTRACT；未选两键皆缺席；选了没填标识只送空串不送壳引用', () => {
  const contract = serviceRulePayloadOf({ ...filled, appliesTo: 'customerContract', appliesToId: 'SYN-CONTRACT-01' });
  equal(contract.customerServiceRule!.customerContract, 'SYN-CONTRACT-01');
  ok(!('serviceProduct' in contract.customerServiceRule!));
  deepEqual(contract.references, { CUSTOMER_CONTRACT: 'SYN-CONTRACT-01' });

  const unchosen = serviceRulePayloadOf({ ...filled, appliesTo: '' });
  ok(!('serviceProduct' in unchosen.customerServiceRule!));
  ok(!('customerContract' in unchosen.customerServiceRule!));
  ok(!('references' in unchosen));

  const blank = serviceRulePayloadOf({ ...filled, appliesToId: '  ' });
  equal(blank.customerServiceRule!.serviceProduct, '');
  ok('serviceProduct' in blank.customerServiceRule!);
  ok(!('references' in blank));
});

// 服务端按键在场与否分辨「没有上界」；两张表零行仍各送 `[]`——「两项合起来至少一行」是跨行的门，由构造门答，表单不拦；
// 行序原样送，文档按种类序 / 索赔类型序归一是服务端规范化的事（换行序摘要不变由 Go 侧测试钉）。
test('区间上界留空即键缺席；两张表零行仍送空数组；行序原样送', () => {
  const payload = serviceRulePayloadOf({ ...filled, claimDeadlines: [], minimumMaterials: [] });
  ok(!('effectiveEndsAt' in payload));
  deepEqual(payload.customerServiceRule!.claimDeadlines, []);
  deepEqual(payload.customerServiceRule!.minimumMaterials, []);
  ok('claimDeadlines' in payload.customerServiceRule! && 'minimumMaterials' in payload.customerServiceRule!);

  const bounded = serviceRulePayloadOf({ ...filled, effectiveEndsAt: '2027-01-01' });
  equal(bounded.effectiveEndsAt, '2027-01-01T00:00:00Z');

  deepEqual(
    serviceRulePayloadOf(filled).customerServiceRule!.claimDeadlines.map((row) => row.kind),
    ['CONCLUSION_REVIEW', 'FIRST_CLAIM'],
  );
});

// 期限种类的码原样送（只去空白）：未选是空串而不是缺键，服务端点名那一格；表单不改大小写、不查表——改了就是本地在裁。
// 起算事件、日历、索赔类型是开放引用，同样原样送。
test('未选的期限种类是空串而不是缺键；码与引用只去空白不改写', () => {
  const payload = serviceRulePayloadOf({
    ...filled,
    claimDeadlines: [{ ...filled.claimDeadlines[0], kind: '', startEvent: ' Event-X ' }],
  });
  equal(payload.customerServiceRule!.claimDeadlines[0].kind, '');
  ok('kind' in payload.customerServiceRule!.claimDeadlines[0]);
  equal(payload.customerServiceRule!.claimDeadlines[0].startEvent, 'Event-X');
});

// 时长是整数格：空即缺席（服务端把缺席读成 0、点名 .days），编不进 JSON 整数的文本也缺席且记进本地问题——那不是替
// 服务端判，是根本组不出那份载荷（判据同 pre-acceptance-financial-control-policy-form 的顺序格）。零与负数照发，让服务端说「须为正」。
test('时长留空即键缺席；非整数文本缺席并记进本地问题；零与负数照发', () => {
  const draft: ServiceRuleDraft = {
    ...filled,
    claimDeadlines: [
      { ...filled.claimDeadlines[0], days: '' },
      { ...filled.claimDeadlines[1], days: '1.5' },
      { ...emptyClaimDeadlineDraft(), days: 'thirty' },
      { ...emptyClaimDeadlineDraft(), days: '0' },
      { ...emptyClaimDeadlineDraft(), days: '-3' },
    ],
  };
  const rows = serviceRulePayloadOf(draft).customerServiceRule!.claimDeadlines;
  ok(!('days' in rows[0]));
  ok(!('days' in rows[1]));
  ok(!('days' in rows[2]));
  equal(rows[3].days, 0);
  equal(rows[4].days, -3);

  deepEqual(Object.keys(serviceRuleLocalProblems(draft)).sort(), [
    'customerServiceRule.claimDeadlines[1].days',
    'customerServiceRule.claimDeadlines[2].days',
  ]);
  deepEqual(serviceRuleLocalProblems(filled), {});
});

// 材料清单是多行文本、一行一项：空行不是项，各项去首尾空白；重复的项照发——「至少一项且不重复」由 NewMinimumMaterialsRule 答。
test('材料清单一行一项：空行不算项、各项去空白、重复照发；全空即空清单', () => {
  const payload = serviceRulePayloadOf({
    ...filled,
    minimumMaterials: [
      { claimKind: 'claim-a', materials: 'material-photo\nmaterial-photo\n  \nmaterial-invoice' },
      { claimKind: 'claim-b', materials: '\n  \n' },
    ],
  });
  deepEqual(payload.customerServiceRule!.minimumMaterials[0].materials, ['material-photo', 'material-photo', 'material-invoice']);
  deepEqual(payload.customerServiceRule!.minimumMaterials[1].materials, []);
});

// 伞票 07 硬句：表单不算摘要、不收也不送批准人；租户与录入者由操作者信封给。本册特有：只发布 PC 这一侧的正文——
// 载荷里除本册一格外没有别册的正文键。
test('载荷里没有身份、没有摘要，正文只有本册一格', () => {
  const payload = serviceRulePayloadOf(filled);
  const text = JSON.stringify(payload);
  for (const forbidden of ['tenant', 'submitter', 'approver', 'contentDigest', 'approval', 'canonicalization']) {
    ok(!text.includes(`"${forbidden}`), `载荷不该带 ${forbidden}`);
  }
  const bodyKeys = Object.keys(payload).filter(
    (key) => !['kind', 'objectId', 'version', 'scope', 'effectiveStartsAt', 'effectiveEndsAt', 'references'].includes(key),
  );
  deepEqual(bodyKeys, ['customerServiceRule']);
});

// 公共半边把服务端点名、表单没认领的路径单列为「未认领」。表单自己能产出的每一条键都必须已认领，两张子表按行展开、
// 材料按项展开；反向只许多认领三样：行本身（服务端在各格立得住而行拼不成时点名整行）与 applicability 一格（两键恰一
// 由服务端判成这一格的问题，载荷里没有这个键）。
test('表单认领的 JSON 路径覆盖载荷能产出的每一条键；多认领的只有行本身与 applicability', () => {
  const draft: ServiceRuleDraft = {
    ...filled,
    effectiveEndsAt: '2027-01-01',
    minimumMaterials: [...filled.minimumMaterials, { claimKind: 'claim-damage', materials: 'material-photo' }],
  };
  const payload = serviceRulePayloadOf(draft);
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
  const claimed = serviceRuleFieldPaths(draft);
  for (const path of produced) {
    ok(claimed.includes(path), `路径 ${path} 未被表单认领`);
  }
  deepEqual(
    claimed.filter((path) => !produced.includes(path)),
    [
      'customerServiceRule.applicability',
      'customerServiceRule.claimDeadlines[0]',
      'customerServiceRule.claimDeadlines[1]',
      'customerServiceRule.minimumMaterials[0]',
      'customerServiceRule.minimumMaterials[1]',
    ],
  );
  // 认领表随控件与行数走：选合同即认领合同那两条、不再认领产品那两条；删一行少一组路径。
  const contract = serviceRuleFieldPaths({ ...draft, appliesTo: 'customerContract' });
  ok(contract.includes('references.CUSTOMER_CONTRACT') && contract.includes('customerServiceRule.customerContract'));
  ok(!contract.includes('references.SERVICE_PRODUCT') && !contract.includes('customerServiceRule.serviceProduct'));
  ok(!serviceRuleFieldPaths({ ...draft, claimDeadlines: [draft.claimDeadlines[0]] }).some((path) => path.includes('claimDeadlines[1]')));
});

// 期限种类下拉只吃服务端词表（票 20：一口按 kind 答各册正文的封闭集，集合名就是载荷里那格的键名）：本文件不内置
// FIRST_CLAIM / MATERIAL_SUPPLEMENT / CONCLUSION_REVIEW，只按名取集；没有那一集就是没有，表单据此显占位、不自造码。
test('从词表答复里按名取一集的码；缺席交回 null 而不是空数组', () => {
  const sets = [{ name: 'kind', codes: ['FIRST_CLAIM', 'MATERIAL_SUPPLEMENT', 'CONCLUSION_REVIEW'] }];
  deepEqual(serviceRuleCodesOf(sets, 'kind'), ['FIRST_CLAIM', 'MATERIAL_SUPPLEMENT', 'CONCLUSION_REVIEW']);
  equal(serviceRuleCodesOf(sets, 'startEvent'), null);
  equal(serviceRuleCodesOf([], 'kind'), null);
  deepEqual(serviceRuleCodesOf([{ name: 'kind', codes: [] }], 'kind'), []);
});

test('两张表各自加行、删行、改行：加行追加一行空行，删行按下标，改行只动那一行', () => {
  const added = withClaimDeadlineRowAdded(filled);
  equal(added.claimDeadlines.length, 3);
  deepEqual(added.claimDeadlines[2], emptyClaimDeadlineDraft());
  deepEqual(added.minimumMaterials, filled.minimumMaterials);

  const removed = withClaimDeadlineRowRemoved(filled, 0);
  deepEqual(removed.claimDeadlines, [filled.claimDeadlines[1]]);
  deepEqual(withClaimDeadlineRowRemoved(filled, 5).claimDeadlines, filled.claimDeadlines);

  const patched = withClaimDeadlineRowPatched(filled, 1, { days: '45' });
  equal(patched.claimDeadlines[1].days, '45');
  equal(patched.claimDeadlines[1].kind, 'FIRST_CLAIM');
  deepEqual(patched.claimDeadlines[0], filled.claimDeadlines[0]);

  const materialsAdded = withMinimumMaterialsRowAdded(filled);
  equal(materialsAdded.minimumMaterials.length, 2);
  deepEqual(materialsAdded.minimumMaterials[1], emptyMinimumMaterialsDraft());
  deepEqual(withMinimumMaterialsRowRemoved(filled, 0).minimumMaterials, []);
  const materialsPatched = withMinimumMaterialsRowPatched(filled, 0, { claimKind: 'claim-damage' });
  equal(materialsPatched.minimumMaterials[0].claimKind, 'claim-damage');
  equal(materialsPatched.minimumMaterials[0].materials, filled.minimumMaterials[0].materials);
  equal(materialsPatched.objectId, filled.objectId);
});

// 不预选、不给默认：适用对象二选一控件起始未选；两张表起始零行——「这一版对期限无客户差异」是正文说出的真话，
// 预开一行空行会替操作者预设「这张表有内容」，删行才能表达空表。
test('空草稿：壳五格与父行三格全空，适用对象未选，两张表零行', () => {
  const draft = emptyServiceRuleDraft();
  equal(Object.keys(draft).length, 11);
  for (const [key, value] of Object.entries(draft)) {
    if (key === 'claimDeadlines' || key === 'minimumMaterials') continue;
    equal(value, '', `${key} 不为空`);
  }
  deepEqual(draft.claimDeadlines, []);
  deepEqual(draft.minimumMaterials, []);
  equal(Object.keys(emptyClaimDeadlineDraft()).length, 4);
  equal(Object.keys(emptyMinimumMaterialsDraft()).length, 2);
});
