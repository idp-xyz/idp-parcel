-- 「操作者接入管理」能力面（ADR-0363 决定一；票 operator-access-administration/01）：守操作者登记、授予登记、撤销登记三个写面
-- 与本租户操作者册的查阅读，与既有各格并列、可授可撤带生效区间。与 accessidentity.CapabilityOperatorAccessAdministration 同笔放开
-- （0001 头注那条约定）。本迁移不种任何行：谁持有操作者接入管理是租户取值，租户的第一个管理授予经受控命令行登（决定四）。
--
-- 只重建能力面这一条 CHECK。全集取重建这条 CHECK 的各份迁移的并集，再加本份一个词；理由同 0039 对决定种类那条：迁移不按编号
-- 先后施加，只列本份基线上的全集会把后来的词删掉。这一格不带决定种类也不带作业范围，既有的两条配对 CHECK 已经如此约束。

ALTER TABLE access_identity.operator_grant
    DROP CONSTRAINT operator_grant_capability_face_decided;

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_capability_face_decided
        CHECK (capability_face IN (
            'REGISTRY_CONFIGURATION_WRITE', 'MASTER_DATA_AND_OPERATIONS_READ', 'OPERATION_DECISION',
            'WORK_FACT_REGISTRATION', 'OPERATOR_ACCESS_ADMINISTRATION'
        ));
