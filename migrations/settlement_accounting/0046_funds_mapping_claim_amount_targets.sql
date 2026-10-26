-- 资金映射加两格目标：客户赔付（customer_claim_amount 那一条，赔付义务或索赔费用退款）与追偿认可金额
-- （recovery_acknowledgement 那一条）。映射与核销只改核销状态与未分配余额，不改被指向的金额本体，所以金额册
-- 一列不加。既有四格原样在集合里，存量行照旧满足新 CHECK。

ALTER TABLE settlement_accounting.funds_mapping
    DROP CONSTRAINT funds_mapping_target_kind_closed;

ALTER TABLE settlement_accounting.funds_mapping
    ADD CONSTRAINT funds_mapping_target_kind_closed
        CHECK (target_kind IN ('STATEMENT', 'PAYABLE', 'CREDIT_NOTE', 'ACCOUNT',
                               'CUSTOMER_COMPENSATION', 'RECOVERY_RECEIVABLE'));
