-- 授权动作封闭集加「仓配代办」（ADR-0371）：WAREHOUSE_FULFILLMENT_AGENCY。0032 重建过的
-- authorization_grant_action_closed 把当时的字面量钉在 CHECK 里。不改已施加的迁移，这里 DROP 再 ADD
-- 同名约束。不预填任何行：哪个货主获准代办是租户取值，缺席答未配置。
--
-- contract_delegation 的 action CHECK 不扩。仓配代办的实际决定方是货主客户账户，合同委派不挂它。

ALTER TABLE party_commercial.authorization_grant
    DROP CONSTRAINT authorization_grant_action_closed;

ALTER TABLE party_commercial.authorization_grant
    ADD CONSTRAINT authorization_grant_action_closed
        CHECK (action IN (
            'MANUAL_REVIEW',
            'ACTIVE_REJECTION',
            'SOURCE_DATA_AMENDMENT',
            'CONTROLLED_CLOSURE',
            'REOPENING',
            'WAREHOUSE_FULFILLMENT_AGENCY'
        ));
