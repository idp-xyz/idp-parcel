// 作业与履约查阅页的结果词表。词取 CONTEXT.md 与传输层的封闭词，不自造译法；
// 节点、载具、参与方、方向、单位等开放引用一律原词直示，翻译任何一个都是替租户
// 造第二套口径。

/** 收寄判断封闭两格（nodeoperations domain ReceptionKind）。 */
export const receptionKindLabels: Record<string, string> = {
  INTAKE_FORMED: '已形成收寄',
  PENDING_IDENTIFICATION: '待识别',
};

/** 实物控制建立来源封闭两格（nodeoperations domain PhysicalControl）。 */
export const controlKindLabels: Record<string, string> = {
  NODE_INTAKE: '节点收寄',
  HANDOVER_IN: '交接转入',
};

/** 集运单元实例阶段封闭三相，中文取 CONTEXT 生命周期原词。 */
export const consolidationPhaseLabels: Record<string, string> = {
  OPEN: '开放装入',
  SEALED: '已封装',
  CLOSED: '已关闭',
};

/**
 * 权威交接结果封闭三格（transportfulfillment domain HandoverVerdict）。中文词
 * 与共享词表 domainStatusTones 同词（已交接/已拒收/待确认），呈现层按词表着色。
 */
export const handoverVerdictLabels: Record<string, string> = {
  HANDED_OVER: '已交接',
  REFUSED: '已拒收',
  PENDING_CONFIRMATION: '待确认',
};

/** 词表没收录的码原样示码，不猜词——封闭集扩了、页面还没跟上时如实露码。 */
export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}
