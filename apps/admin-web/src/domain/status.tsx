import type { HTMLAttributes, ReactNode } from 'react';
import { StatusBadge, type StatusBadgeProps } from '@idpxyz/ui-patterns';
import { Tag } from '@idpxyz/ui-primitives';
import { domainStatusLayers, statusLayerShapes, tagVariantByTone, type StatusLayer } from './status-layers';

// 租户管理台的领域状态词表：状态词 → 呈现色调的唯一映射。
//
// 词一律取权威文档原词（docs/domain/GLOSSARY.md 与各上下文 CONTEXT.md），不自造译法、
// 不另写展示文案；同一个词在多个上下文出现时只登记一次，条目注释点出各出处与取舍。
// 色调只是管理台的呈现判断，不是领域状态本身：状态由各上下文按有效事件派生
// （GLOSSARY「状态」），本表不拥有任何状态，也不把各源状态拼成统一状态机——
// GLOSSARY「追踪摘要」明确要求摘要不得成为覆盖各源状态的统一状态机，词表同受此约束。

/**
 * 呈现色调，按词的业务语义分档：
 * - neutral：中性阶段或中性终局，无需关注（草稿、已撤回、已关闭）；
 * - info：正常推进中的阶段，只作进度提示（已提交、处理中）；
 * - warning：结论尚未形成，需要有人续办、补充或人工处置（待确认、未决、解析未决）；
 * - critical：确定性失败、冲突或阻断，需责任方修正（已拒绝、适用冲突、不可达）；
 * - positive：明确成立或有利终局（已接受、可达、结清）。
 */
export type StatusTone = 'neutral' | 'info' | 'warning' | 'critical' | 'positive';

export const domainStatusTones = {
  // —— 委托生命周期 · 出处：docs/domain/parcel-shipment/CONTEXT.md 的委托生命周期 ——
  // 草稿在 party-commercial 商业版本与 settlement-accounting 对账单里也是起始态，同为中性。
  '草稿': 'neutral',
  '已提交': 'info',
  // visibility-exception 处置请求的「已接受范围」同用此词，都是有利结论。
  '已接受': 'positive',
  '已拒绝': 'critical',
  // 撤回与取消都是合法业务终局，不是错误（GLOSSARY「委托撤回」「包裹取消决定」）。
  '已撤回': 'neutral',
  '已取消': 'neutral',
  '已完成': 'positive',

  // —— 接受判断与资料采用 · 出处：docs/domain/parcel-shipment/CONTEXT.md 的接受判断任务、
  // GLOSSARY「当前客户资料版本采用判断」 ——
  // 未决时委托仍为「已提交」、等待续办方行动，所以是提醒档而非失败档。
  '未决': 'warning',
  '已采用': 'positive',
  '待下游判断': 'info',
  '待补充': 'warning',
  '冲突': 'critical',

  // —— 商业解析结果 · 出处：docs/domain/party-commercial/CONTEXT.md 商业依据唯一适用规则、
  // docs/application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md ——
  // 无适用依据与适用冲突要商业责任方修依据，属确定性阻断；解析未决（重试同一次调用）、
  // 依据未解析（回第一阶段重解）、已失效（提交前被新修订推翻后重解）都可续办，属提醒档；
  // 输入未受理要调用方改请求，是确定性不受理。
  '无适用依据': 'critical',
  '适用冲突': 'critical',
  '解析未决': 'warning',
  '输入未受理': 'critical',
  '依据未解析': 'warning',
  '已失效': 'warning',

  // —— 商业版本生命周期 · 出处：docs/domain/party-commercial/CONTEXT.md 各商业对象生命周期 ——
  // 已发布对商业版本意味着等生效边界，对对账单意味着单号与金额已固定
  // （docs/domain/settlement-accounting/CONTEXT.md），都是推进中的确定阶段。
  '已发布': 'info',
  '已生效': 'positive',
  '已退役': 'neutral',

  // —— 参与方身份生命周期 · 出处：docs/domain/party-commercial/CONTEXT.md Lifecycles 下
  // 「参与方身份（业务参与方、责任法人、货主客户账户）」（已登记 → 已生效 → 已停用）——
  // 已登记是登记已落册、生效时点未到：CONTEXT 原句「生效时点未到的登记不支持新的商业决定」，
  // 是推进中的确定阶段、无需有人行动，与`已发布`同档。已停用是携显式依据与时点的合法终局，
  // 身份及其历史登记保留、既有引用继续有效，不靠着色报警，与`已退役`同档。`已生效`共用上面那格。
  '已登记': 'info',
  '已停用': 'neutral',

  // —— 参与方关系生命周期 · 出处：docs/domain/party-commercial/CONTEXT.md Lifecycles 下「参与方关系」
  // （候选关系 → 已生效 → 已到期 / 已撤销 / 已替代）——
  // 候选关系是登记进来、等批准的关系（登记快照缺 approval 即候选）：CONTEXT 原句「双方身份、角色、方向、范围、
  // 依据和有效期间完整，并通过适用批准」才成已生效——推进中的确定阶段，与`已登记`同档。`已生效`共用上面那格。
  '候选关系': 'info',
  // 已到期：商业版本有效期自然结束、关系有效期间届满；visibility-exception 处置请求受理有效期届满同用此词。
  // 已替代：有后继接着（关系格里带后继标识）。两者都是确定性中性终局，不需要人注意：继续需要该动作时形成
  // 关联新对象，不靠着色报警。
  '已到期': 'neutral',
  '已替代': 'neutral',
  // 已撤销取 warning 而不是 neutral（票 admin-web-group-legal-entities/09 裁决 2）：到期是时间到了、替代有后继接着，
  // 撤销是携依据的主动终止——CONTEXT「自明确生效边界起不再支持新的商业决定」，在边界之后仍拿这段关系做决定
  // 就是错，色调要把这一格点出来。transport-fulfillment 总单适用状态的「已撤销」同用此词、同一判据：撤销的总单
  // 不再是有效运输凭证，看的人要知道边界。
  '已撤销': 'warning',

  // —— 面单交易 · 出处：GLOSSARY「面单交易定案」「面单继续尝试决定」 ——
  '结果待确认': 'warning',
  // 定案只说明结果已明确、不再待确认，定下的可能是失败，故中性而非 positive。
  '已定案': 'neutral',
  // 受控关闭阻断截断边界后的新尝试、重开需同级或更高授权，值得提醒但不是失败。
  '受控关闭': 'warning',

  // —— 运输交接与可达性 · 出处：GLOSSARY「权威运输交接结果」「可达性判断」 ——
  '已交接': 'positive',
  '已拒收': 'critical',
  '待确认': 'warning',
  '可达': 'positive',
  // 不可达是权威确定性结论；产品允许待路由时是否仍接受由 parcel-shipment 翻译，
  // 呈现层不替它作判断。
  '不可达': 'critical',
  // 资料不足是证据不够判、可补齐重判，文档明禁把它伪装成不可达（GLOSSARY「可达性判断」）。
  '资料不足': 'warning',

  // —— 异常案件与处置请求 · 出处：docs/domain/visibility-exception/CONTEXT.md ——
  // 待响应即首次响应时限计时中，是复核队列里最需要人接手的一档。
  '待响应': 'warning',
  '处理中': 'info',
  '已关闭': 'neutral',
  // 已归并是关闭结论的一种：重复案件并入主案件，历史保留。
  '已归并': 'neutral',
  '待处理': 'info',
  '部分接受': 'warning',
  // 处置请求被源上下文拒绝；节点与运输的承接决定同用此词（GLOSSARY「节点监管协作承接决定」）。
  '拒绝': 'critical',
  '要求补充': 'warning',

  // —— 对账与核销 · 出处：docs/domain/settlement-accounting/CONTEXT.md、
  // GLOSSARY「费用明细」「对账单」 ——
  // 预估、暂估是费用明细的正常阶段而非风险，归中性档。
  '预估': 'neutral',
  '暂估': 'neutral',
  '已确认': 'positive',
  '争议': 'warning',
  '结清': 'positive',
  // 收付款存在歧义时必须保留为未分配、等人工核销，不得自动分配，故需提醒。
  '未分配': 'warning',
} as const satisfies Record<string, StatusTone>;

/** 管理台会呈现的生命周期状态词，全部取自权威文档原词。 */
export type DomainStatus = keyof typeof domainStatusTones;

// 色调档 → ui-patterns StatusBadge 变体名的翻译只发生在这一处：critical/positive 是
// 本仓语义命名，组件库叫 danger/success。组件库的 pending 变体不启用——等待类词按
// 「是否需要有人行动」落入 info 或 warning，比按图标分档更贴近治理台复核队列的用法。
const badgeStatusByTone: Record<StatusTone, NonNullable<StatusBadgeProps['status']>> = {
  neutral: 'neutral',
  info: 'info',
  warning: 'warning',
  critical: 'danger',
  positive: 'success',
};

export interface LayeredStatusBadgeProps extends Omit<StatusBadgeProps, 'status' | 'children'> {
  /** 哪一层（票 admin-web-ux-alignment/05）：决定形；词表词由 StatusBadgeFor 按 domainStatusLayers 查，非词表的合成演示才直接给。 */
  layer: StatusLayer;
  /** 色调：决定色；词表词由 StatusBadgeFor 按 domainStatusTones 查。 */
  tone: StatusTone;
  children: ReactNode;
}

/**
 * 形随层、色随词的渲染件——层 → 形在 status-layers.ts 的 statusLayerShapes 一处定义，这里只按形取组件。
 * lifecycle 一层就是 StatusBadge 本身，`icon` / `hideIcon` 原样透传；其它形只收 className 与 HTML 属性。
 * tag-mono 与 tag-pill 两形不着色：Tag 的 outline 变体只有描边、没有按色调分的变体，`tone` 在这两形上不参与——
 * 这是层还没有词时的设计系统层预设（黄金标准要五层在设计系统层预先分开），第一批 sla / flag 词进表时若要色，
 * 改 statusLayerShapes 换形即可，不动词表、不动页面。
 */
export function LayeredStatusBadge({ layer, tone, children, icon, hideIcon, className, ...rest }: LayeredStatusBadgeProps) {
  const htmlProps = rest as HTMLAttributes<HTMLSpanElement>;
  switch (statusLayerShapes[layer]) {
    case 'badge':
      return (
        <StatusBadge status={badgeStatusByTone[tone]} icon={icon} hideIcon={hideIcon} className={className} {...htmlProps}>
          {children}
        </StatusBadge>
      );
    case 'badge-plain':
      return (
        <StatusBadge status={badgeStatusByTone[tone]} hideIcon className={joinClasses('font-semibold', className)} {...htmlProps}>
          {children}
        </StatusBadge>
      );
    case 'tag-mono':
      return (
        <Tag variant="outline" size="sm" className={joinClasses('font-mono', className)} {...htmlProps}>
          {children}
        </Tag>
      );
    case 'tag-square':
      return (
        <Tag variant={tagVariantByTone[tone]} size="sm" className={joinClasses('rounded-sm', className)} {...htmlProps}>
          {children}
        </Tag>
      );
    case 'tag-pill':
      return (
        <Tag variant="outline" size="sm" className={joinClasses('rounded-full', className)} {...htmlProps}>
          {children}
        </Tag>
      );
  }
}

function joinClasses(...parts: (string | undefined)[]): string | undefined {
  const joined = parts.filter(Boolean).join(' ');
  return joined === '' ? undefined : joined;
}

export interface StatusBadgeForProps extends Omit<StatusBadgeProps, 'status' | 'children'> {
  /** 要呈现的状态词。徽章文案就是词本身，不接受另写文案——另写等于自造译法。 */
  status: DomainStatus;
}

/** 按词表查层与色调渲染一个状态词徽章：层定形、词定色。 */
export function StatusBadgeFor({ status, ...rest }: StatusBadgeForProps) {
  return (
    <LayeredStatusBadge layer={domainStatusLayers[status]} tone={domainStatusTones[status]} {...rest}>
      {status}
    </LayeredStatusBadge>
  );
}
