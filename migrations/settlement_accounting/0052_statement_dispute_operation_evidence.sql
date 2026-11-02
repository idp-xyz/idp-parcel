-- 客户对账单争议的操作证据（ADR-0354 决定四；票 home-today-todo/03）：客服照客户来电或来信经运营决定口代登记一项争议、财务核实后
-- 形成处理时，各记下经哪个渠道、由谁提交（操作者渠道上是 `OPERATOR/<发行方>#<sub>`）。UC-SA-003 审计要保留争议方与决定方；
-- 争议方是客户，代登记的是这里的那个人。
--
-- 只作审计，判据同 0050 那两列：不进 content_digest，不进重放比对，也不进任何门——同一项由另一位操作者重放答已存在，留下的仍是首次
-- 那一位。处理那一列随裁定三件一起被待复核之后的再处理换掉，与裁定三件同一个口径。可空：编排直调与存量行一律 NULL，读作「这一份
-- 不记提交方」。
ALTER TABLE settlement_accounting.statement_dispute
    ADD COLUMN operation_evidence text,
    ADD COLUMN resolution_evidence text,
    ADD CONSTRAINT statement_dispute_operation_evidence_not_blank
        CHECK (operation_evidence IS NULL OR btrim(operation_evidence) <> ''),
    -- 没有裁定就没有处理人；等号碰上 NULL 给 NULL，先 IS NULL。
    ADD CONSTRAINT statement_dispute_resolution_evidence_follows_resolution
        CHECK (resolution_evidence IS NULL
               OR (btrim(resolution_evidence) <> '' AND resolution IS NOT NULL));
