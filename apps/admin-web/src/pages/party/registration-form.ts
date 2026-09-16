// 参与方模块各登记表单（责任法人 / 业务参与方 / 参与方关系 / 身份停用）共用的纯逻辑（票 admin-web-group-legal-
// entities/13 第 3 条抬出）。四份 *-form.ts 各自只留本册的形状（草稿、载荷键、认领路径、修订建议），跨册同一判的编码层
// 规则住在这里；此前它住在 business-party-form.ts 里导出、另两份反向依赖兄弟模块（票 10 评审 N3）。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：只做把格编对的事。

const positiveInteger = /^[1-9]\d*$/;

/**
 * 修订号编成正整数；编不出交回 undefined，由各表单的问题表拦在送之前、载荷里该键缺席——不造一个假数顶上。
 * 首尾空白在这一格允许：它编成的是数不是身份串，周围空白不构成第二个值；身份串各表单自己裁不裁与它无关。
 */
export function revisionOf(raw: string): number | undefined {
  const trimmed = raw.trim();
  return positiveInteger.test(trimmed) ? Number(trimmed) : undefined;
}
