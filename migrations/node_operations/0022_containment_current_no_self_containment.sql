-- 集运包含不得成环（NO CONTEXT「集运包含关系必须无循环」，UC-NO-003 `AT-NO-039`；票 docs-consistency/code/19）。
--
-- 环里最短的一种是单元装进自己：容纳索引一行之内（member_id = unit_id）就判得出，这一条 CHECK 守它，绕过适配器
-- 直接写的一行也进不去。更长的环跨行——A 在 B 里、B 在 A 里是两行，各看各的都合法——CHECK 引用不了别的行，
-- 库面守不住；那一半由移入编排沿直接父级链路上溯判断，并以租户级事务锁把并发移入串起来（适配器
-- ConsolidationUnits.LineageForUpdate）。不写触发器：本仓的业务不变量由领域与编排表达，库面只钉行内形状。
--
-- 既有行若已自装，本迁移会直接失败并点名约束，而不是悄悄放过：那样的行是环，没有任何解读能让它合法。

ALTER TABLE node_operations.containment_current
    ADD CONSTRAINT containment_current_not_self_contained
        CHECK (member_id <> unit_id);
