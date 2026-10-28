// 界面用语词典 · 状态与枚举码（改版第一步，设计源「四、用语词典」）。
//
// 代码、CONTEXT、ADR 里的领域词不动；**界面上显示的词**一律经这里翻译，一处定义、全站生效。英文枚举值不再直接上屏：
// 查不到的码在开发模式告警一次并原样显示，让漏译的码在开发时就被看见，而不是在用户面前变成一串大写字母。
//
// 中文按运营一线的叫法而不是系统内部的叫法（`SUBMITTED` 对客服是「待受理」，不是「已提交」；`REGISTERED` 的身份对配置员是
// 「待生效」）。领域文档里的原词（`已提交`、`已登记`）仍是领域语言，只是界面上换成一线的话——两套词之间的对照就是这张表。
//
// 色调按设计原则四：待处理琥珀（warn）、推进中蓝（info）、成立 / 有利终局绿（good）、失败 / 阻断红（bad）、中性或已结束灰（muted）。
// 色调是管理台的呈现判断，不是领域状态本身；也不把各源状态拼成统一状态机（GLOSSARY「追踪摘要」的约束同样管着这张表）。
//
// 同一个码在不同册里意思不同时（`REGISTERED` 的身份是待生效、作业设备是在用），按册另给一份放 scopedStatusVocabulary，
// 查时先按册、再落总表。只在意思真不同时才开一册，同义不同册的码只登总表一处。
//
// 本文件不引任何组件库，也不读环境变量：告警钩子由装配点注入（main.tsx 只在开发模式注入 console.warn），node:test 里
// 用桩钉它；测试链按 CommonJS 发射，import.meta 在这里编不过。
//
// 物流动态那一段里的里程碑词（DELIVERED / PICKED_UP…）与各源事实类型（node-intake / effective-delivery…）是开放词表，
// 里程碑目录本身是租户登记的实例参数。这里收的是随产品发布的**参考译法**：演示租户 SYN-TENANT-01 按真实租户对待
// （ADR-0150），它登记的码先译好；别的租户登了新码，页面如实露码并告警，由那张票补译，不猜。

export type StatusTone = 'good' | 'info' | 'warn' | 'bad' | 'muted';

export interface StatusEntry {
  label: string;
  tone: StatusTone;
}

const good = (label: string): StatusEntry => ({ label, tone: 'good' });
const info = (label: string): StatusEntry => ({ label, tone: 'info' });
const warn = (label: string): StatusEntry => ({ label, tone: 'warn' });
const bad = (label: string): StatusEntry => ({ label, tone: 'bad' });
const muted = (label: string): StatusEntry => ({ label, tone: 'muted' });

/** 总表：码 → 界面词与色调。 */
export const statusVocabulary: Record<string, StatusEntry> = {
  // —— 运单（委托）生命周期 · parcel-shipment ——
  SUBMITTED: warn('待受理'),
  ACCEPTED: good('已受理'),
  REJECTED: bad('已拒绝'),
  WITHDRAWN: muted('已撤回'),
  CANCELLED: muted('已取消'),
  PARCEL_CANCELLED: muted('已取消'),
  DISPOSITION_PENDING: warn('待处置'),
  // 接受判断任务：判完了（已完成）与没人再判了（已停止）是两个事实，词不合并。
  RUNNING: info('处理中'),
  COMPLETE: good('已完成'),
  COMPLETED: good('已完成'),
  STOPPED: muted('已停止'),
  UNDECIDED: warn('未决'),
  // 准入控制：OPEN 只表示没有生效暂停，不表示允许建单，所以不是 good。
  OPEN: info('开放'),
  PAUSED: warn('已暂停'),

  // —— 身份与关系 · party-commercial ——
  // 已登记但生效时点未到，对配置员就是「待生效」；候选关系是等批准，对配置员就是「待确认」。
  REGISTERED: warn('待生效'),
  EFFECTIVE: good('已生效'),
  DEACTIVATED: muted('已停用'),
  CANDIDATE: warn('待确认'),
  EXPIRED: muted('已到期'),
  // 撤销是携依据的主动终止，边界之后仍拿这段关系做决定就是错，要点出来；到期与替代是时间到了、有后继接着，中性。
  REVOKED: warn('已撤销'),
  SUPERSEDED: muted('已替代'),

  // —— 商业版本、价格表草稿与发布稿 · party-commercial / parcel-pricing ——
  DRAFT: muted('草稿'),
  VALIDATED: info('已校验'),
  PENDING_APPROVAL: warn('待批准'),
  APPROVED: good('已批准'),
  PUBLISHED: info('已发布'),
  RETIRED: muted('已退役'),

  // —— 费用与账务 · settlement-accounting ——
  // 预估、暂估是费用的正常阶段不是风险，归中性；争议与未分配等人核，琥珀。
  ESTIMATED: muted('预估'),
  PROVISIONAL: muted('暂估'),
  CONFIRMED: good('已确认'),
  SETTLED: good('已结算'),
  PARTIALLY_ACCEPTED: warn('部分接受'),
  PENDING_REVIEW: warn('待复核'),
  DISPUTED: warn('争议'),
  UNAPPLIED: warn('未分配'),

  // —— 物流动态 · 标准里程碑（租户登记的实例参数，这里是参考译法，见文件头）——
  PICKED_UP: good('已揽收'),
  IN_TRANSIT: info('运输中'),
  DEPARTED: info('已发出'),
  ARRIVED: info('已到达'),
  OUT_FOR_DELIVERY: info('派送中'),
  DELIVERED: good('已签收'),
  DELIVERY_FAILED: bad('派送失败'),
  CUSTOMS_SUBMITTED: info('已申报'),
  CUSTOMS_HOLD: warn('海关查验'),
  CUSTOMS_MOVEMENT: info('关区转运'),
  CUSTOMS_CLEARED: good('已放行'),
  EXCEPTION: bad('异常'),
  // 终局责任结果（parcel-shipment 终局规则的四值）：有效送达对一线就是签收。
  EFFECTIVE_DELIVERY: good('已签收'),
  RETURN_COMPLETED: muted('已退回'),
  SERVICE_TERMINATED: bad('服务终止'),
  REGULATORY_DISPOSITION: bad('监管处置'),

  // —— 物流动态 · 各源事实类型（源上下文拥有的开放词表，参考译法）——
  'final-outcome-adopted': good('已完结'),
  'initial-route-formed': info('已规划路线'),
  'route-reassessed': info('路线已复核'),
  'node-intake': good('已收寄'),
  // 节点间交出是路上的一步（蓝），与运输的权威交接结果「已交接」（绿、成立）不是一个词，免得同词异色。
  'node-handover': info('已交出'),
  'effective-delivery': good('已签收'),
  'declaration-submitted': info('已申报'),
  'external-result-release-full': good('已放行'),
  'external-result-release-partial': warn('部分放行'),
  'external-result-release-conditional': warn('附条件放行'),
  'transport-departed': info('已发出'),
  'transport-arrived': info('已到达'),
  // 异常动线上客户拒收后形成退运的那条事实（演示租户四段动线之一）。
  'exception-journey-return': warn('退运中'),

  // —— 清关 · customs-compliance ——
  FULL: good('全部放行'),
  PARTIAL: warn('部分放行'),
  CONDITIONAL: warn('附条件放行'),
  CLEARED: good('已放行'),
  HELD: warn('查验中'),
  REGISTERED_PENDING_SEND: warn('待发送'),
  ACKNOWLEDGED: good('已确认'),
  FAILED: bad('失败'),
  PENDING_CONFIRMATION: warn('待确认'),
  PENDING: warn('待确认'),
  MET: good('满足'),
  UNMET: bad('未满足'),
  CONFLICTING: bad('冲突'),
  // 放行检查的准入答案：ADMITTED 对一线就是「可放行」；NOT_GUARDED 是本就不用检查，不是没检查。
  NOT_GUARDED: muted('无需检查'),
  ADMITTED: good('可放行'),
  NOT_CONFIGURED: warn('未配置'),
  BLOCKED: bad('已拦截'),
  CONCLUDED: muted('已结束'),
  HANDED_OVER: good('已交接'),
  UNRESOLVED: warn('未解决'),
  NONE: muted('无覆盖'),
  COVERED: good('已覆盖'),
  NO_DELTA: good('无差额'),
  SHORT: bad('不足'),
  EXCESS: warn('超额'),
  VALID: good('有效'),
  INVALIDATED: muted('已失效'),

  // —— 运输与网点 · transport-fulfillment / node-operations / network-routing ——
  REFUSED: bad('已拒绝'),
  INTAKE_FORMED: good('已收寄'),
  PENDING_IDENTIFICATION: warn('待识别'),
  SEALED: info('已封装'),
  CLOSED: muted('已关闭'),
  CURRENTLY_EFFECTIVE: good('当前有效'),
  LAPSED: muted('已失效'),

  // —— 异常与索赔 · visibility-exception ——
  AWAITING_RESPONSE: warn('待响应'),
  IN_PROGRESS: info('处理中'),
  SUPPLEMENT_REQUIRED: warn('要求补充'),
  AUTO_ESTABLISH: info('自动立案'),
  ATTACH_TO_EXISTING: info('关联既有案件'),
  MANUAL_REVIEW: warn('进入人工复核'),
  NO_CASE: muted('不立案'),
  ELIGIBLE: good('符合条件'),
  INELIGIBLE: bad('不符合'),
  AWAITING_SUPPLEMENT: warn('等待补充材料'),
  FULLY_ESTABLISHED: good('全部成立'),
  PARTIALLY_ESTABLISHED: warn('部分成立'),
  NOT_ESTABLISHED: bad('不成立'),
  UNDETERMINABLE: warn('无法认定'),
  PREPARED: muted('已准备'),
  CHANNEL_ACCEPTED: info('渠道已接受'),
  SUBMISSION_FAILED: bad('提交失败'),
  SHOWN: good('展示'),
  NOT_DISCLOSED: muted('不披露'),
  CANCELLATION_ACCEPTED: good('取消已接受'),
  PARTIALLY_CANCELLED: warn('部分取消'),
  NO_LONGER_CANCELLABLE: muted('已不可取消'),
  CANCELLATION_REFUSED: bad('取消被拒'),

  // —— 计价与渠道择优 · parcel-pricing / parcel-shipment ——
  CONFLICT: bad('冲突'),
  UNRATABLE: bad('不可计价'),
  SELECTED: good('选中'),
  NOT_SELECTED: muted('落选'),
  EXCLUDED: muted('出局'),
  TIED: warn('并列'),
};

/**
 * 分册覆盖：同一个码在这几本册里意思与总表不同。册名是页面自己起的呈现分组名，不是限界上下文名——
 * 一本册里的码只在那本册里这么读。
 */
export const scopedStatusVocabulary: Record<string, Record<string, StatusEntry>> = {
  // 作业设备册：REGISTERED 是在用、不是等生效（设备没有生效时点）。
  'work-device': { REGISTERED: good('在用'), DEACTIVATED: muted('已停用') },
  // 价格评价：PENDING 等的是下游判断，FAILED 是评价没形成。
  'pricing-evaluation': { PENDING: warn('待判断'), FAILED: bad('未形成') },
  // 追偿动作的过程节点：SUBMITTED 是向对方提交了，DELIVERED 是送达对方，ACKNOWLEDGED 是对方确认。
  'claim-recovery': { SUBMITTED: info('已提交'), DELIVERED: good('已送达'), ACKNOWLEDGED: good('对方已确认') },
  // 权威运输交接结果：REFUSED 是收货方拒收这一票实物。
  handover: { REFUSED: bad('已拒收') },
  // 关务关闭义务：HANDED_OVER 是义务交由他方承接，CONCLUDED 是义务终结。
  'customs-obligation': { HANDED_OVER: good('已承接'), CONCLUDED: muted('已终结') },
  // 技术回执层：ACKNOWLEDGED / FAILED 说的是报文有没有送达，不是业务受理。
  'customs-receipt': { ACKNOWLEDGED: good('技术成功'), FAILED: bad('技术失败') },
  // 集运单元阶段：OPEN 是还能往里装。
  consolidation: { OPEN: info('开放装入') },
  // 税费覆盖：PARTIAL 是部分覆盖，不是部分放行。
  'duty-coverage': { PARTIAL: warn('部分覆盖') },
};

type UnknownCodeHandler = (code: string) => void;

let warnUnknown: UnknownCodeHandler | null = null;
const warned = new Set<string>();

/**
 * 装配点注入「查不到的码」的告警去处（开发模式给 console.warn，生产不注入）；传 null 关掉。每个码只告警一次——
 * 同一个码在一张表里出现几十次，警几十遍只会把真正的新码淹掉。
 */
export function configureVocabularyWarnings(handler: UnknownCodeHandler | null): void {
  warnUnknown = handler;
  warned.clear();
}

/** 码的界面词与色调；查不到的码原样作词、色调中性，并按文件头说的告警一次。 */
export function statusEntryFor(code: string, scope?: string): StatusEntry {
  const scoped = scope !== undefined ? scopedStatusVocabulary[scope]?.[code] : undefined;
  const entry = scoped ?? statusVocabulary[code];
  if (entry !== undefined) return entry;
  if (warnUnknown !== null && !warned.has(code)) {
    warned.add(code);
    warnUnknown(code);
  }
  return { label: code, tone: 'muted' };
}

export function labelFor(code: string, scope?: string): string {
  return statusEntryFor(code, scope).label;
}

export function toneFor(code: string, scope?: string): StatusTone {
  return statusEntryFor(code, scope).tone;
}

/**
 * 按界面词反查色调，给只拿得到词、拿不到码的格用（行模型为了能在 node:test 里跑只交字符串）。同一个词被几个码共用时
 * 它们的色调一致——这是词典自己守的约定（status.test.ts 钉着），所以取第一个命中的就够。查不到答中性。
 */
export function toneForLabel(label: string): StatusTone {
  for (const entry of Object.values(statusVocabulary)) {
    if (entry.label === label) return entry.tone;
  }
  for (const scoped of Object.values(scopedStatusVocabulary)) {
    for (const entry of Object.values(scoped)) {
      if (entry.label === label) return entry.tone;
    }
  }
  return 'muted';
}

/** 总表收录的全部码，给测试与工具数覆盖面用。 */
export const knownStatusCodes: readonly string[] = Object.keys(statusVocabulary);
