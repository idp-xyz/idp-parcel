-- 发生项有效性修订驱动供应商预期成本的计价纠错（ADR-0232 决定七、八；票 demo-chain-gaps/15）。
--
-- 失效纠错版本：一行预期成本可以是「它依的发生项已失效、这份成本不再成立」的那一版。计价结果原样带过去（零是一个价格，失效不是），
-- 只多一格 withdrawn 说它不再成立；它必回指被它纠正的那一版。既有的行都不是。
ALTER TABLE settlement_accounting.supplier_expected_cost
    ADD COLUMN withdrawn boolean NOT NULL DEFAULT false;

ALTER TABLE settlement_accounting.supplier_expected_cost
    ADD CONSTRAINT supplier_expected_cost_withdrawn_corrects
        CHECK (NOT withdrawn OR prior_version IS NOT NULL);

-- 一版至多被纠正一次：一份成本的版本链是一条线，链头只有一个。失效纠错与替代纠错若都回指同一版，经营口径就说不出哪一版是当前有效的
-- 预期成本。两者在同一把发生项锁下判（SupplierExpectedCosts.LoadOccurrenceCostState），这条索引是库面那一道。
CREATE UNIQUE INDEX supplier_expected_cost_corrected_once
    ON settlement_accounting.supplier_expected_cost (tenant_id, prior_version)
 WHERE prior_version IS NOT NULL;

-- 结算记下的「这一项发生项的这一版已失效」：失效信封到时这一版可能还没有成本行（它的评价还在路上），之后才到的评价不该再按它形成成本。
-- 一行是（租户、发生项、失效的那一版），记着是哪一版失效版本说的。只插不改；同一封信重投落回同一行。
CREATE TABLE settlement_accounting.supplier_cost_occurrence_invalidation (
    tenant_id            text        NOT NULL,
    occurrence_id        text        NOT NULL,
    invalidated_version  text        NOT NULL,
    invalidating_version text        NOT NULL,
    recorded_at          timestamptz NOT NULL,
    inserted_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT supplier_cost_occurrence_invalidation_pkey
        PRIMARY KEY (tenant_id, occurrence_id, invalidated_version),

    CONSTRAINT supplier_cost_occurrence_invalidation_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(occurrence_id) <> ''
            AND btrim(invalidated_version) <> ''
            AND btrim(invalidating_version) <> ''
            AND invalidated_version <> invalidating_version
        )
);
