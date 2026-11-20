-- 收货事实到达时预报已取消或已完结：原事实保全为一件待认领到货。
-- 到货标识由本上下文签发。原事实的内容仍在收货事实表，本表指向那条事实，并记下原预报与货主线索。
-- 货主只是线索。不入商品库存，也不入整箱库存。认领与处置不在本表。
--
-- 早先把这种事实写成 NOT_RECEIVABLE。改记成 DIVERTED_TO_UNCLAIMED，好让结果与现在的答法同一词。
-- 那些旧行当时没有到货，本迁移不补建。
--
-- 本表不对预报修订表建外键，只指向本上下文自己的收货事实。
-- 本迁移不种任何新行。

UPDATE warehouse_fulfillment.receipt_confirmation
   SET outcome = 'DIVERTED_TO_UNCLAIMED'
 WHERE outcome = 'NOT_RECEIVABLE';

ALTER TABLE warehouse_fulfillment.receipt_confirmation
    DROP CONSTRAINT receipt_confirmation_outcome;

ALTER TABLE warehouse_fulfillment.receipt_confirmation
    ADD CONSTRAINT receipt_confirmation_outcome
        CHECK (outcome IN ('CONFIRMED', 'DIVERTED_TO_UNCLAIMED'));

CREATE TABLE warehouse_fulfillment.unclaimed_arrival (
    tenant_id        text NOT NULL,
    arrival_id       text NOT NULL,
    fact_id          text NOT NULL,
    pre_advice_id    text NOT NULL,
    owner_account_id text NOT NULL,

    CONSTRAINT unclaimed_arrival_pkey
        PRIMARY KEY (tenant_id, arrival_id),

    CONSTRAINT unclaimed_arrival_fact_key
        UNIQUE (tenant_id, fact_id),

    CONSTRAINT unclaimed_arrival_fact_fkey
        FOREIGN KEY (tenant_id, fact_id)
        REFERENCES warehouse_fulfillment.receipt_confirmation (tenant_id, fact_id),

    CONSTRAINT unclaimed_arrival_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(arrival_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(pre_advice_id) <> ''
            AND btrim(owner_account_id) <> ''
        )
);

CREATE INDEX unclaimed_arrival_pre_advice_idx
    ON warehouse_fulfillment.unclaimed_arrival (tenant_id, pre_advice_id);
