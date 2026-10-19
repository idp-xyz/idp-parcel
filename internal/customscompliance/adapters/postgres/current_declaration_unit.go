package postgres

// currentDeclarationUnitCTE 是「对象当前所在的申报单元」的唯一一处 SQL 定义，供按对象反查本上下文册子的读面拼进
// 自己的 WITH 子句（参数约定：$1 租户、$2 对象引用）。门禁、放行与申报事实说的「当前单元」是同一个判据，各读面
// 各写一遍，替代关系有了写入方、要加「有效替代」过滤时就会各改各的。
//
// 当前单元排掉被替代的那些（另一单元的 replaces_unit_id 指向它）。替代关系今天还没有写入方，这一句先按列的
// 语义写好，写入方来了读面不必改。
const currentDeclarationUnitCTE = `current_unit AS (
		     SELECT u.unit_id, u.case_id, u.procedure_ref
		       FROM customs_compliance.declaration_unit u
		      WHERE u.tenant_id = $1
		        AND u.members @> jsonb_build_array($2::text)
		        AND NOT EXISTS (
		                SELECT 1
		                  FROM customs_compliance.declaration_unit r
		                 WHERE r.tenant_id = u.tenant_id
		                   AND r.replaces_unit_id = u.unit_id
		            )
		 )`
