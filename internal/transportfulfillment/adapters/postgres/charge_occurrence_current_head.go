package postgres

import "fmt"

// currentChargeOccurrenceHead 是发生项「仍有效」的唯一一处定义，拼进 `WHERE` 的一个条件：这一版不是失效版，且没有别的
// 版本以 corrects_version 回指它。后半句读的是每一项的当前版——被替代的旧版已由新版接替；前半句挡掉失效的那一项，它说
// 的是「那个范围不成立」，其成员可以另登。读发生项「仍有效」的查询（FindCovering、ListCurrentOnScope、ListCurrentCarrying）
// 都引它，改这句话只改这里。
//
// 调用方要守三条：
//   - 查询里发生项那张表必须别名为 head。别名写死在片段里而不收参数：用错时是 SQL 报错，不会悄悄换了意思。
//   - invalidatedParam 是该查询里绑定「失效种类」那一格的参数序号，绑定的值必须是 domain.OccurrenceInvalidated.String()；
//     失效种类走绑定参数而不写成 SQL 字面量，词表因此只在领域里定义一处。
//   - 拼进 SQL 文本的只有这个 int 序号与本函数里的常量，调用方不得往片段里再拼任何外部输入。
func currentChargeOccurrenceHead(invalidatedParam int) string {
	return fmt.Sprintf(`head.revision_kind IS DISTINCT FROM $%d
		    AND NOT EXISTS (
		        SELECT 1
		          FROM transport_fulfillment.transport_charge_occurrence AS successor
		         WHERE successor.tenant_id = head.tenant_id
		           AND successor.occurrence_ref = head.occurrence_ref
		           AND successor.corrects_version = head.validity_version)`, invalidatedParam)
}
