-- 按成员反查它曾被直接装入哪几件集运单元的读面（票 docs-consistency/code/74，ADR-0299）：全程追踪在一件作业实物后来识别成功时，
-- 要回头找它随袋走过的移动事实，需要问节点作业「这件实物、这个嵌套单元曾被装进哪几件单元」。集运作业事实层的既有索引只按单元
-- （0003 的 consolidation_fact_by_unit），按成员问会扫全租户的事实；读面按成员问，索引也按成员立。
--
-- 只加索引，不动表形状与约束：读面读的是既有事实。成员列在移入、移出两格才在场，其余格上是 NULL，不进索引。
--
-- 不用 CONCURRENTLY：本仓迁移执行器只收能在事务块内运行的迁移（migrate.applyStep 头注写明了这一条显式未决）。建索引期间
-- consolidation_fact 的写口要等；实测 100 万行约 0.5 秒。
CREATE INDEX consolidation_fact_by_member
    ON node_operations.consolidation_fact (tenant_id, member_id, unit_id)
    WHERE member_id IS NOT NULL;
