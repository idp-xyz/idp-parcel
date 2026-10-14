-- 应收结算账户成为资金映射的第四个目标（ADR-0182 决定一）。费用本体仍不是映射目标。
-- 已确认客户费用补可空到期日（决定三）：没有这一列就答不出逾期，信用状况行不能靠猜。

ALTER TABLE settlement_accounting.funds_mapping
    DROP CONSTRAINT funds_mapping_target_kind_closed;

ALTER TABLE settlement_accounting.funds_mapping
    ADD CONSTRAINT funds_mapping_target_kind_closed
        CHECK (target_kind IN ('STATEMENT', 'PAYABLE', 'CREDIT_NOTE', 'ACCOUNT'));

ALTER TABLE settlement_accounting.customer_charge
    ADD COLUMN due_date date;
