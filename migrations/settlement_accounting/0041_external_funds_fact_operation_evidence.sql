-- 外部资金事实版本的操作证据（ADR-0236 决定三；票 demo-chain-gaps/140）：租户财务人员经管理台手工采用一版资金事实时，记下经哪个
-- 渠道、由谁提交（操作者渠道上是 `OPERATOR/<发行方>#<sub>`）。
--
-- 只作审计：不进 content_digest，不进重放比对，也不进任何门——同一版本由另一位操作者重放答已存在，留下的仍是首次那一位。可空：
-- 集成客户端族与受控登记 CLI 两路不记提交方，存量行与这两路新落的行一律 NULL，NULL 读作「这一版不记提交方」，不读作「无人提交」。
-- 列落在版本行而不是身份行：首版与每次更正可能出自不同的人，每一版各记各的。
ALTER TABLE settlement_accounting.external_funds_fact_version
    ADD COLUMN operation_evidence text,
    ADD CONSTRAINT external_funds_fact_version_operation_evidence_not_blank
        CHECK (operation_evidence IS NULL OR btrim(operation_evidence) <> '');
