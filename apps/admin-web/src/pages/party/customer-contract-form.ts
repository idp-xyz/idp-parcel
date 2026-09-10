// 客户合同发布表单的纯逻辑（票 admin-write-faces/10）：草稿形状、草稿 → 载荷、表单认领的 JSON 路径。
// 全部是纯函数，node:test 钉着；组件 CustomerContractPublicationForm.tsx 只负责摆。写法照
// pricing/series-form.ts 与 publication-draft-flow.ts。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句；票 10「恰一与『不适用必带依据』由服务端裁」）。
// 二选一控件（行的「指名策略 / 显式不适用」、合同级的「要求 / 不适用」）只决定把哪一格放进载荷；没选、
// 选了没填，照样送上去——答回来的是构造门对那一行 / 那一节的拒绝，不是表单替人补的一句。本册的载荷全是
// 字符串格，没有「编不进 JSON 类型」的格，所以也没有 localProblems。
//
// 第三层：合同层交付条件一节（票 admin-write-faces/25，ADR-0133 决定四），纯逻辑在 delivery-condition-section.ts 与产品
// 版本表单共用；一格都没填即整节缺席（这一版没有合同层声明），填了任一格整节送、tightens 两格空着也送由服务端点名。
// 「只能收紧」由服务端写口答，预览看不出、发布那一步才拒——表单不自判。

import {
  deliveryConditionDeclared,
  deliveryConditionFieldPaths,
  deliveryConditionPayloadOf,
  deliveryConditionRenderedPaths,
  emptyDeliveryConditionDraft,
  type DeliveryConditionDraft,
} from './delivery-condition-section';
import type {
  CommercialPublicationPayload,
  ControlBindingPayload,
  PreAcceptanceControlPayload,
} from './publication-draft-api';

/** 行的二选一：指名一份接受前财务控制策略，或显式不适用并写依据。`''` = 还没选。 */
export type BindingMode = '' | 'policy' | 'inapplicable';

export interface ControlBindingDraft {
  chargeScope: string;
  mode: BindingMode;
  policy: string;
  inapplicabilityBasis: string;
}

/** 合同级「要不要接受前财务控制」（Go `PreAcceptanceControlRequirement` 的 String() 原词）。`''` = 还没选。 */
export type ControlRequirementDraft = '' | 'REQUIRED' | 'NOT_APPLICABLE';

export interface CustomerContractDraft {
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  /** 接单规则包对象标识：同时进壳上的指名引用与正文，两处必须相等由服务端核（ErrRulePackageReferenceMismatch）。 */
  rulePackage: string;
  bindings: ControlBindingDraft[];
  controlRequirement: ControlRequirementDraft;
  controlNotApplicableBasis: string;
  /** 合同层交付条件一节（票 25）；一格都没填即整节不进载荷。 */
  deliveryConditions: DeliveryConditionDraft;
}

/** 第三层在载荷里的根：这一节的各格路径都挂在它下面。 */
export const customerContractDeliveryConditionsPath = 'customerContract.deliveryConditions';

export function emptyBindingDraft(): ControlBindingDraft {
  return { chargeScope: '', mode: '', policy: '', inapplicabilityBasis: '' };
}

export function emptyCustomerContractDraft(): CustomerContractDraft {
  return {
    objectId: '',
    version: '',
    scope: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    rulePackage: '',
    bindings: [emptyBindingDraft()],
    controlRequirement: '',
    controlNotApplicableBasis: '',
    deliveryConditions: emptyDeliveryConditionDraft(),
  };
}

// 一行只送被选的那一格：另一格是操作者切换模式前留下的旧值，送上去会让领域门拒一个页面上看不见的东西。
// 没选模式的行两格都不送——那一行的拒绝由服务端点名 bindings[i]。
function bindingPayloadOf(binding: ControlBindingDraft): ControlBindingPayload {
  const row: ControlBindingPayload = { chargeScope: binding.chargeScope };
  if (binding.mode === 'policy' && binding.policy !== '') row.policy = binding.policy;
  if (binding.mode === 'inapplicable' && binding.inapplicabilityBasis !== '') {
    row.inapplicabilityBasis = binding.inapplicabilityBasis;
  }
  return row;
}

// 合同级声明一节永远在场：没选要求就送空要求——「本版不作合同级声明」在受控批文里是合法的缺键，但在运营
// 主路径上它不是表单可以替人默认的一格，让服务端答「集合外的控制要求」把选择交回操作者。依据只随
// `不适用`送，理由同上面行的那一格。
function controlPayloadOf(draft: CustomerContractDraft): PreAcceptanceControlPayload {
  const control: PreAcceptanceControlPayload = { requirement: draft.controlRequirement };
  if (draft.controlRequirement === 'NOT_APPLICABLE' && draft.controlNotApplicableBasis !== '') {
    control.notApplicableBasis = draft.controlNotApplicableBasis;
  }
  return control;
}

/** 草稿 → 载荷。可缺的格留空即不进载荷（Go 侧 omitempty 同义）；载荷里没有身份也没有摘要。 */
export function payloadOf(draft: CustomerContractDraft): CommercialPublicationPayload {
  const payload: CommercialPublicationPayload = {
    kind: 'CUSTOMER_CONTRACT',
    objectId: draft.objectId,
    version: draft.version,
    scope: draft.scope,
    effectiveStartsAt: draft.effectiveStartsAt,
    customerContract: {
      contractContent: { rulePackage: draft.rulePackage },
      preAcceptanceControl: controlPayloadOf(draft),
    },
  };
  if (draft.effectiveEndsAt !== '') payload.effectiveEndsAt = draft.effectiveEndsAt;
  if (draft.rulePackage !== '') payload.references = { ACCEPTANCE_RULE_PACKAGE: draft.rulePackage };
  if (draft.bindings.length > 0) {
    payload.customerContract!.contractContent.bindings = draft.bindings.map(bindingPayloadOf);
  }
  if (deliveryConditionDeclared(draft.deliveryConditions, 'contract')) {
    payload.customerContract!.deliveryConditions = deliveryConditionPayloadOf(draft.deliveryConditions, 'contract');
  }
  return payload;
}

const shellPaths = ['objectId', 'version', 'scope', 'effectiveStartsAt', 'effectiveEndsAt'] as const;

/**
 * 表单渲染了哪几条 JSON 路径（流程组件把服务端点名的其余路径单独列出）。约定行按行数长：服务端对整行
 * （恰一）与行里每一格都可能点名。壳上的规则包引用与正文里的规则包在页面上是同一格。
 */
export function customerContractFieldPaths(draft: CustomerContractDraft): string[] {
  const paths: string[] = [
    ...shellPaths,
    'references.ACCEPTANCE_RULE_PACKAGE',
    'customerContract.contractContent',
    'customerContract.contractContent.rulePackage',
    'customerContract.preAcceptanceControl.requirement',
    'customerContract.preAcceptanceControl.notApplicableBasis',
  ];
  draft.bindings.forEach((_, index) => {
    const row = `customerContract.contractContent.bindings[${index}]`;
    paths.push(row, `${row}.chargeScope`, `${row}.policy`, `${row}.inapplicabilityBasis`);
  });
  paths.push(...deliveryConditionFieldPaths(customerContractDeliveryConditionsPath, draft.deliveryConditions, 'contract'));
  return paths;
}

/**
 * 组件里显 Problems 的路径表（票 22 判据 3），按 CustomerContractPublicationForm 的 JSX 逐处抄：壳五格各一 Field；规则包
 * 一格连带壳上引用与正文根（alsoPaths）；合同级声明的「要求」一格，依据格隐着时由它代显依据格的问题（alsoPaths）；
 * 每条约定行 RowFrame 显行本身，费用范围一格，「指名策略 / 不适用依据」两格只显一格、隐着的那格的问题由显着的代显
 * （alsoPaths）——所以隐显都不影响这张表；交付条件一节照 DeliveryConditionFields 的 JSX。与上面的认领表由
 * publication-form-rendered-paths.test.ts 比对。
 */
export function customerContractRenderedPaths(draft: CustomerContractDraft): string[] {
  const paths: string[] = [
    ...shellPaths,
    'customerContract.contractContent.rulePackage',
    'references.ACCEPTANCE_RULE_PACKAGE',
    'customerContract.contractContent',
    'customerContract.preAcceptanceControl.requirement',
    'customerContract.preAcceptanceControl.notApplicableBasis',
  ];
  draft.bindings.forEach((_, index) => {
    const row = `customerContract.contractContent.bindings[${index}]`;
    paths.push(row, `${row}.chargeScope`, `${row}.policy`, `${row}.inapplicabilityBasis`);
  });
  paths.push(...deliveryConditionRenderedPaths(customerContractDeliveryConditionsPath, draft.deliveryConditions, 'contract'));
  return paths;
}
