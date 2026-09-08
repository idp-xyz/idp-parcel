import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import {
  declaresFx,
  emptyPricePolicyDraft,
  planBindingConversionOptions,
  priceDirectionOptions,
  pricePolicyFieldPaths,
  pricePolicyPayloadOf,
  showsTaxClassification,
  showsVolumetricFactor,
  taxDispositionOptions,
  withShellCopiedIntoPolicy,
  type PricePolicyDraft,
} from './price-policy-form';

// 票 admin-write-faces/14：价格规则逐字段表单的纯逻辑。钉的是「草稿 → 载荷」这一步与它对公共半边（publication-draft-flow）
// 的两个承诺：载荷形状逐格镜像 Go 的 CommercialPublicationPayload.pricePolicy（含口径节），表单认领的 JSON 路径覆盖它自己
// 能产出的每一条键；外加本册特有的三句——口径随正文同笔、显隐是呈现不是裁门、封闭集不预选不代填。

const filled: PricePolicyDraft = {
  objectId: ' SYN-PRICE-RULE-CN-SG ',
  version: 'v2',
  scope: 'SYN-SCOPE-01',
  effectiveStartsAt: '2026-10-01',
  effectiveEndsAt: '',
  direction: 'SELL',
  pricingPlan: 'PLAN-CN-SG-SELL@v1',
  planDirection: 'BUY',
  conversion: 'FROZEN_BUY_EVALUATION',
  policyScope: 'pricing-scope-1 ',
  policyEffectiveStartsAt: '2026-10-01T00:00:00Z',
  policyEffectiveEndsAt: '2026-12-31',
  taxDisposition: 'TAX_EXCLUSIVE',
  taxClassification: 'vat-standard',
  volumetricFactor: 'sell-divisor-5000-cm',
  fxQuoteType: 'boc-cash-selling',
  fxAsOfSemantics: 'AT_ORDER_DATE',
  fxAsOfPolicyVersion: 'asof-policy/v3',
};

test('草稿组成载荷：kind 固定 PRICE_RULE，正文与口径节在同一份载荷里，各格去首尾空白，只到天的日期补成当天零点 UTC', () => {
  deepEqual(pricePolicyPayloadOf(filled), {
    kind: 'PRICE_RULE',
    objectId: 'SYN-PRICE-RULE-CN-SG',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    pricePolicy: {
      direction: 'SELL',
      pricingPlan: 'PLAN-CN-SG-SELL@v1',
      planDirection: 'BUY',
      conversion: 'FROZEN_BUY_EVALUATION',
      scope: 'pricing-scope-1',
      effectiveStartsAt: '2026-10-01T00:00:00Z',
      effectiveEndsAt: '2026-12-31T00:00:00Z',
      caliber: {
        taxDisposition: 'TAX_EXCLUSIVE',
        taxClassification: 'vat-standard',
        volumetricFactor: 'sell-divisor-5000-cm',
        fx: { quoteType: 'boc-cash-selling', asOfSemantics: 'AT_ORDER_DATE', asOfPolicyVersion: 'asof-policy/v3' },
      },
    },
  });
});

// 服务端按键在场与否分辨「没有」：区间上界、税务分类、体积系数、汇率节都是缺席而不是空串。口径节本身永远在——本表单
// 不给「先发正文、回头补口径」的两步；口径节里没有方向键。
test('可缺的键缺席而不是空串；口径节永远随正文同送；口径节里没有方向键', () => {
  const payload = pricePolicyPayloadOf({
    ...filled,
    direction: 'BUY',
    planDirection: 'BUY',
    conversion: 'NONE',
    policyEffectiveEndsAt: '  ',
    taxDisposition: 'TAX_NOT_APPLICABLE',
    taxClassification: '',
    volumetricFactor: ' ',
    fxQuoteType: '',
    fxAsOfSemantics: '',
    fxAsOfPolicyVersion: '',
  });
  ok(!('effectiveEndsAt' in payload));
  ok(!('effectiveEndsAt' in payload.pricePolicy!));
  ok(!('references' in payload));
  ok('caliber' in payload.pricePolicy!);
  deepEqual(payload.pricePolicy!.caliber, { taxDisposition: 'TAX_NOT_APPLICABLE' });
  ok(!('direction' in payload.pricePolicy!.caliber!));

  const bounded = pricePolicyPayloadOf({ ...filled, effectiveEndsAt: '2027-01-01' });
  equal(bounded.effectiveEndsAt, '2027-01-01T00:00:00Z');
});

// 汇率一节可缺是合法的商业声明（不涉及外币）；填了任一格就整节送，缺的两格以空串到达、由服务端逐格点名（三格同在同缺
// 由服务端裁，表单不替它补也不替它删）。
test('汇率三格全空即整节缺席；填了任一格就整节送，缺的照空串送去让服务端点名', () => {
  const none = { ...filled, fxQuoteType: '', fxAsOfSemantics: ' ', fxAsOfPolicyVersion: '' };
  equal(declaresFx(none), false);
  ok(!('fx' in pricePolicyPayloadOf(none).pricePolicy!.caliber!));

  const partial = { ...none, fxAsOfSemantics: 'AT_ORDER_DATE' };
  equal(declaresFx(partial), true);
  deepEqual(pricePolicyPayloadOf(partial).pricePolicy!.caliber!.fx, {
    quoteType: '',
    asOfSemantics: 'AT_ORDER_DATE',
    asOfPolicyVersion: '',
  });
});

// 显隐是呈现不是裁门：隐掉的条件格若草稿里有值照送，由服务端按 CHECK 同形的构造门答在那一格；表单不静默丢、不代填。
test('隐掉的条件格有值照送：不适用却填了分类、采购方向却填了系数都原样进载荷', () => {
  const draft = { ...filled, direction: 'BUY', planDirection: 'BUY', conversion: 'NONE', taxDisposition: 'TAX_NOT_APPLICABLE' };
  equal(showsTaxClassification(draft), false);
  equal(showsVolumetricFactor(draft), false);
  const caliber = pricePolicyPayloadOf(draft).pricePolicy!.caliber!;
  equal(caliber.taxClassification, 'vat-standard');
  equal(caliber.volumetricFactor, 'sell-divisor-5000-cm');
});

test('条件格显隐：分类只在含税 / 未税显、系数只在销售方向显；未选时两格都显', () => {
  equal(showsTaxClassification({ taxDisposition: 'TAX_INCLUSIVE' }), true);
  equal(showsTaxClassification({ taxDisposition: 'TAX_EXCLUSIVE' }), true);
  equal(showsTaxClassification({ taxDisposition: 'TAX_NOT_APPLICABLE' }), false);
  equal(showsTaxClassification({ taxDisposition: '' }), true);
  equal(showsVolumetricFactor({ direction: 'SELL' }), true);
  equal(showsVolumetricFactor({ direction: 'BUY' }), false);
  equal(showsVolumetricFactor({ direction: 'INTERNAL' }), false);
  equal(showsVolumetricFactor({ direction: '' }), true);
});

// 封闭集四格不预选、不代填：空串照送，服务端按集合外点名；conversion 空串不折成 NONE（ADR-0057：planDirection 与 conversion
// 如实收、不从方案反推）。选项的码取领域 String() 原词，中文只显给人看。
test('封闭集四格未选即空串照送，不代填；选项码是领域原词', () => {
  const body = pricePolicyPayloadOf({ ...emptyPricePolicyDraft(), pricingPlan: 'p@v1' }).pricePolicy!;
  equal(body.direction, '');
  equal(body.planDirection, '');
  equal(body.conversion, '');
  equal(body.caliber!.taxDisposition, '');
  deepEqual(priceDirectionOptions.map((option) => option.value), ['BUY', 'SELL', 'INTERNAL']);
  deepEqual(planBindingConversionOptions.map((option) => option.value), ['NONE', 'FROZEN_BUY_EVALUATION']);
  deepEqual(taxDispositionOptions.map((option) => option.value), ['TAX_INCLUSIVE', 'TAX_EXCLUSIVE', 'TAX_NOT_APPLICABLE']);
  for (const option of [...priceDirectionOptions, ...planBindingConversionOptions, ...taxDispositionOptions]) {
    ok(option.label.startsWith(`${option.value} · `), `选项 ${option.value} 的中文该跟在码后面`);
  }
});

// 伞票 07 硬句：表单不算摘要、不收也不送批准人；租户与录入者由操作者信封给。载荷里出现这些键会被服务端按未知键拒。
test('载荷里没有身份也没有摘要', () => {
  const text = JSON.stringify(pricePolicyPayloadOf(filled));
  for (const forbidden of ['tenant', 'submitter', 'approver', 'contentDigest', 'approval', 'canonicalization']) {
    ok(!text.includes(`"${forbidden}`), `载荷不该带 ${forbidden}`);
  }
});

// 公共半边把服务端点名、表单没认领的路径单列为「未认领」。表单自己能产出的每一条键都必须已认领，否则本册自己的格出了
// 问题会被显示成别处的问题；反向只许多认领一条节级路径 pricePolicy.caliber.fx（服务端对整节记问题的落点）。
test('表单认领的 JSON 路径覆盖载荷能产出的每一条键（含口径节与汇率节）', () => {
  const payload = pricePolicyPayloadOf({ ...filled, effectiveEndsAt: '2027-01-01' });
  const produced: string[] = [];
  const walk = (value: unknown, prefix: string) => {
    if (value !== null && typeof value === 'object') {
      for (const [key, nested] of Object.entries(value as Record<string, unknown>)) walk(nested, prefix ? `${prefix}.${key}` : key);
    } else {
      produced.push(prefix);
    }
  };
  walk(payload, '');
  for (const path of produced) {
    ok(pricePolicyFieldPaths.includes(path), `路径 ${path} 未被表单认领`);
  }
  const nodeLevel = ['pricePolicy.caliber.fx'];
  for (const path of pricePolicyFieldPaths) {
    ok(produced.includes(path) || nodeLevel.includes(path), `认领了载荷产不出的路径 ${path}`);
  }
});

test('「从版本壳带入」只抄范围与区间两样，其余各格不动', () => {
  const copied = withShellCopiedIntoPolicy({
    ...filled,
    effectiveEndsAt: '2027-01-01',
    policyScope: 'stale',
    policyEffectiveStartsAt: 'stale',
    policyEffectiveEndsAt: 'stale',
  });
  equal(copied.policyScope, 'SYN-SCOPE-01');
  equal(copied.policyEffectiveStartsAt, '2026-10-01');
  equal(copied.policyEffectiveEndsAt, '2027-01-01');
  equal(copied.direction, 'SELL');
  equal(copied.pricingPlan, 'PLAN-CN-SG-SELL@v1');
  equal(copied.taxClassification, 'vat-standard');
});

test('空草稿十八格全空', () => {
  const draft = emptyPricePolicyDraft();
  equal(Object.keys(draft).length, 18);
  for (const [key, value] of Object.entries(draft)) equal(value, '', `${key} 不为空`);
});
