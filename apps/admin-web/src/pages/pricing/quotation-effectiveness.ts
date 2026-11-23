import type { QuotationSendDetailBody } from './quotation-api';

export function quotationPublicationRequirementText(requirement: string | undefined): string {
  switch (requirement) {
    case 'REQUIRED':
      return '发布以接受为前提：要求';
    case 'NOT_REQUIRED':
      return '发布以接受为前提：不要求';
    default:
      return '发布以接受为前提：未配置';
  }
}

export function quotationEffectivenessLines(detail: Pick<QuotationSendDetailBody, 'priceOnRegister' | 'selectsNow' | 'scopeNamed' | 'validAcceptance' | 'recordedPublicationBasis'>): string[] {
  return [
    detail.priceOnRegister ? '所引价格内容已在册。' : '所引价格内容还没在册。',
    detail.selectsNow ? '这一版此刻仍在适用期内。' : '这一版此刻不在适用期内，或已被后继截止、退役。',
    detail.scopeNamed ? '这位客户此刻适用的合同已经指名这个计价范围。' : '这位客户此刻适用的合同还没指名这个计价范围。',
    detail.validAcceptance ? '这条报价是一条有效接受。' : '这条报价还不是一条有效接受。',
    detail.recordedPublicationBasis ? '这条报价是所引价卡发布时记下的依据。' : '这条报价不是所引价卡发布时记下的依据。',
  ];
}

export function quotationNextStepText(step: string | undefined): string {
  switch (step) {
    case 'PUBLISH_CARD':
      return '合同已经指名这个计价范围。下一步是发布这张价卡。';
    case 'ALREADY_EFFECTIVE':
      return '价卡已在册，这一版此刻仍适用，合同已经指名这个范围。';
    case 'CANNOT_PUBLISH':
      return '所引草稿已撤下，不能发布。';
    case 'NOT_SELECTABLE':
      return '价卡已在册，合同已经指名这个范围，但这一版此刻不在适用期内，或已被后继截止、退役。';
    case 'PUBLISH_CONTRACT':
      return '合同还没指名这个计价范围。下一步是发布指名这个范围的新一版合同。新接受落在新版区间里才换上来源列表。这里不发布新合同。';
    default:
      return '合同还没指名这个计价范围。下一步是发布指名这个范围的新一版合同。新接受落在新版区间里才换上来源列表。这里不发布新合同。';
  }
}

/** 合同已指名，且这一版仍是没撤下的已批准草稿。已经在册的不再给发布。 */
export function quotationCanPublish(input: {
  lane: string;
  scopeNamed?: boolean;
  priceOnRegister?: boolean;
  referencedDraftWithdrawn?: boolean;
}): boolean {
  return input.lane === 'APPROVED_DRAFT' && input.scopeNamed === true && input.priceOnRegister !== true && input.referencedDraftWithdrawn !== true;
}

export const quotationPublicationRequirementOutcomeLabels: Record<string, string> = {
  RECORDED: '已登记。',
  ALREADY_RECORDED: '这条规则已经登记过同样的内容，没有再写一条。',
  CONTENT_CONFLICT: '已经登记过不同的内容，没有覆盖。',
};
