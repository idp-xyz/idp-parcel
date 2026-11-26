-- 库存读口翻页看流水：一个货主名下按入账时刻从新到旧取一页，再数同一组条件下一共几笔。
-- 流水只追加，是这个上下文里长得最快的表；既有的键没有一只以（租户，货主）开头，按货主取要扫整个租户的流水再排序。
-- 列序与读口的排序一致（入账时刻，再流水标识），取页与键集比较都走这一只。
--
-- 不用 CONCURRENTLY：本仓迁移执行器只收能在事务块内运行的迁移（migrate.applyStep 头注写明了这一条显式未决）。建索引期间
-- stock_movement 的写口要等。

CREATE INDEX stock_movement_owner_posted_idx
    ON warehouse_fulfillment.stock_movement (tenant_id, owner_account_id, posted_at DESC, movement_id DESC);
