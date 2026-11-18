-- 对账单可以只有调整行：费用出账之后才形成的调整，账期里没有新费用时照样出单，净额为负即贷记。
-- 费用行与调整行不能同时为空。
--
-- 后续纳入随带它的那张对账单作废而撤回。撤回只在原行上记时刻与依据，不删行；同一笔调整撤回之后可以再纳入一次，
-- 「一笔调整只纳入一次」只管没撤回的那些。本迁移不种任何租户的行。

ALTER TABLE settlement_accounting.customer_statement
    DROP CONSTRAINT customer_statement_lines_present;

ALTER TABLE settlement_accounting.customer_statement
    ADD CONSTRAINT customer_statement_lines_or_adjustments_present
        CHECK (
            jsonb_typeof(lines) = 'array'
            AND jsonb_typeof(adjustment_lines) = 'array'
            AND (jsonb_array_length(lines) > 0 OR jsonb_array_length(adjustment_lines) > 0)
        );

ALTER TABLE settlement_accounting.subsequent_inclusion
    ADD COLUMN withdrawn_at     timestamptz,
    ADD COLUMN withdrawal_basis text;

ALTER TABLE settlement_accounting.subsequent_inclusion
    ADD CONSTRAINT subsequent_inclusion_withdrawal_coherent
        CHECK (
            (withdrawn_at IS NULL AND withdrawal_basis IS NULL)
            OR (withdrawn_at IS NOT NULL AND withdrawal_basis IS NOT NULL AND btrim(withdrawal_basis) <> '')
        );

DROP INDEX settlement_accounting.subsequent_inclusion_one_per_adjustment;

CREATE UNIQUE INDEX subsequent_inclusion_one_per_adjustment
    ON settlement_accounting.subsequent_inclusion (tenant_id, adjustment_id)
    WHERE adjustment_id IS NOT NULL AND withdrawn_at IS NULL;
