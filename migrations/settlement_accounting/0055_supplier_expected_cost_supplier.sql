-- 供应商预期成本记下所依协议的供应商（ADR-0358；票 document-numbering/14）。
--
-- supplier_party_ref 是 agreement_ref 那一版协议的供应商（往来单位标识，与账单接收的 supplier_ref 同一种值），由形成编排在形成
-- 首版与纠错版本时经消费侧适配器问 party-commercial 得来，失效纠错版本照搬被纠正那一版的。匹配门凭它判账单行点名的成本是不是
-- 这张账单的供应商的。
--
-- 可空、不回填：本迁移之前形成的行没有这一格，读作「未记供应商」，匹配门对它答未记、不放过。回填要读
-- party_commercial.supplier_agreement，那是另一个上下文的表，结算的迁移不读它；旧行由补记口
-- （parcel-settlement-register expected-cost-supplier-backfill）经同一只适配器问过之后只在这一格为空时写下。已发布协议版本的正文
-- 不可覆盖，所以补记问到的就是形成时会问到的那一家。
--
-- 施加在有数据的库上：只加一列可空列与一道只管非空值的检查，不改任何已有行。另建一个只含未记行的部分索引，给补记口按租户
-- 列出它们用；补完之后它是空的。

ALTER TABLE settlement_accounting.supplier_expected_cost
    ADD COLUMN supplier_party_ref text;

ALTER TABLE settlement_accounting.supplier_expected_cost
    ADD CONSTRAINT supplier_expected_cost_supplier_not_blank
        CHECK (supplier_party_ref IS NULL OR btrim(supplier_party_ref) <> '');

CREATE INDEX supplier_expected_cost_awaiting_supplier
    ON settlement_accounting.supplier_expected_cost (tenant_id, version)
    WHERE supplier_party_ref IS NULL;
