-- 数据可见范围（ADR-0385）：操作者看得见哪些客户账户、哪些仓，按授予登记。与
-- accessidentity.CapabilityDataVisibility、DataVisibilityKind 同笔放开。本迁移不种任何行：谁看得见什么是租户取值。
--
-- 只重建能力面这一条 CHECK。全集取 0040 那一份（它已并入此前各份），再加本份一个词。迁移不按编号先后施加，
-- 只列本份基线上的全集会把后来的词删掉。之后谁再重建这道 CHECK，都取这份全集再加自己的词。
--
-- 可见范围用自己的两列，不复用作业范围：作业范围的「不带等于全授」不能拿到这里（ADR-0385 决定二）。
-- 这一格必带种类与引用，别的格必不带。引用 * 是这一类全部，由领域认，库只要求非空白。

ALTER TABLE access_identity.operator_grant
    ADD COLUMN visibility_kind text,
    ADD COLUMN visibility_ref  text;

ALTER TABLE access_identity.operator_grant
    DROP CONSTRAINT operator_grant_capability_face_decided;

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_capability_face_decided
        CHECK (capability_face IN (
            'REGISTRY_CONFIGURATION_WRITE', 'MASTER_DATA_AND_OPERATIONS_READ', 'OPERATION_DECISION',
            'WORK_FACT_REGISTRATION', 'OPERATOR_ACCESS_ADMINISTRATION', 'DATA_VISIBILITY'
        ));

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_visibility_matches_face
        CHECK (
            ((capability_face = 'DATA_VISIBILITY') = (visibility_kind IS NOT NULL))
            AND ((visibility_kind IS NULL) = (visibility_ref IS NULL))
        );

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_visibility_decided
        CHECK (
            visibility_kind IS NULL
            OR (visibility_kind IN ('CUSTOMER_ACCOUNT', 'WAREHOUSE') AND btrim(visibility_ref) <> '')
        );
