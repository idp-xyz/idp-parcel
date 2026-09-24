-- 货主客户账户目录读口迁到 ADR-0144（票 catalogue-read-pagination/02）：缺省序是 -registeredAt——登记时刻倒序，
-- 决胜键账户号与之同向。ADR-0144 决定七要求每迁一册，登记表补一条与缺省序同形的索引（租户 + 排序维 + 标识）。
--
-- 眼下的读面先按主键（租户 + 账户 + 修订）把每个账户折叠到最新修订再排，外层的排序穿不过折叠，取页与计数都用不上
-- 这条索引：它按决定七补齐了形状，还没有读者。按修订折叠的册该配什么索引（服务折叠的索引，或最新修订投影），与
-- ADR-0144 越权风险点 3 同一前提，量级上来时回到那份记录一并定。只建索引，不动数据。
CREATE INDEX customer_account_registration_by_recorded_at
    ON party_commercial.customer_account_registration (tenant_id, recorded_at DESC, account_id DESC);
