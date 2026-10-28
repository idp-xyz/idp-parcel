-- 供应商账单接收与审核应付的操作证据（ADR-0340 决定三；票 product-strategy-boundary/22 第 3 笔）：租户的结算人员经运营决定口接收一份
-- 账单并裁决逐行匹配、或点下一行的审核时，记下经哪个渠道、由谁提交（操作者渠道上是 `OPERATOR/<发行方>#<sub>`）。UC-SA-004 审计
-- 要记审核人与决定人；audited_payable.auditor_ref 是审核授权册上登的那一方，不是点下这一次的人。
--
-- 只作审计，判据同 0041 那一列：不进 content_digest，不进重放比对，也不进任何门——同一份由另一位操作者重放答已存在，留下的仍是首次
-- 那一位。可空：受控登记口与存量行一律 NULL，读作「这一份不记提交方」。
ALTER TABLE settlement_accounting.supplier_bill_reception
    ADD COLUMN operation_evidence text,
    ADD CONSTRAINT supplier_bill_reception_operation_evidence_not_blank
        CHECK (operation_evidence IS NULL OR btrim(operation_evidence) <> '');

ALTER TABLE settlement_accounting.audited_payable
    ADD COLUMN operation_evidence text,
    ADD CONSTRAINT audited_payable_operation_evidence_not_blank
        CHECK (operation_evidence IS NULL OR btrim(operation_evidence) <> '');
