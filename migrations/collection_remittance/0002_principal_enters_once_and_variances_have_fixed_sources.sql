-- 代收本金只凭渠道代收报告入账一次，凭代收事实的记账按依据至多一笔，差额的来源位置
-- 限死（ADR-0271）。与领域构造门 domain.RecordSubledgerPosting 是同一份规则的两处落点，
-- 两处同笔改：只写在编排里的那半，绕过编排的写入在库里看不出守没守。
--
-- 本迁移只加约束与索引，不改任何已落行。已有记账若违反其中一条（按 0001 的旧口径落下
-- 的重复入账、自待清分落入的短款），ADD CONSTRAINT 与唯一索引会在施加时失败、整笔
-- 迁移回滚——那正是要人回头看的账：哪一笔是重的，库自己判不了，不能替它挑一笔删掉。
-- 演示租户的存量（报告入在途、到账移位、短款自在途落入）满足全部各条。

-- 一条代收事实至多支撑一笔记账：报告一笔入账，到账一笔移位（ADR-0271 决定三）。按
-- （租户，依据引用）唯一，只管凭代收事实的那一类——清分、汇付、差额与冲正的依据本就
-- 允许支撑多笔。写口的 ON CONFLICT DO NOTHING 不带冲突目标，撞上这条同样折成`已登记`，
-- 编排按记账标识读不回同一笔，答内容冲突。
CREATE UNIQUE INDEX subledger_posting_one_per_collection_fact
    ON collection_remittance.subledger_posting (tenant_id, basis_ref)
    WHERE basis_kind = 'COLLECTION_FACT';

-- 入账只入渠道在途：只有渠道代收报告自外部来源入账，而它的意思就是钱在渠道手上。
ALTER TABLE collection_remittance.subledger_posting
    ADD CONSTRAINT subledger_posting_intake_lands_in_transit
        CHECK (from_position <> 'EXTERNAL_SOURCE' OR to_position = 'IN_TRANSIT_AT_CHANNEL');

-- 凭代收事实只有两种形：报告入账，与到账把渠道在途移到待清分。凭的是哪一层事实这里
-- 看不见（要跨表），由编排读回事实核；库上只封住形。
ALTER TABLE collection_remittance.subledger_posting
    ADD CONSTRAINT subledger_posting_collection_fact_shapes
        CHECK (
            basis_kind <> 'COLLECTION_FACT'
            OR (from_position = 'EXTERNAL_SOURCE' AND to_position = 'IN_TRANSIT_AT_CHANNEL')
            OR (from_position = 'IN_TRANSIT_AT_CHANNEL' AND to_position = 'AWAITING_ALLOCATION')
        );

-- 短款只从渠道在途落入：差额那部分钱从没回到运营企业，从待清分或应付客户挪就是挪走
-- 已到账的本金。溢款只从待清分落入：它之后能归到应付客户，只能装已经到账的钱。
ALTER TABLE collection_remittance.subledger_posting
    ADD CONSTRAINT subledger_posting_shortfall_from_in_transit
        CHECK (to_position <> 'SHORTFALL' OR from_position = 'IN_TRANSIT_AT_CHANNEL');

ALTER TABLE collection_remittance.subledger_posting
    ADD CONSTRAINT subledger_posting_surplus_from_awaiting_allocation
        CHECK (to_position <> 'SURPLUS' OR from_position = 'AWAITING_ALLOCATION');

-- 按指令判上限要读一条指令名下的全部事实；按指令取事实是本表新增的读法。
CREATE INDEX collection_fact_by_instruction
    ON collection_remittance.collection_fact (tenant_id, instruction_ref);
