// 交付条件一节的纯逻辑（票 admin-write-faces/25；ADR-0133 决定四）：草稿形状、「这一节声明了没有」、草稿 → 载荷、认领与渲染
// 的 JSON 路径。产品版本表单与客户合同表单各挂一节、共用这一份——同一节在两张表单里各抄一份，就是 awf/13 / 18 评审点过的
// 那种私有副本。全部是纯函数，node:test 钉着；组件 DeliveryConditionFields.tsx 只负责摆。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。三格都是开放引用（PAR-NET-09 / PAR-COM-05 / 06 实例半边）：
// 方式是多行文本一行一项（照 customer-service-rule-form 的材料清单选形——正文里方式是「集合」不是「行」），两条规则引用
// 手填；合同层的 tightens 两格也手填——壳上 `references.SERVICE_PRODUCT` 只有对象没有版本号，不从它推（pc-gaps/11 判断题 2）。
// 零方式、同方式两行、层与 tightens 的配对、「只能收紧」一律送上去由服务端答；本文件没有 localProblems。
//
// **一节可缺**（照 acceptance-rule-package-form 的 sectionDeclared）：一格都没填即整节缺席——这一版没有交付条件，不声明；
// 填了任一格整节原样送，空格照送由服务端逐格点名。不预开、不预选、不给任何方式的候选。

import type { DeliveryConditionPayload } from './publication-draft-api';

/** 哪一册的正文格挂这一节：层由册定，合同层多 tightens 两格。 */
export type DeliveryConditionLayer = 'product' | 'contract';

export interface DeliveryConditionDraft {
  /** 允许的交付方式，一行一项；空行不算项。 */
  methodsText: string;
  recipientScopeRule: string;
  proofOfDeliveryRule: string;
  /** 所收紧的服务产品版本，只在合同层用；产品层表单不摆这两格。 */
  tightensObjectId: string;
  tightensVersion: string;
}

export function emptyDeliveryConditionDraft(): DeliveryConditionDraft {
  return { methodsText: '', recipientScopeRule: '', proofOfDeliveryRule: '', tightensObjectId: '', tightensVersion: '' };
}

/** 多行文本 → 一项一串：按行切、去首尾空白、空行不是项。读一种输入格式，不裁门——重复的项与整张空由服务端答。 */
export function methodLinesOf(text: string): string[] {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line !== '');
}

/**
 * 这一节声明了没有：产品层看三格，合同层再看 tightens 两格。一格都没填就是「这一版不声明交付条件」，整节不进载荷；
 * 填了任一格整节都进——空格由服务端点名，表单不替操作者判「这算不算填了」。
 */
export function deliveryConditionDeclared(draft: DeliveryConditionDraft, layer: DeliveryConditionLayer): boolean {
  const fields = [draft.methodsText, draft.recipientScopeRule, draft.proofOfDeliveryRule];
  if (layer === 'contract') fields.push(draft.tightensObjectId, draft.tightensVersion);
  return fields.some((value) => value.trim() !== '');
}

/**
 * 草稿 → 载荷一节（调用方已按 deliveryConditionDeclared 决定要不要带）。合同层 tightens 两格**永远送**：空着也送，让服务端
 * 点名 `tightens.objectId` / `tightens.version`；不送才是缺键，那一格的问题就会落成一句不指格的「未受理」。产品层不带 tightens
 * 键——产品层带了它是类别错误，由服务端答。
 */
export function deliveryConditionPayloadOf(draft: DeliveryConditionDraft, layer: DeliveryConditionLayer): DeliveryConditionPayload {
  const payload: DeliveryConditionPayload = {
    methods: methodLinesOf(draft.methodsText),
    recipientScopeRule: draft.recipientScopeRule.trim(),
    proofOfDeliveryRule: draft.proofOfDeliveryRule.trim(),
  };
  if (layer === 'contract') {
    payload.tightens = { objectId: draft.tightensObjectId.trim(), version: draft.tightensVersion.trim() };
  }
  return payload;
}

/**
 * 这一节认领的 JSON 路径（服务端逐格问题按它点名）：节根、方式每一项（methods[i]）、两条规则引用；合同层再加 tightens 根与两格。
 * 方式只认领到项，不认领 `methods` 这一键本身：服务端对整张集合（零项、重复）的话是预览上的未受理成因，不落在键上。
 */
export function deliveryConditionFieldPaths(root: string, draft: DeliveryConditionDraft, layer: DeliveryConditionLayer): string[] {
  const paths = [root, `${root}.recipientScopeRule`, `${root}.proofOfDeliveryRule`];
  methodLinesOf(draft.methodsText).forEach((_, index) => paths.push(`${root}.methods[${index}]`));
  if (layer === 'contract') paths.push(`${root}.tightens`, `${root}.tightens.objectId`, `${root}.tightens.version`);
  return paths;
}

/**
 * 组件里显 Problems 的路径表（票 22 判据 3），按 DeliveryConditionFields 的 JSX 逐处抄：节根一处 Problems；两条规则引用各一
 * Field；方式文本框下按项号汇显 methods[i]；合同层 tightens 两格各一 Field、根由对象标识那格代显（alsoPaths）。与上面的认领表
 * 由 publication-form-rendered-paths.test.ts 比对——改 JSX 里的 path 要同步改这里。
 */
export function deliveryConditionRenderedPaths(root: string, draft: DeliveryConditionDraft, layer: DeliveryConditionLayer): string[] {
  return deliveryConditionFieldPaths(root, draft, layer);
}
