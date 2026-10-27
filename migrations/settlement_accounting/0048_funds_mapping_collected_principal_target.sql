-- 资金映射加一格目标：整条认作代收本金（COLLECTED_PRINCIPAL）。认作的是确认到账的外部资金事实自己，目标引用就是这条
-- 事实的引用；映射只回答「这条回款属于谁」，不是核销对象，所以映射表与核销表的其余列都不加。既有六格原样在集合里，存量行
-- 满足新 CHECK。
--
-- 库上封三件：目标种类词表、认作的目标等于事实自己（CHECK）、一条事实至多一条认作（唯一索引）；后两道兜住绕过映射编排与
-- 事实锁的写入。与别的映射互斥、只认确认到账的事实，这两条要读事实或同事实上的别的映射行，库上看不见，由编排在事实身份行的
-- 锁下判（MapExternalFundsHandler.Map）、领域构造门判（domain.MapFundsToTarget）。

ALTER TABLE settlement_accounting.funds_mapping
    DROP CONSTRAINT funds_mapping_target_kind_closed;

ALTER TABLE settlement_accounting.funds_mapping
    ADD CONSTRAINT funds_mapping_target_kind_closed
        CHECK (target_kind IN ('STATEMENT', 'PAYABLE', 'CREDIT_NOTE', 'ACCOUNT',
                               'CUSTOMER_COMPENSATION', 'RECOVERY_ACKNOWLEDGEMENT',
                               'COLLECTED_PRINCIPAL'));

-- 目标等于事实自己：库上能封的一条——认作代收本金的映射，目标引用必须等于它所映射的事实引用。
ALTER TABLE settlement_accounting.funds_mapping
    ADD CONSTRAINT funds_mapping_collected_principal_names_the_fact
        CHECK (target_kind <> 'COLLECTED_PRINCIPAL' OR target_ref = fact_id);

-- 一条事实至多一条代收本金映射：认作是事实一级的状态，不是可叠加的金额。编排在事实锁下判；这条索引兜住绕过锁的写入。
CREATE UNIQUE INDEX funds_mapping_one_collected_principal_per_fact
    ON settlement_accounting.funds_mapping (tenant_id, fact_id)
    WHERE target_kind = 'COLLECTED_PRINCIPAL';
