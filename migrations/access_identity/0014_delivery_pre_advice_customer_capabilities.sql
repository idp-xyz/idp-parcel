-- 客户面能力加两个词：送站预报提交、送站预报撤回（ADR-0212 决定二；票 ps-external-mark-relations/09）。照首方客户族「一口一词」
-- （票 first-party-api/00 完成记录「代决」一节），与 accessidentity.CustomerCapability 同笔放开（0010 里这条 CHECK 的注释「两处同笔
-- 改」）。本迁移不种任何行：哪个客户端被授予这两格是租户取值（参数登记册 PAR-INT-13）。
--
-- 只重建客户面能力这一条 CHECK；决定种类、外部事实类型与准入范围对照各条都不动。全集取此前各份迁移给出的客户面能力加本份两个
-- 词。之后谁再重建这条 CHECK，都取此前各份的并集：一份编号在前、却晚进 main 的迁移若也改了这条，按编号施加时本份排在它之后，
-- 只取本份基线上的全集就会把它的词删掉、让那几格授予登不进。

ALTER TABLE access_identity.customer_client_grant
    DROP CONSTRAINT customer_client_grant_capability_decided;

ALTER TABLE access_identity.customer_client_grant
    ADD CONSTRAINT customer_client_grant_capability_decided
        CHECK (capability IN (
            'SHIPMENT_REQUEST_SUBMISSION',
            'DELIVERY_PRE_ADVICE_SUBMISSION', 'DELIVERY_PRE_ADVICE_WITHDRAWAL'
        ));
