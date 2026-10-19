-- 「运营决定」能力面加一种决定：运输委托提交（ADR-0151 决定一；ADR-0202 决定一，票 demo-chain-gaps/23）。运营人员向外部
-- 服务提供方提出明确的运输范围与条件，是管理台上作决定的口。与 accessidentity.DecisionTransportCommissionSubmission 同笔放开
-- （0001 头注那条约定）。本迁移不种任何行：谁被授予这一种决定是租户取值。
--
-- 约束整条重建：CHECK 不能只追加一个值，漏抄任何一种，已登的那一类授予就再也写不进来。
--
-- 列的是各份重建的并集，不只是本份编号之前的全集：0011（身份冲突处置）编号在本份之后、却先进了 main，0009（运输收费
-- 发生项登记）与本份同期在途。迁移按「还没施加过的步骤一律补上」执行，不看编号先后，所以在已施加 0011 的库上本份是后
-- 施加的那一份；只列本份基线上的全集，就会把后来的那几种从库面删掉——表里已有那几种的授予时整个迁移计划中止，没有时
-- 则悄悄让之后的授予登不进。各份都列并集，无论谁先施加，终态都相同（0011 头注定下的同一约定）。多列 Go 侧此刻还没定义
-- 的词无害：授予册只经领域词表写入，库面这条 CHECK 只守「不是封闭集外的串」。之后谁再重建这条约束，照样取并集。

ALTER TABLE access_identity.operator_grant
    DROP CONSTRAINT operator_grant_decision_kind_decided;

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_decision_kind_decided
        CHECK (decision_kind IS NULL OR decision_kind IN (
            'MANUAL_REVIEW_COMPLETION', 'ACTIVE_REJECTION', 'AUTHORIZED_DISPOSITION',
            'CONTROLLED_CLOSURE', 'CONTROLLED_REOPENING',
            'SEGMENT_CLOSURE', 'DISPATCH_TASK_REGISTRATION', 'LOAD_ASSIGNMENT', 'PARTICIPATION_TERMINATION',
            'EFFECTIVE_TIME_JUDGMENT', 'CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT',
            'PENDING_IDENTITY_RECHECK',
            'TRANSPORT_COMMISSION_SUBMISSION', 'TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION',
            'IDENTITY_CONFLICT_DISPOSITION'
        ));
