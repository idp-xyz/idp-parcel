-- 客户费用补齐确认时必须固定的七项事实（SA CONTEXT「费用形成与证据」硬句：每条确认
-- 费用必须固定责任法人、结算相对方、收付方向、结算账户、合同或责任依据、结算币种、
-- 主要计费范围和来源事实，任何一项不能通过当前组织、当前客户属性或报表筛选临时推断）。
-- 形状照 ADR-0087 决定一。结算币种是那句里的第八项，已由 0013 的三件组成列，此处不重复。
--
-- 合同或责任依据不与 confirmation_basis 合用：后者是确认依据（哪份依据让它可以确认），
-- 前者是这笔钱依据哪份合同该收付，两者不同物，合用会让其中一个永远说不出口。来源事实
-- 同样不与 evaluation_ref 合用：评价是依据，来源事实是评价的输入。
--
-- 收付方向自立封闭词表 RECEIVABLE/PAYABLE，不复用本模块 recovery_adjustment_direction_closed
-- 的 DEBIT/CREDIT——那是借贷方向，与收付不是一回事。
--
-- 0003 与 0013 已施加不可改写（校验和把关），本文件以补列落地。表此刻无生产数据
-- （产品尚无租户），补列不涉及回填。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL。

ALTER TABLE settlement_accounting.customer_charge
    ADD COLUMN responsible_entity     text,
    ADD COLUMN counterparty_ref       text,
    ADD COLUMN charge_direction       text,
    ADD COLUMN settlement_account_id  text,
    ADD COLUMN contract_basis         text,
    ADD COLUMN primary_charging_scope text,
    ADD COLUMN source_fact_ref        text;

ALTER TABLE settlement_accounting.customer_charge
    ADD CONSTRAINT customer_charge_direction_closed
        CHECK (
            charge_direction IS NULL
            OR charge_direction IN ('RECEIVABLE', 'PAYABLE')
        ),

    -- 七项同在或同缺，形状照同表 customer_charge_confirmation_coupled。非确认行要求
    -- 七项全缺而不是「不作要求」：领域侧的 CustomerCharge 在确认之前根本表达不出这些
    -- 事实（ConfirmedFacts 只在已确认费用上给出），放行一条带事实的预估行等于开出一条
    -- 领域产不出也读不回的写入路径。CONTEXT 只约束确认费用，因此俱全这一侧只压在
    -- CONFIRMED 上——对预估与暂估强加俱全会让它们落不进册。
    ADD CONSTRAINT customer_charge_confirmation_facts_coupled
        CHECK (
            (stage <> 'CONFIRMED'
             AND responsible_entity IS NULL
             AND counterparty_ref IS NULL
             AND charge_direction IS NULL
             AND settlement_account_id IS NULL
             AND contract_basis IS NULL
             AND primary_charging_scope IS NULL
             AND source_fact_ref IS NULL)
            OR (stage = 'CONFIRMED'
                AND responsible_entity IS NOT NULL AND btrim(responsible_entity) <> ''
                AND counterparty_ref IS NOT NULL AND btrim(counterparty_ref) <> ''
                AND charge_direction IS NOT NULL
                AND settlement_account_id IS NOT NULL AND btrim(settlement_account_id) <> ''
                AND contract_basis IS NOT NULL AND btrim(contract_basis) <> ''
                AND primary_charging_scope IS NOT NULL AND btrim(primary_charging_scope) <> ''
                AND source_fact_ref IS NOT NULL AND btrim(source_fact_ref) <> '')
        );
