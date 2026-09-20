import type { DomainStatus, StatusTone } from './status';

// 状态词的「层」轴（票 admin-web-ux-alignment/05）：黄金标准「状态语义黄金标准」要求 Lifecycle / SLA / Risk / Severity / Flags 五层在设计系统层
// 预先分开，不许一张词 → 色表混装。domain/status.tsx 的 domainStatusTones 只答「什么色调」，这里答「哪一层」；形随层走，色随词走，两轴各一处。
//
// 本文件不引任何原语（只 `import type`）：它是纯数据，node:test 直接钉；渲染在 status.tsx 的 LayeredStatusBadge。

/**
 * 五层，顺序即黄金标准列举的顺序：
 * - lifecycle：对象自己的生命周期状态与判断结论（CONTEXT 把接受判断 / 商业解析 / 可达性都建成有自己生命周期的对象，其结论就是该对象的状态词）；
 * - sla：时限相关的态（超时、临近、按时）——按时间边界算出来的，不是对象状态；
 * - risk：对象上的风险判定（高 / 中 / 低一类）；
 * - severity：异常与案件的严重度分级；
 * - flag：标记类（关注 / 冻结 / 待核实一类），可多枚并存。
 */
export const statusLayers = ['lifecycle', 'sla', 'risk', 'severity', 'flag'] as const;
export type StatusLayer = (typeof statusLayers)[number];

/**
 * 每个状态词恰归一层。`satisfies Record<DomainStatus, StatusLayer>` 让词表与本表一起长：domainStatusTones 多一词这里编不过，少一词也编不过。
 *
 * 今天表里的词全部归 lifecycle——包括「已接受 / 已拒绝 / 冲突 / 无适用依据 / 可达 / 不可达」这类判断结论：CONTEXT 把它们建成**该判断对象**的状态，
 * 不是挂在对象上的风险标记；「资料不足 / 待补充 / 要求补充」这类续办提示也仍是所在对象的状态，不是 flag。其余四层今天没有词——留层不留词，
 * 追踪 ETA（sla）、案件严重度（severity）、关务限制（risk / flag）的读口登记格之后再进表（票面裁决 2）。
 */
export const domainStatusLayers = {
  // —— 委托生命周期 ——
  '草稿': 'lifecycle',
  '已提交': 'lifecycle',
  '已接受': 'lifecycle',
  '已拒绝': 'lifecycle',
  '已撤回': 'lifecycle',
  '已取消': 'lifecycle',
  '已完成': 'lifecycle',
  // —— 接受判断与资料采用 ——
  '未决': 'lifecycle',
  '已采用': 'lifecycle',
  '待下游判断': 'lifecycle',
  '待补充': 'lifecycle',
  '冲突': 'lifecycle',
  // —— 商业解析结果 ——
  '无适用依据': 'lifecycle',
  '适用冲突': 'lifecycle',
  '解析未决': 'lifecycle',
  '输入未受理': 'lifecycle',
  '依据未解析': 'lifecycle',
  '已失效': 'lifecycle',
  // —— 商业版本生命周期 ——
  '已发布': 'lifecycle',
  '已生效': 'lifecycle',
  '已退役': 'lifecycle',
  // —— 参与方身份生命周期 ——
  '已登记': 'lifecycle',
  '已停用': 'lifecycle',
  // —— 参与方关系生命周期 ——
  '候选关系': 'lifecycle',
  '已到期': 'lifecycle',
  '已替代': 'lifecycle',
  '已撤销': 'lifecycle',
  // —— 面单交易 ——
  '结果待确认': 'lifecycle',
  '已定案': 'lifecycle',
  '受控关闭': 'lifecycle',
  // —— 运输交接与可达性 ——
  '已交接': 'lifecycle',
  '已拒收': 'lifecycle',
  '待确认': 'lifecycle',
  '可达': 'lifecycle',
  '不可达': 'lifecycle',
  '资料不足': 'lifecycle',
  // —— 异常案件与处置请求 ——
  '待响应': 'lifecycle',
  '处理中': 'lifecycle',
  '已关闭': 'lifecycle',
  '已归并': 'lifecycle',
  '待处理': 'lifecycle',
  '部分接受': 'lifecycle',
  '拒绝': 'lifecycle',
  '要求补充': 'lifecycle',
  // —— 对账与核销 ——
  '预估': 'lifecycle',
  '暂估': 'lifecycle',
  '已确认': 'lifecycle',
  '争议': 'lifecycle',
  '结清': 'lifecycle',
  '未分配': 'lifecycle',
} as const satisfies Record<DomainStatus, StatusLayer>;

/**
 * 层 → 形。形是设计系统层的选择，一层一形、五形各异，看的人不读字就分得出层：
 * - badge：填充圆角徽章带图标（今天的 StatusBadge）——生命周期；
 * - tag-mono：描边标签、等宽字——时限是数字感的东西；
 * - tag-square：实底方角标签——风险要比生命周期更「硬」；
 * - badge-plain：填充徽章无图标、字重加一档——严重度靠字重与色，不靠图标；
 * - tag-pill：描边药丸、小号——标记可多枚并存，要轻。
 */
export const statusShapes = ['badge', 'tag-mono', 'tag-square', 'badge-plain', 'tag-pill'] as const;
export type StatusShape = (typeof statusShapes)[number];

export const statusLayerShapes = {
  lifecycle: 'badge',
  sla: 'tag-mono',
  risk: 'tag-square',
  severity: 'badge-plain',
  flag: 'tag-pill',
} as const satisfies Record<StatusLayer, StatusShape>;

/** 色调 → ui-primitives Tag 变体；与 status.tsx 的 `badgeStatusByTone` 同一档次划分，只是组件库两套命名。 */
export type TagVariant = 'default' | 'outline' | 'success' | 'warning' | 'error' | 'primary';
export const tagVariantByTone = {
  neutral: 'default',
  info: 'primary',
  warning: 'warning',
  critical: 'error',
  positive: 'success',
} as const satisfies Record<StatusTone, TagVariant>;
