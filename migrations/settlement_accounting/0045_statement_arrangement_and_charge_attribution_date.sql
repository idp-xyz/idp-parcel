-- 对账安排登记册、费用项目一册只留形态、客户费用带来源发生时点与归属日（ADR-0269）。
--
-- 对账安排：按（租户、结算账户）登业务时区与截单时刻，排版次，读口取最新一版。新一版只作用于此后形成的归属日，
-- 旧版留在册上。截单时刻是当天的分钟数，0 是午夜这一份登记。对账周期的日历不在这本册里。账户不在账户册上
-- 由外键拒。不登任何租户的行，不预填时区或截单时刻。
CREATE TABLE settlement_accounting.statement_arrangement (
    tenant_id      text        NOT NULL,
    account_id     text        NOT NULL,
    revision       integer     NOT NULL,
    timezone_name  text        NOT NULL,
    cutoff_minute  integer     NOT NULL,
    registered_at  timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT statement_arrangement_pkey
        PRIMARY KEY (tenant_id, account_id, revision),

    CONSTRAINT statement_arrangement_account_fkey
        FOREIGN KEY (tenant_id, account_id)
        REFERENCES settlement_accounting.settlement_account (tenant_id, account_id),

    CONSTRAINT statement_arrangement_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(account_id) <> ''
            AND btrim(timezone_name) <> ''
        ),

    CONSTRAINT statement_arrangement_revision_positive
        CHECK (revision >= 1),

    CONSTRAINT statement_arrangement_cutoff_minute
        CHECK (cutoff_minute BETWEEN 0 AND 1439)
);

-- 费用项目一册只登形态：时区与截单时刻在对账安排上，这里再存一份就是第二权威。这一册没有种子行，不搬数。
ALTER TABLE settlement_accounting.charge_attribution
    DROP CONSTRAINT charge_attribution_scope_not_blank,
    DROP CONSTRAINT charge_attribution_cutoff_minute,
    DROP COLUMN timezone_name,
    DROP COLUMN cutoff_minute;

ALTER TABLE settlement_accounting.charge_attribution
    ADD CONSTRAINT charge_attribution_scope_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(fee_item) <> '');

-- 来源发生时点：归属日形态为来源发生时取它，形成费用时从来源事实钉上。归属日：随确认形成，确认时判不出就留空，
-- 截单时当场判、判出即落；只在已确认行上有。两列可空：判不出是一种合法状态，不填占位日。
ALTER TABLE settlement_accounting.customer_charge
    ADD COLUMN source_occurred_at timestamptz,
    ADD COLUMN attribution_date date,
    ADD CONSTRAINT customer_charge_attribution_only_when_confirmed
        CHECK (attribution_date IS NULL OR stage = 'CONFIRMED');

-- 存量费用都出自包裹计费时点那一路，它的 SELL 请求上记着计价基准时点，即这件包裹的责任起点或终局的生效时点
-- （ADR-0190 决定二）。按请求回指补上；对不上请求的行留空，形态为来源发生的归属日对它判不出，不另找时点顶上。
UPDATE settlement_accounting.customer_charge AS charge
   SET source_occurred_at = request.pricing_basis_at
  FROM settlement_accounting.sell_evaluation_request AS request
 WHERE request.tenant_id = charge.tenant_id
   AND request.customer_charge_id = charge.charge_id
   AND charge.source_occurred_at IS NULL;

-- 截单按费用找它在不在一张未作废的已发布对账单上：行是 jsonb 数组，按包含查。
CREATE INDEX customer_statement_lines_idx
    ON settlement_accounting.customer_statement USING gin (lines jsonb_path_ops);
