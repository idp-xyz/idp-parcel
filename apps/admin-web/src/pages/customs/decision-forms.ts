import type { GuidedFieldSpec, GuidedFormDefinition, GuidedFormValues } from '../../components/guided-form/guided-forms';
import type { CustomsDecisionKind } from './decisions';

const text = (path: string, label: string, extra: Partial<GuidedFieldSpec> = {}): GuidedFieldSpec => ({
  path,
  label,
  kind: 'text',
  required: true,
  ...extra,
});

const direction = (path: string, label: string): GuidedFieldSpec => ({
  path,
  label,
  kind: 'select',
  required: true,
  options: [
    { value: 'IMPORT', label: '进口' },
    { value: 'EXPORT', label: '出口' },
  ],
});

const guardedActions = [
  { value: 'OUTBOUND_RELEASE', label: '出库' },
  { value: 'LOADING_DEPARTURE', label: '装载出发' },
  { value: 'CROSS_CUSTOMS_MOVEMENT', label: '跨关区移动' },
  { value: 'FINAL_DELIVERY', label: '交付' },
] as const;

const gateAction: GuidedFieldSpec = {
  path: 'action',
  label: '检查哪一步',
  kind: 'select',
  required: true,
  options: guardedActions,
};

const resultLayer: GuidedFieldSpec = {
  path: 'layer',
  label: '结果层',
  kind: 'select',
  required: true,
  options: [
    { value: 'REGULATORY_RECEIPT', label: '海关已接收' },
    { value: 'BUSINESS_ACCEPTANCE', label: '海关已受理' },
    { value: 'PROCESS_DECISION', label: '过程决定' },
    { value: 'ASSESSED_DUTY', label: '核定税费' },
    { value: 'RELEASE_RESULT', label: '放行结果' },
    { value: 'DISPOSITION_DECISION', label: '处置决定' },
  ],
};

const releaseKind: GuidedFieldSpec = {
  path: 'release.kind',
  label: '放行种类',
  kind: 'select',
  options: [
    { value: 'FULL', label: '完整放行' },
    { value: 'PARTIAL', label: '部分放行' },
    { value: 'CONDITIONAL', label: '附条件放行' },
  ],
};

function form(fields: readonly GuidedFieldSpec[], validate?: GuidedFormDefinition['validate']): GuidedFormDefinition {
  return validate ? { fields, validate } : { fields };
}

const forms: Record<CustomsDecisionKind, GuidedFormDefinition> = {
  'case-establishment': form([
    text('jurisdiction', '辖区'),
    direction('direction', '方向'),
    text('procedure', '监管程序'),
    text('obligation', '关务义务'),
    { path: 'parcels', label: '包裹号', kind: 'string-list', required: true, hint: '一行一个包裹号' },
    {
      path: 'roles',
      label: '参与方角色',
      kind: 'rows',
      required: true,
      rowLabel: '角色',
      rowFields: [text('role', '角色'), text('party', '参与方'), text('authority', '权限')],
    },
  ]),
  'submission-authorization': form([text('unitId', '报关批次')]),
  'declaration-submission': form([
    text('unitId', '报关批次'),
    text('caseId', '清关单'),
    text('procedure', '监管程序'),
    { path: 'members', label: '成员包裹号', kind: 'string-list', required: true },
    text('dossier', '申报资料快照引用'),
    text('roles', '角色快照引用'),
    text('target', '申报通道'),
  ]),
  'release-gate-verification': form([text('scope', '申报范围（报关批次）'), gateAction, text('boundary', '边界（监管程序）')]),
  'case-closure': form([
    text('caseId', '清关单'),
    { path: 'cutoffAt', label: '截点', kind: 'datetime', required: true },
  ]),
  'manual-lodgement': form([
    text('version', '提交版本'),
    text('original', '递交凭据'),
    { path: 'lodgedAt', label: '实际递交时间', kind: 'datetime', hint: '不填则取现在' },
    text('reference', '受理号', { required: false }),
  ]),
  'external-result-entry': form(
    [
      text('sourceId', '结果标识', { hint: '同一份回执只用一个；系统会加上 MANUAL/' }),
      resultLayer,
      text('role', '发出机构角色'),
      text('rawSemantics', '回执原文'),
      text('claimedVersion', '对应申报版本'),
      { path: 'attempt', label: '第几次提交', kind: 'integer', required: true },
      text('scope', '申报范围（报关批次）'),
      { path: 'occurredAt', label: '回执上的业务时间', kind: 'datetime', required: true },
      text('original', '所凭原件'),
      text('correctsSourceId', '更正哪一条', { required: false, hint: '更正人工录入的结果时带上 MANUAL/' }),
      releaseKind,
      text('release.authority', '放行机构', { required: false }),
      text('release.condition', '放行条件', { required: false, hint: '附条件放行时填写' }),
    ],
    // 放行三格与结果层对不上时不替人改：把放行内容悄悄去掉再交，一份放行回执就会被记成低一层的结果，而结果不可改写。
    (_values, payload) => {
      const release = payload.release as { kind?: string; authority?: string; condition?: string } | undefined;
      if (payload.layer !== 'RELEASE_RESULT') {
        return release
          ? '结果层不是放行结果，放行种类、放行机构与放行条件要留空；这份是放行回执的话，请把结果层改成放行结果'
          : null;
      }
      if (!release?.kind || !release.authority) return '放行结果要填放行种类和放行机构';
      if (release.kind === 'CONDITIONAL' && !release.condition) return '附条件放行要填写条件';
      if (release.kind === 'FULL' && release.condition) return '完整放行不带放行条件';
      return null;
    },
  ),
  'manifest-reference-entry': form([
    text('manifestId', '舱单号'),
    text('version', '版本'),
    text('procedure', '监管程序'),
    direction('direction', '方向'),
    text('scope', '范围'),
    text('carrier', '责任承运商'),
    text('original', '所凭原件'),
  ]),
  'manifest-revision-entry': form([
    text('manifestId', '舱单号'),
    text('version', '新版本'),
    text('scope', '新范围'),
    text('carrier', '责任承运商'),
    text('original', '所凭原件'),
  ]),
  'regulatory-restriction': form([
    text('restrictionId', '限制标识'),
    text('decision', '依据的海关结果'),
    { path: 'constrains', label: '限制哪几步', kind: 'choices', required: true, options: guardedActions },
  ]),
  'regulatory-restriction-release': form([text('restrictionId', '要解除的限制'), text('result', '依据的放行结果')]),
};

export function customsDecisionForm(kind: CustomsDecisionKind): GuidedFormDefinition {
  return forms[kind];
}

export type CustomsDecisionPrefill = Partial<Record<CustomsDecisionKind, GuidedFormValues>>;

/** 只留行上的原始标识：空的、占位的横线、几项并成一格的都不带。 */
function known(values: Record<string, string | undefined>): GuidedFormValues {
  const out: GuidedFormValues = {};
  for (const [path, value] of Object.entries(values)) {
    if (value && value !== '—' && !value.includes('\n')) out[path] = value;
  }
  return out;
}

/**
 * 清关列表的一行给哪一口带哪几格。按口分开写，不按格名合并：「版本」在线下递交上是申报版本，在舱单上是舱单版本。
 * 只带指名已有单据的格。新版本号、更正对象这类要人判断的格不带——带了就会被原样交出去。
 * 申报范围与边界：放行检查与海关结果按报关批次与它的监管程序判。
 */
export function customsDecisionPrefill(row: { key: string; values: Record<string, string> }): CustomsDecisionPrefill {
  const v = row.values;
  switch (row.key.slice(0, row.key.indexOf(':'))) {
    case 'case':
      return {
        'declaration-submission': known({ caseId: v.id }),
        'case-closure': known({ caseId: v.id }),
      };
    case 'unit':
      return {
        'submission-authorization': known({ unitId: v.id }),
        'declaration-submission': known({ unitId: v.id, caseId: v.caseId, procedure: v.procedure }),
        'external-result-entry': known({ scope: v.id }),
        'release-gate-verification': known({ scope: v.id, boundary: v.procedure }),
      };
    case 'snapshot':
      return {
        'declaration-submission': known({ unitId: v.unit, procedure: v.procedure, dossier: v.dossier, roles: v.roles }),
        'manual-lodgement': known({ version: v.version }),
      };
    case 'submission':
      return {
        'manual-lodgement': known({ version: v.version }),
        'external-result-entry': known({ claimedVersion: v.version, scope: v.unit }),
      };
    case 'manifest':
      return { 'manifest-revision-entry': known({ manifestId: v.manifest, carrier: v.carrier }) };
    default:
      return {};
  }
}
