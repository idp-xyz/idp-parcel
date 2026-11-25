import type { GuidedFieldSpec, GuidedFormDefinition, GuidedFormValues } from '../../components/guided-form/guided-forms';
import type { ExceptionDecisionKind } from './decisions';

const text = (path: string, label: string, extra: Partial<GuidedFieldSpec> = {}): GuidedFieldSpec => ({
  path,
  label,
  kind: 'text',
  required: true,
  ...extra,
});

const signal: GuidedFieldSpec[] = [
  text('parcel', '包裹号'),
  text('signalKind', '异常信号类型'),
  text('episodeId', '哪一期信号'),
];

const forms: Record<ExceptionDecisionKind, GuidedFormDefinition> = {
  'case-take-up': { fields: [text('caseId', '案件')] },
  'case-closure': { fields: [text('caseId', '案件'), text('conclusion', '结论')] },
  'case-merge': { fields: [text('caseId', '要并走的案件'), text('mainCaseId', '主案件')] },
  'evidence-submission': { fields: [text('provider', '交来材料的一方'), text('contentDigest', '内容指纹')] },
  'evidence-appraisal': {
    fields: [
      text('evidenceId', '证据项'),
      {
        path: 'appraisal',
        label: '评价',
        kind: 'select',
        required: true,
        options: [
          { value: 'CREDITED', label: '采信' },
          { value: 'DISCREDITED', label: '不采信' },
        ],
      },
      text('basis', '评价依据'),
    ],
  },
  'evidence-disclosure-preparation': {
    fields: [text('evidenceId', '证据项'), text('redactedDigest', '脱敏后的指纹'), text('scope', '这份版本给谁看')],
  },
  'customer-disclosure-decision': { fields: signal },
  'customer-disclosure-authorization': { fields: signal },
  'customer-exception-notification': { fields: signal },
  'customer-exception-notification-manual-record': {
    fields: [...signal, text('method', '通知方式', { hint: '如电话、邮件' }), text('original', '所凭原件')],
  },
};

export function exceptionDecisionForm(kind: ExceptionDecisionKind): GuidedFormDefinition {
  return forms[kind];
}

/**
 * 选中的案件带进接单与关闭。归并不带：选中的这一件是要并走的还是主案件，页面不知道。披露和通知要的是信号期，
 * 案件行上没有。
 */
export function exceptionDecisionPrefill(caseId: string | null): Partial<Record<ExceptionDecisionKind, GuidedFormValues>> {
  return caseId ? { 'case-take-up': { caseId }, 'case-closure': { caseId } } : {};
}
