import type { GuidedFieldSpec, GuidedFormDefinition, GuidedFormValues } from '../../components/guided-form/guided-forms';
import type { SettlementDecisionKind } from './decisions';

const text = (path: string, label: string, extra: Partial<GuidedFieldSpec> = {}): GuidedFieldSpec => ({
  path,
  label,
  kind: 'text',
  required: true,
  ...extra,
});

const minor = (path: string, label: string): GuidedFieldSpec =>
  text(path, label, { hint: '最小货币单位的整数串，例如 96.00 元填 9600' });

function minorText(value: unknown, label: string): string | null {
  if (typeof value !== 'string' || !/^\d+$/.test(value)) return `${label}请填最小货币单位的整数串`;
  return null;
}

const forms: Record<SettlementDecisionKind, GuidedFormDefinition> = {
  'charge-confirmation': { fields: [text('charge', '费用')] },
  'charge-adjustment': {
    fields: [
      text('adjustment', '调整标识'),
      text('charge', '已确认的费用'),
      {
        path: 'kind',
        label: '种类',
        kind: 'select',
        required: true,
        options: [
          { value: 'PRICING_CORRECTION', label: '计价纠错' },
          { value: 'COMMERCIAL_CONCESSION', label: '商业让利' },
        ],
      },
      {
        path: 'direction',
        label: '方向',
        kind: 'select',
        required: true,
        options: [
          { value: 'DEBIT', label: '加收' },
          { value: 'CREDIT', label: '减收' },
        ],
      },
      text('evaluation', '新评价引用', { required: false, hint: '计价纠错时必填' }),
      text('authorization', '授权引用', { required: false, hint: '商业让利时必填' }),
      text('originalCurrency', '原币'),
      minor('originalMinor', '原币金额'),
      text('settlementCurrency', '结算币种'),
      minor('settlementMinor', '结算金额'),
      text('conversion', '折算引用', { required: false, hint: '原币与结算币不同时填写' }),
    ],
    validate: (_values, payload) => {
      const original = minorText(payload.originalMinor, '原币金额');
      if (original) return original;
      const settlement = minorText(payload.settlementMinor, '结算金额');
      if (settlement) return settlement;
      // 两种依据有此无彼：换过种类之后上一种的引用还留在格里，照交会被拒而答复不指是哪一格。
      if (payload.kind === 'PRICING_CORRECTION') {
        if (!payload.evaluation) return '计价纠错要填写新评价引用';
        if (payload.authorization) return '计价纠错不带授权引用，请把那一格清空';
      }
      if (payload.kind === 'COMMERCIAL_CONCESSION') {
        if (!payload.authorization) return '商业让利要填写授权引用';
        if (payload.evaluation) return '商业让利不带新评价引用，请把那一格清空';
      }
      return null;
    },
  },
  'statement-publication': {
    fields: [
      text('account', '结算账户'),
      text('period', '账期'),
      text('version', '版本', { placeholder: 'v1' }),
      text('currency', '币种'),
      { path: 'charges', label: '费用', kind: 'string-list', hint: '勾上带走调整时可以不列' },
      { path: 'lastAttributionDate', label: '本期最后归属日', kind: 'date', required: true },
      minor('declaredTotal', '申明总额'),
      { path: 'cutOffAt', label: '截单时间', kind: 'datetime', required: true },
      {
        path: 'carryUnbilledAdjustments',
        label: '把还没出账的调整随这张单带走',
        kind: 'boolean',
        omitWhenFalse: true,
        hint: '勾上之后申明总额只报费用，费用可以一笔都不列',
      },
    ],
    validate: (_values, payload) => {
      const total = minorText(payload.declaredTotal, '申明总额');
      if (total) return total;
      if (!payload.carryUnbilledAdjustments && !payload.charges) return '请至少填写一笔费用';
      return null;
    },
  },
  'statement-void': { fields: [text('statement', '对账单号'), text('basis', '作废依据')] },
  'late-charge-inclusion': {
    fields: [
      text('inclusion', '纳入标识'),
      text('statement', '原对账单号'),
      text('charge', '后到的已确认费用'),
      text('subsequentPeriod', '后续账期'),
    ],
  },
  'adjustment-inclusion': {
    fields: [
      text('inclusion', '纳入标识'),
      text('statement', '原对账单号'),
      text('adjustment', '已登记的调整'),
      text('subsequentPeriod', '后续账期'),
    ],
  },
};

export function settlementDecisionForm(kind: SettlementDecisionKind): GuidedFormDefinition {
  return forms[kind];
}

type SettlementDecisionPrefill = Partial<Record<SettlementDecisionKind, GuidedFormValues>>;

/** 点中的客户费用带进确认与调整两口。 */
export function chargeDecisionPrefill(charge: string | null): SettlementDecisionPrefill {
  return charge ? { 'charge-confirmation': { charge }, 'charge-adjustment': { charge } } : {};
}

/** 点中的对账单带进作废与两种纳入。发布不带：发布形成的是新单。 */
export function statementDecisionPrefill(statement: string | null): SettlementDecisionPrefill {
  return statement
    ? { 'statement-void': { statement }, 'late-charge-inclusion': { statement }, 'adjustment-inclusion': { statement } }
    : {};
}
