-- 「运营决定」能力面（ADR-0151 决定一；票 operator-channel/15）：委托侧五个运营决定口与 TF 在管理台上
-- 作决定或判断的各口，授予按租户 × 决定种类登记。与 accessidentity.CapabilityOperationDecision、
-- accessidentity.DecisionKind 同笔放开（0001 头注那条约定）。
--
-- 决定种类是授予的一列，不是另一张表：撤了再授仍是两笔授予、各带自己的种类，与别的能力面同一套
-- 只增不改的纪律。本迁移不种任何行。

ALTER TABLE access_identity.operator_grant
    ADD COLUMN decision_kind text;

ALTER TABLE access_identity.operator_grant
    DROP CONSTRAINT operator_grant_capability_face_decided;

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_capability_face_decided
        CHECK (capability_face IN ('REGISTRY_CONFIGURATION_WRITE', 'MASTER_DATA_AND_OPERATIONS_READ', 'OPERATION_DECISION'));

-- 「运营决定」一格必带种类，别的格必不带：不带种类的一笔等于把所有决定一并授出，带了种类的
-- 登记册配置写则是一行说不清在授什么的授予。
ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_decision_kind_matches_face
        CHECK ((capability_face = 'OPERATION_DECISION') = (decision_kind IS NOT NULL));

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_decision_kind_decided
        CHECK (decision_kind IS NULL OR decision_kind IN (
            'MANUAL_REVIEW_COMPLETION', 'ACTIVE_REJECTION', 'AUTHORIZED_DISPOSITION',
            'CONTROLLED_CLOSURE', 'CONTROLLED_REOPENING',
            'SEGMENT_CLOSURE', 'DISPATCH_TASK_REGISTRATION', 'LOAD_ASSIGNMENT', 'PARTICIPATION_TERMINATION',
            'EFFECTIVE_TIME_JUDGMENT', 'CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT'
        ));
