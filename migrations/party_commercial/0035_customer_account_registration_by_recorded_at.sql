-- 货主客户账户目录读口迁到 ADR-0144（票 catalogue-read-pagination/02）：缺省序是 -registeredAt——登记时刻倒序，
-- 决胜键账户号与之同向。ADR-0144 决定七要求每迁一册，登记表补一条与缺省序同形的索引（租户 + 排序维 + 标识）。
--
-- 上列先把每个账户折叠到最新修订再排，这条索引给「按登记时刻倒序扫一个租户的登记行」一条现成的路；折叠本身
-- 仍按主键（租户 + 账户 + 修订）走。按租户读、租户级规模下这样够用（ADR-0144 越权风险点 3），量级上来要回到
-- 那份记录重估。只建索引，不动数据。
CREATE INDEX customer_account_registration_by_recorded_at
    ON party_commercial.customer_account_registration (tenant_id, recorded_at DESC, account_id DESC);
