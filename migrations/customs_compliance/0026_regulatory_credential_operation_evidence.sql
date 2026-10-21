-- 监管凭证版本的操作证据（ADR-0236 决定三；票 demo-chain-gaps/140）：租户关务人员经管理台照凭证原件手工登记一版凭证时，记下经
-- 哪个渠道、由谁登记（操作者渠道上是 `OPERATOR/<发行方>#<sub>`）。
--
-- 只作审计：不进 CCC-1 的内容比对，也不进任何门——凭证适用性判断读不到它；同一身份由另一位操作者以同内容重登答已存在，留下的仍是
-- 首次那一位。可空：集成客户端族与受控登记 CLI 两路不记登记人，存量行与这两路新落的行一律 NULL，NULL 读作「这一版不记登记人」，
-- 不读作「无人登记」。
ALTER TABLE customs_compliance.regulatory_credential
    ADD COLUMN operation_evidence text,
    ADD CONSTRAINT regulatory_credential_operation_evidence_not_blank
        CHECK (operation_evidence IS NULL OR btrim(operation_evidence) <> '');
