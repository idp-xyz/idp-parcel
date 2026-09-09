// 价格规则版本逐字段表单的纯逻辑（票 admin-write-faces/14；ADR-0101 决定八本册选形：逐字段表单，方案从价卡目录选，
// 口径作条件节）：草稿形状、封闭集选项、条件格显隐、草稿 → 载荷、表单认领的 JSON 路径。全部是纯函数，node:test 钉着；
// 组件 PricePolicyPublicationForm.tsx 只负责摆，五步由公共半边 PublicationDraftFlow 走。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。空字段、区间先后、引用在不在册、方向 × 方案方向 × 转换
// 的绑定矩阵、口径两条件格的在场规则，一律送上去让服务端答——预览口逐格 problems 回来挂到对应格旁，跨格的绑定矩阵以
// 成因一句回来。连「必填」都不拦，免得两处口径；没有一格编不进载荷类型（全是文本），所以不给 localProblems。
//
// **显隐是呈现，不是裁门。** 税务分类只在含税 / 未税时显、体积系数只在销售方向时显；隐掉的格草稿里的值**不清、照送**
// ——「隐了却传了」由服务端按 CHECK 同形的构造门答在那一格上，组件把隐格上的值与问题如实显出来，不替人静默丢。
//
// **口径随正文同笔登记。** 本表单送出的载荷永远带口径节（CONTEXT「商业价格规则必须声明含税、未税或税务不适用」），
// 不给「先发正文、回头补口径」的两步；汇率一节可缺——不涉及外币的政策没有汇率口径，三格全空即不送这一节，填了任一格
// 就整节送、缺的由服务端点名（三格同在同缺）。

import { commercialDirectionLabels, planBindingConversionLabels, taxDispositionLabels } from './presentation';
import { normalizeMoment } from './publication-form-shared';
import type {
  CommercialPublicationPayload,
  FxCaliberPayload,
  PricePolicyBodyPayload,
  PricePolicyCaliberPayload,
} from './publication-draft-api';

/**
 * 草稿：版本壳五格 + 政策正文七格 + 口径节（税务两格、体积一格、汇率三格），全是文本。壳上的范围与区间是**版本**的
 * （SaveVersion 逐列比对的项），正文里的是**政策**自己的（0010 的 policy_scope_ref 与区间），两样分开给。
 * 封闭集四格（方向、方案方向、转换、税务口径）空串即「未选」——不预选（伞票硬句），服务端按集合外点名。
 */
export interface PricePolicyDraft {
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  direction: string;
  pricingPlan: string;
  planDirection: string;
  conversion: string;
  policyScope: string;
  policyEffectiveStartsAt: string;
  policyEffectiveEndsAt: string;
  taxDisposition: string;
  taxClassification: string;
  volumetricFactor: string;
  fxQuoteType: string;
  fxAsOfSemantics: string;
  fxAsOfPolicyVersion: string;
}

export function emptyPricePolicyDraft(): PricePolicyDraft {
  return {
    objectId: '',
    version: '',
    scope: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    direction: '',
    pricingPlan: '',
    planDirection: '',
    conversion: '',
    policyScope: '',
    policyEffectiveStartsAt: '',
    policyEffectiveEndsAt: '',
    taxDisposition: '',
    taxClassification: '',
    volumetricFactor: '',
    fxQuoteType: '',
    fxAsOfSemantics: '',
    fxAsOfPolicyVersion: '',
  };
}

/** 封闭集一格的选项：码是送上去的原词，中文只显给人看（集外照原样示出，由服务端点名）。 */
export interface ClosedSetOption {
  value: string;
  label: string;
}

function optionsOf(table: Record<string, string>): ClosedSetOption[] {
  return Object.entries(table).map(([value, label]) => ({ value, label: `${value} · ${label}` }));
}

/** 价格方向三格（domain PriceDirection 的 String() 原词），中文与政策册读面同词。 */
export const priceDirectionOptions: readonly ClosedSetOption[] = optionsOf(commercialDirectionLabels);
/** 方案绑定转换两格（domain PlanBindingConversion）。 */
export const planBindingConversionOptions: readonly ClosedSetOption[] = optionsOf(planBindingConversionLabels);
/** 税务口径三格（domain TaxDisposition）。 */
export const taxDispositionOptions: readonly ClosedSetOption[] = optionsOf(taxDispositionLabels);

/**
 * 税务分类那一格显不显：只在含税 / 未税时显。**显隐是呈现**——不适用却填了分类照样送上去，由服务端答在这一格。
 * 未选税务口径时也显：操作者先填哪格由他定，服务端点名时这一格得在场。
 */
export function showsTaxClassification(draft: Pick<PricePolicyDraft, 'taxDisposition'>): boolean {
  return draft.taxDisposition !== 'TAX_NOT_APPLICABLE';
}

/**
 * 体积系数那一格显不显：只在销售方向时显（采购与法人间方向的系数在承运商价卡上，本上下文不得再声明一份）。未选方向时
 * 也显，理由同上。
 */
export function showsVolumetricFactor(draft: Pick<PricePolicyDraft, 'direction'>): boolean {
  return draft.direction === '' || draft.direction === 'SELL';
}

/**
 * 表单渲染的 JSON 路径（载荷键名原词）。公共半边拿它分「本表单的格」与「未认领的路径」：服务端点名的路径不在这张表里
 * 就单列出来，不静默丢。`pricePolicy.caliber.fx` 是汇率整节的路径（三格各自立得住却仍构造不出时服务端记在节上）。
 * 这张表与 pricePolicyPayloadOf 能产出的键一一对应（外加节级那一条），测试钉着。
 */
export const pricePolicyFieldPaths: readonly string[] = [
  'kind',
  'objectId',
  'version',
  'scope',
  'effectiveStartsAt',
  'effectiveEndsAt',
  'pricePolicy.direction',
  'pricePolicy.pricingPlan',
  'pricePolicy.planDirection',
  'pricePolicy.conversion',
  'pricePolicy.scope',
  'pricePolicy.effectiveStartsAt',
  'pricePolicy.effectiveEndsAt',
  'pricePolicy.caliber.taxDisposition',
  'pricePolicy.caliber.taxClassification',
  'pricePolicy.caliber.volumetricFactor',
  'pricePolicy.caliber.fx',
  'pricePolicy.caliber.fx.quoteType',
  'pricePolicy.caliber.fx.asOfSemantics',
  'pricePolicy.caliber.fx.asOfPolicyVersion',
];

/** 汇率三格是否有任一格填了：填了就整节送（缺的由服务端点名），全空即「不声明汇率口径」，整节缺席。 */
export function declaresFx(draft: Pick<PricePolicyDraft, 'fxQuoteType' | 'fxAsOfSemantics' | 'fxAsOfPolicyVersion'>): boolean {
  return draft.fxQuoteType.trim() !== '' || draft.fxAsOfSemantics.trim() !== '' || draft.fxAsOfPolicyVersion.trim() !== '';
}

/**
 * 草稿 → 产品定义的载荷。可缺的键缺席而不是空串：两个区间上界、税务分类、体积系数、汇率节——服务端按键在场与否分辨
 * 「没有」，空串会被当成填了空的值送进构造门。封闭集四格照原样送（含空串「未选」，服务端按集合外点名，不代填 NONE）。
 * 隐掉的条件格若草稿里有值**照送**（显隐是呈现不是裁门）。**载荷里没有身份也没有摘要**：租户与录入者由接入渠道的操作者
 * 信封给，摘要只有服务端算。
 */
export function pricePolicyPayloadOf(draft: PricePolicyDraft): CommercialPublicationPayload {
  const caliber: PricePolicyCaliberPayload = { taxDisposition: draft.taxDisposition.trim() };
  if (draft.taxClassification.trim() !== '') caliber.taxClassification = draft.taxClassification.trim();
  if (draft.volumetricFactor.trim() !== '') caliber.volumetricFactor = draft.volumetricFactor.trim();
  if (declaresFx(draft)) {
    const fx: FxCaliberPayload = {
      quoteType: draft.fxQuoteType.trim(),
      asOfSemantics: draft.fxAsOfSemantics.trim(),
      asOfPolicyVersion: draft.fxAsOfPolicyVersion.trim(),
    };
    caliber.fx = fx;
  }
  const body: PricePolicyBodyPayload = {
    direction: draft.direction.trim(),
    pricingPlan: draft.pricingPlan.trim(),
    planDirection: draft.planDirection.trim(),
    conversion: draft.conversion.trim(),
    scope: draft.policyScope.trim(),
    effectiveStartsAt: normalizeMoment(draft.policyEffectiveStartsAt),
    caliber,
  };
  if (draft.policyEffectiveEndsAt.trim() !== '') body.effectiveEndsAt = normalizeMoment(draft.policyEffectiveEndsAt);

  const payload: CommercialPublicationPayload = {
    kind: 'PRICE_RULE',
    objectId: draft.objectId.trim(),
    version: draft.version.trim(),
    scope: draft.scope.trim(),
    effectiveStartsAt: normalizeMoment(draft.effectiveStartsAt),
    pricePolicy: body,
  };
  if (draft.effectiveEndsAt.trim() !== '') payload.effectiveEndsAt = normalizeMoment(draft.effectiveEndsAt);
  return payload;
}

/** 「从版本壳带入」：把壳上的适用范围与区间抄进政策正文那三格，其余不动。抄的是文本，仍由服务端逐格判；不是默认。 */
export function withShellCopiedIntoPolicy(draft: PricePolicyDraft): PricePolicyDraft {
  return {
    ...draft,
    policyScope: draft.scope,
    policyEffectiveStartsAt: draft.effectiveStartsAt,
    policyEffectiveEndsAt: draft.effectiveEndsAt,
  };
}
