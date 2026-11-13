-- 集成客户端册加一类外部事实：外部监管舱单引用（UC-CC-012）。
--
-- 监管舱单由责任承运商在产品之外形成并提交，本产品只收它的引用。提交方是承运商的系统，所以归集成客户端族；与监管结果
-- （CUSTOMS_EXTERNAL_RESULT）分开立，能回传监管结果的客户端不因此能替承运商报舱单。与
-- accessidentity.FactExternalRegulatoryManifest 同笔放开（0003 头注那条约定）。
--
-- 本迁移不种任何行：哪家承运商的哪只客户端被授予这一类事实、绑定哪个来源身份，是租户取值。
--
-- 事实类型的全集取此前各份重建的并集（0011 头注那条约定）：迁移不按编号先后施加，只列本份基线上的全集会把后来的类型
-- 删掉、让它们的授予登不进。

ALTER TABLE access_identity.integration_client_grant
    DROP CONSTRAINT integration_client_grant_fact_type_decided;

ALTER TABLE access_identity.integration_client_grant
    ADD CONSTRAINT integration_client_grant_fact_type_decided
        CHECK (fact_type IN (
            'CUSTOMS_EXTERNAL_RESULT', 'REGULATORY_CREDENTIAL', 'CARRIER_TRACKING',
            'CARRIER_FIRST_EFFECTIVE_PICKUP_EVIDENCE', 'EXTERNAL_FUNDS_FACT',
            'CARRIER_BOOKING_ANSWER',
            'EXTERNAL_REGULATORY_MANIFEST'
        ));
