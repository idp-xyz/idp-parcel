// 设置区「追踪规则」「通知与披露」两页的界面口径与行模型（改版第二轮）。纯函数，node:test 钉着；两页只负责摆。
//
// 六本册在领域里叫里程碑映射、分诊规则、冲突信号规则、通知策略、披露策略、异常披露规则（CONTEXT 原词，代码与登记口
// 照旧用）；配置员认的是它们**管什么事**：动态归成哪个状态、异常怎么分流、什么时候通知客户、客户能看到什么。所以签名、
// 列名与提示都按管的事说，册名原词与版本号、批准人、原码一起进右栏「审计」与登记签的「技术详情」。
//
// 本文件收的是这两页自己的说法；presentation.ts 里同几本册的签词（visibilityCatalogueKindLabels 等）仍给索赔前置页与
// 登记答复用，不在这里改它——那一页另有作者在改，同一份表两处动会在合并时互相顶掉。
//
// 码一律经用语词典（vocabulary/status.ts）译成中文：事实类型、里程碑、信号类型、可信度、渠道、义务判据都是租户登记的
// 开放引用，词典收的是参考译法，查不到的码原样显示并在开发模式告警一次。

import type { TechnicalRow } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import { formatInstant } from '../moment';
import type {
  DisclosureCellRecord,
  VisibilityCatalogueKind,
  VisibilityCatalogueListResponseBody,
} from './catalogue-api';

/**
 * 动态来自哪个环节（源上下文封闭五元）。词按运营认的环节说：托运是「运单」、节点是「网点」、关务是「清关」；
 * 物流动态页的 sourceContextLabels 仍是那一页的说法，不在这里改它。
 */
export const stageLabels: Record<string, string> = {
  PARCEL_SHIPMENT: '运单',
  NETWORK_ROUTING: '路线',
  NODE_OPERATIONS: '网点',
  TRANSPORT_FULFILLMENT: '运输',
  CUSTOMS_COMPLIANCE: '清关',
};

export type JudgmentKind = 'MILESTONE_MAPPING' | 'TRIAGE_RULE' | 'CONFLICT_SIGNAL_RULE';
export type DisclosureKind = 'NOTIFICATION_POLICY' | 'DISCLOSURE_POLICY' | 'EXCEPTION_DISCLOSURE_RULE';
export type RuleBookKind = JudgmentKind | DisclosureKind;

export const judgmentKinds: readonly JudgmentKind[] = ['MILESTONE_MAPPING', 'TRIAGE_RULE', 'CONFLICT_SIGNAL_RULE'];
export const disclosureKinds: readonly DisclosureKind[] = [
  'NOTIFICATION_POLICY',
  'DISCLOSURE_POLICY',
  'EXCEPTION_DISCLOSURE_RULE',
];

/**
 * 签名：说这本册管什么事。「对客户显示」（披露策略）与「异常告知」（异常披露规则）是相邻两本册（0012 / 0023），
 * 签名各说各的事，不缩成同一个词。
 */
export const ruleBookLabels: Record<RuleBookKind, string> = {
  MILESTONE_MAPPING: '状态归类',
  TRIAGE_RULE: '异常分流',
  CONFLICT_SIGNAL_RULE: '动态矛盾',
  NOTIFICATION_POLICY: '通知时限',
  DISCLOSURE_POLICY: '对客户显示',
  EXCEPTION_DISCLOSURE_RULE: '异常告知',
};

/** 领域里的册名原词，进审计与技术详情，给对文档、对登记口的人。 */
export const ruleBookDomainNames: Record<RuleBookKind, string> = {
  MILESTONE_MAPPING: '里程碑映射',
  TRIAGE_RULE: '分诊规则',
  CONFLICT_SIGNAL_RULE: '冲突信号规则',
  NOTIFICATION_POLICY: '通知策略',
  DISCLOSURE_POLICY: '披露策略',
  EXCEPTION_DISCLOSURE_RULE: '异常披露规则',
};

/** 每本册一句：它决定什么、没规则时会怎样。签下方与空态共用。 */
export const ruleBookNotes: Record<RuleBookKind, string> = {
  MILESTONE_MAPPING: '各环节报上来的动态归成哪个物流状态；没有对应规则的动态保持「未归类」，系统不会替它猜一个状态。',
  TRIAGE_RULE: '某类异常信号按可信度决定怎么处理：自动立案、并入已有案件、转人工复核，或不立案。',
  CONFLICT_SIGNAL_RULE: '同一件事前后报来的动态互相矛盾、又分不出先后时，记成哪一类异常信号。每家公司只有一条。',
  NOTIFICATION_POLICY: '要通知客户时用什么渠道、在多长时间内必须通知到。',
  DISCLOSURE_POLICY: '每个客户能看到哪些物流信息：状态、预计送达、最终结果、备注四项，各自显示、待确认还是不显示。',
  EXCEPTION_DISCLOSURE_RULE: '某个客户遇到某类异常时要不要告诉客户、能不能自动发出、用哪份文案。',
};

/** 登记签的标题：说这个任务做什么。冲突信号规则每家公司一条，所以是「设定」不是「新增」。 */
export const ruleRegistrationTitles: Record<RuleBookKind, string> = {
  MILESTONE_MAPPING: '新增状态归类规则',
  TRIAGE_RULE: '新增异常分流规则',
  CONFLICT_SIGNAL_RULE: '设定动态矛盾规则',
  NOTIFICATION_POLICY: '新增通知时限',
  DISCLOSURE_POLICY: '新增对客户显示规则',
  EXCEPTION_DISCLOSURE_RULE: '新增异常告知规则',
};

/** 受控登记命令行的子命令名（cmd/parcel-ve-register 已发布的原词），只给填写提示里那句对接说明用。 */
const registrationCommands: Record<RuleBookKind, string> = {
  MILESTONE_MAPPING: 'milestone-mapping',
  TRIAGE_RULE: 'triage-rules',
  CONFLICT_SIGNAL_RULE: 'conflict-signal-rule',
  NOTIFICATION_POLICY: 'notification-policy',
  DISCLOSURE_POLICY: 'disclosure-policy',
  EXCEPTION_DISCLOSURE_RULE: 'exception-disclosure-rules',
};

// 填写提示：只说填什么、填错了会怎样。形状与命令行工具吃的同一份，这一句留给对接的人。
function hint(kind: RuleBookKind, fields: string): string {
  return `${fields} 内容与命令行工具（${registrationCommands[kind]}）吃的同一份；所属公司由系统记下，不用填。写错的字段名会被退回，不会当成没填。`;
}

export const ruleRegistrationHints: Record<RuleBookKind, string> = {
  MILESTONE_MAPPING: hint(
    'MILESTONE_MAPPING',
    '请填 version（这一版的编号）、approvedBy（批准人）、effectiveFrom（从何时起用）和 entries（至少一条）；每条填 source（环节：' +
      'PARCEL_SHIPMENT 运单 / NETWORK_ROUTING 路线 / NODE_OPERATIONS 网点 / TRANSPORT_FULFILLMENT 运输 / CUSTOMS_COMPLIANCE 清关）、' +
      'factKind（动态类型）、milestone（归成的状态）。同一版里同一环节的同一动态只能写一条。',
  ),
  TRIAGE_RULE: hint(
    'TRIAGE_RULE',
    '请填 version、approvedBy、effectiveFrom 和 entries（至少一条）；每条填 kind（信号类型）、confidence（可信度）、outcome（处理方式：' +
      'AUTO_ESTABLISH 自动立案 / ATTACH_TO_EXISTING 并入已有案件 / MANUAL_REVIEW 转人工复核 / NO_CASE 不立案），自动立案时可填 team（负责团队）。',
  ),
  CONFLICT_SIGNAL_RULE: hint(
    'CONFLICT_SIGNAL_RULE',
    '请填 signalKind（记成哪类信号）、version（识别规则的版本）、confidence（可信度依据）、approvedBy（批准人），四项都必填。' +
      '每家公司只有一条：已经设定过时再提交会被退回，要换规则请联系管理员。',
  ),
  NOTIFICATION_POLICY: hint(
    'NOTIFICATION_POLICY',
    '请填 policy（对应的客户显示规则版本）、channel（通知渠道）、deadlineAfter（多久之内，如 "72h"，必须大于零）、' +
      'obligation（怎样算通知到）、approvedBy（批准人）。要换时限就登一条新的，旧的仍保留。',
  ),
  DISCLOSURE_POLICY: hint(
    'DISCLOSURE_POLICY',
    '请填 version、approvedBy、effectiveFrom 和 entries（每个客户一条）；每条填 customer（客户编码），以及 milestones（状态）、eta（预计送达）、' +
      'final（最终结果）、note（备注）四项，每项写 state（SHOWN 显示 / PENDING_CONFIRMATION 待确认 / NOT_DISCLOSED 不显示），只有显示时才填 content（文案）。',
  ),
  EXCEPTION_DISCLOSURE_RULE: hint(
    'EXCEPTION_DISCLOSURE_RULE',
    '请填 version、approvedBy、effectiveFrom 和 entries；每条填 customer（客户编码）、signalKind（异常类型）、confidence（可信度）、' +
      'disclosable（要不要告诉客户，true / false）、autoRelease（能不能自动发出，只有要告诉客户时才能为 true）；要告诉客户时必填 content（文案），不告诉时不填。' +
      '同一版里同一客户、同一异常类型、同一可信度只能写一条。',
  ),
};

/** 一条规则当前生不生效：码取用语词典已有的三格（已生效 / 已到期 / 待生效），不另造词。 */
export type RuleLifecycle = 'EFFECTIVE' | 'EXPIRED' | 'REGISTERED';

export function lifecycleAt(effectiveFrom: string, effectiveTo: string | undefined, now: Date): RuleLifecycle {
  if (Date.parse(effectiveFrom) > now.getTime()) return 'REGISTERED';
  if (effectiveTo !== undefined && Date.parse(effectiveTo) <= now.getTime()) return 'EXPIRED';
  return 'EFFECTIVE';
}

/**
 * 相对时限的人话：读口给的是 PostgreSQL interval 文本（「72:00:00」「3 days 04:00:00」）或登记时的 Go 时长（「72h」）。
 * 认得的折成「72 小时」「3 天 4 小时」；认不得的原样显示，不猜。
 */
export function deadlineText(raw: string): string {
  const interval = /^(?:(\d+) days? ?)?(?:(\d+):(\d{2}):(\d{2}))?$/.exec(raw.trim());
  const goDuration = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(raw.trim());
  let days = 0;
  let hours = 0;
  let minutes = 0;
  if (raw.trim() !== '' && interval && (interval[1] !== undefined || interval[2] !== undefined)) {
    days = Number(interval[1] ?? 0);
    hours = Number(interval[2] ?? 0);
    minutes = Number(interval[3] ?? 0);
  } else if (raw.trim() !== '' && goDuration && (goDuration[1] !== undefined || goDuration[2] !== undefined)) {
    hours = Number(goDuration[1] ?? 0);
    minutes = Number(goDuration[2] ?? 0);
  } else {
    return raw;
  }
  // 整天的时限按小时说更贴近通知承诺的口径（「72 小时内」），只有 interval 本身带了天数才说天。
  const parts: string[] = [];
  if (days > 0) parts.push(`${days} 天`);
  if (hours > 0) parts.push(`${hours} 小时`);
  if (minutes > 0) parts.push(`${minutes} 分钟`);
  return parts.length > 0 ? `${parts.join(' ')}内` : raw;
}

/** 客户格：有名称显名称，读不到名称（册未读回、读不到）如实显编码，不把一次读不到当成「册上没有」。 */
export function customerText(account: string, names: ReadonlyMap<string, string>): string {
  return names.get(account) ?? account;
}

/** 显示四项之一格只说态；显示时用的文案编号是一串引用，进审计（disclosureContentRows），不撑宽表格。 */
export function disclosureCellText(cell: DisclosureCellRecord): string {
  return labelFor(cell.state);
}

const disclosureItemNames = { milestones: '物流状态', eta: '预计送达', final: '最终结果', note: '备注' } as const;

function disclosureContentRows(entry: Record<keyof typeof disclosureItemNames, DisclosureCellRecord>): TechnicalRow[] {
  return (Object.keys(disclosureItemNames) as (keyof typeof disclosureItemNames)[]).map((item) => ({
    label: `${disclosureItemNames[item]}文案`,
    value: entry[item].content ?? '',
  }));
}

export interface RuleRow {
  key: string;
  /** 业务格：列 id → 已排好版的字。 */
  cells: Readonly<Record<string, string>>;
  /** 有生效期的册才有；冲突信号规则与通知时限没有生效期，不给它补一个。 */
  lifecycle?: RuleLifecycle;
  /** 右栏「审计」的几格：册名、版本号、批准人、原码。 */
  audit: TechnicalRow[];
  /** 胶囊格（状态、处理方式）的中文词：格里存的是码、由胶囊翻译，检索要认得中文词。 */
  words?: string[];
}

type ListBody<K extends VisibilityCatalogueKind> = Extract<VisibilityCatalogueListResponseBody, { kind: K }>;

function range(from: string, to?: string): string {
  return `${formatInstant(from)} 起${to ? `，至 ${formatInstant(to)}` : '，长期有效'}`;
}

/**
 * 读口答复 → 行。版本随行重复：册子是拿来查「这类动态会判成什么」的，按版本折行会把要查的规则埋进折叠格。空版本不存在——
 * 登记口要求每版至少一条，行展开不会藏掉任何版本。
 */
export function ruleRowsOf(
  body: ListBody<RuleBookKind>,
  names: ReadonlyMap<string, string>,
  now: Date,
): RuleRow[] {
  const book = ruleBookDomainNames[body.kind];
  switch (body.kind) {
    case 'MILESTONE_MAPPING':
      return body.catalogues.flatMap((catalogue) =>
        catalogue.entries.map((entry) => ({
          // 条目键取登记入口的防撞键（环节 + 动态类型）。
          key: `mapping:${catalogue.version}:${entry.source}:${entry.factKind}`,
          cells: {
            source: stageLabels[entry.source] ?? entry.source,
            fact: labelFor(entry.factKind),
            milestone: entry.milestone,
            effective: range(catalogue.effectiveFrom, catalogue.effectiveTo),
            approvedBy: catalogue.approvedBy,
          },
          lifecycle: lifecycleAt(catalogue.effectiveFrom, catalogue.effectiveTo, now),
          words: [labelFor(entry.milestone)],
          audit: [
            { label: '规则册', value: book },
            { label: '版本号', value: catalogue.version },
            { label: '批准人', value: catalogue.approvedBy },
            { label: '环节码', value: entry.source },
            { label: '动态类型码', value: entry.factKind },
            { label: '状态码', value: entry.milestone },
          ],
        })),
      );
    case 'TRIAGE_RULE':
      return body.catalogues.flatMap((catalogue) =>
        catalogue.entries.map((entry) => ({
          key: `triage:${catalogue.version}:${entry.signalKind}:${entry.confidence}`,
          cells: {
            signal: labelFor(entry.signalKind),
            confidence: labelFor(entry.confidence),
            outcome: entry.outcome,
            effective: range(catalogue.effectiveFrom, catalogue.effectiveTo),
            approvedBy: catalogue.approvedBy,
          },
          lifecycle: lifecycleAt(catalogue.effectiveFrom, catalogue.effectiveTo, now),
          words: [labelFor(entry.outcome)],
          audit: [
            { label: '规则册', value: book },
            { label: '版本号', value: catalogue.version },
            { label: '批准人', value: catalogue.approvedBy },
            { label: '信号类型码', value: entry.signalKind },
            { label: '可信度码', value: entry.confidence },
            { label: '处理方式码', value: entry.outcome },
          ],
        })),
      );
    case 'CONFLICT_SIGNAL_RULE':
      return body.catalogues.map((record) => ({
        // 一家公司一条（0025 键只有租户），行键取信号类型 + 规则版本足以区分并存的历史版。
        key: `conflict:${record.signalKind}:${record.version}`,
        cells: {
          signal: labelFor(record.signalKind),
          confidence: labelFor(record.confidence),
          registeredAt: formatInstant(record.registeredAt),
          approvedBy: record.approvedBy,
        },
        audit: [
          { label: '规则册', value: book },
          { label: '识别规则版本', value: record.version },
          { label: '批准人', value: record.approvedBy },
          { label: '信号类型码', value: record.signalKind },
          { label: '可信度码', value: record.confidence },
        ],
      }));
    case 'NOTIFICATION_POLICY':
      return body.catalogues.map((record) => ({
        // 策略引用即租户内行键（版本化由引用承担），不再拼别的列。
        key: `notify:${record.policy}`,
        cells: {
          channel: labelFor(record.channel),
          deadline: deadlineText(record.deadlineAfter),
          obligation: labelFor(record.obligation),
          approvedBy: record.approvedBy,
        },
        audit: [
          { label: '规则册', value: book },
          { label: '对应显示规则', value: record.policy },
          { label: '批准人', value: record.approvedBy },
          { label: '渠道码', value: record.channel },
          { label: '时限原文', value: record.deadlineAfter },
          { label: '判据码', value: record.obligation },
        ],
      }));
    case 'DISCLOSURE_POLICY':
      return body.catalogues.flatMap((catalogue) =>
        catalogue.entries.map((entry) => ({
          // 条目键取登记入口的防撞键（客户）。
          key: `disclose:${catalogue.version}:${entry.customer}`,
          cells: {
            customer: customerText(entry.customer, names),
            milestones: disclosureCellText(entry.milestones),
            eta: disclosureCellText(entry.eta),
            final: disclosureCellText(entry.final),
            note: disclosureCellText(entry.note),
            effective: range(catalogue.effectiveFrom, catalogue.effectiveTo),
            approvedBy: catalogue.approvedBy,
          },
          lifecycle: lifecycleAt(catalogue.effectiveFrom, catalogue.effectiveTo, now),
          audit: [
            { label: '规则册', value: book },
            { label: '版本号', value: catalogue.version },
            { label: '批准人', value: catalogue.approvedBy },
            { label: '客户编码', value: entry.customer },
            ...disclosureContentRows(entry),
          ],
        })),
      );
    case 'EXCEPTION_DISCLOSURE_RULE':
      return body.catalogues.flatMap((catalogue) =>
        catalogue.entries.map((entry) => ({
          // 条目键取 0023 的主键三维（客户 + 信号类型 + 可信度）。
          key: `disclose-rule:${catalogue.version}:${entry.customer}:${entry.signalKind}:${entry.confidence}`,
          cells: {
            customer: customerText(entry.customer, names),
            signal: labelFor(entry.signalKind),
            confidence: labelFor(entry.confidence),
            disclosable: entry.disclosable ? '告诉客户' : '不告诉',
            autoRelease: entry.autoRelease ? '自动发出' : '人工确认后发',
            effective: range(catalogue.effectiveFrom, catalogue.effectiveTo),
            approvedBy: catalogue.approvedBy,
          },
          lifecycle: lifecycleAt(catalogue.effectiveFrom, catalogue.effectiveTo, now),
          audit: [
            { label: '规则册', value: book },
            { label: '版本号', value: catalogue.version },
            { label: '批准人', value: catalogue.approvedBy },
            { label: '客户编码', value: entry.customer },
            { label: '信号类型码', value: entry.signalKind },
            { label: '可信度码', value: entry.confidence },
            // 文案只在要告诉客户时在场（0023 成对约束）；缺席时这一格不出现。
            { label: '文案编号', value: entry.content ?? '' },
          ],
        })),
      );
  }
}

/** 检索：业务格与审计格都算，搜中文词、编码、版本号都能命中。 */
export function ruleRowMatches(row: RuleRow, needle: string): boolean {
  const query = needle.trim().toLowerCase();
  if (query === '') return true;
  return [...Object.values(row.cells), ...row.audit.map((entry) => entry.value), ...(row.words ?? [])].some((value) =>
    value.toLowerCase().includes(query),
  );
}

/** 计数摘要：有生效期的册另报其中生效中的条数——旧版本留在册上可追溯，但运营关心的是现在用哪条。 */
export function ruleCountSummary(kind: RuleBookKind, rows: readonly RuleRow[]): string {
  const effective = rows.filter((row) => row.lifecycle === 'EFFECTIVE').length;
  const hasLifecycle = rows.some((row) => row.lifecycle !== undefined);
  return hasLifecycle
    ? `${ruleBookLabels[kind]} ${rows.length} 条，其中生效中 ${effective} 条`
    : `${ruleBookLabels[kind]} ${rows.length} 条`;
}

// —— 登记签的答复与错误（两页共用；presentation.ts 的同名表仍给索赔前置页用，理由见文件头）——

/** 登记答案（`application.RegisterCatalogOutcome` 原名）。 */
export const ruleRegistrationOutcomeLabels: Record<string, string> = {
  REGISTERED: '已保存，切回规则列表就能看到。',
  REFUSED: '没有保存，原因见下；已有的规则没有被改动。',
};

/**
 * 退回的理由（`application.CatalogRefusalReason` 原名），逐格说怎么改。末两格不是填错：是已有规则挡着，要换版本号或改生效时间。
 */
export const ruleRefusalReasonLabels: Record<string, string> = {
  SCOPE_MISSING: '缺少对象：通知时限要写对应的显示规则版本（policy），其余规则按提示写客户或类型。',
  VERSION_MISSING: '缺少版本号（version），请给这一版起一个编号。',
  APPROVAL_MISSING: '缺少批准人（approvedBy），请写上谁批准了这份规则。',
  EFFECTIVE_RANGE_MISSING: '缺少起用时间（effectiveFrom），请写从什么时候开始用。',
  EFFECTIVE_RANGE_REVERSED: '起止时间写反了或首尾相同，请改成先起用、后截止。',
  ENTRIES_MISSING: '一条规则都没写（entries 为空），请至少写一条。',
  ENTRY_INCOMPLETE: '有一条规则没写全；通知时限必须大于零，要告诉客户的异常必须带文案、不告诉的不能带。',
  ENTRY_DUPLICATED: '同一版里有两条规则管的是同一件事，请删掉或合并其中一条。',
  VERSION_NOT_OVERWRITABLE: '这个版本号已经用过了，已有的不会被覆盖；请换一个新的版本号。',
  VERSION_OVERLAPS_EXISTING: '同一时间已经有另一版规则在用，请先核对已有规则，再调整起用时间。',
};

/** 传输层错误码（不是业务答复）的人话：说哪里错、怎么办。 */
const ruleProblemNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '这个入口不接受这种提交方式，请刷新页面后重试；仍然不行请联系系统管理员。',
  MALFORMED_REQUEST: '填写的内容格式不对，系统读不出一份完整的规则；请按提示检查字段名和取值后再提交。',
  INTAKE_FAILED: '登录信息没能核对通过，这次什么都没有保存；请稍后重试。',
  NO_ANSWER_FORMED: '系统暂时没能处理，这次是否保存还不确定；请稍后刷新规则列表查看。',
  UNNAMED_OUTCOME: '系统返回了无法识别的结果，请联系系统管理员；重试不会好。',
  UNNAMED_REFUSAL_REASON: '规则被退回，但系统没说明原因，请联系系统管理员；重试不会好。',
};

export function ruleProblemNote(code: string): string {
  return ruleProblemNotes[code] ?? `这次没有成功（错误码 ${code}）；请记下页面上的技术详情联系系统管理员。`;
}
