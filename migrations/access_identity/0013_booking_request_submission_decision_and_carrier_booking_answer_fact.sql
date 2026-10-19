-- 订舱提交与承运答复的两格授予单位（ADR-0202 决定一、二；票 demo-chain-gaps/24）。
--
-- 「运营决定」能力面加一种决定：订舱提交。运营人员针对一份已提交的委托就一个在册具体班次申请舱位，是管理台上作出的生产
-- 决定，授予按这一种单独给（授予粒度等于权力粒度）。与 accessidentity.DecisionBookingRequestSubmission 同笔放开（0001 头注
-- 那条约定）。
--
-- 集成客户端册加一类外部事实：承运订舱答复。提交方是承运系统，所以归集成客户端族而不归运营决定；与
-- accessidentity.FactCarrierBookingAnswer 同笔放开（0003 头注那条约定）。
--
-- 本迁移不种任何行：谁被授予哪一种决定、哪个客户端被授予哪一类事实，是租户取值。
--
-- 决定种类的全集取此前各份重建的并集（0011 头注那条约定）：迁移不按编号先后施加，本份可能晚于编号更大的一份施加，只列
-- 本份基线上的全集会把后来的种类删掉、让它们的授予登不进。
--
-- 全集另含 CAPACITY_RESERVATION、CAPACITY_RELEASE：ADR-0203 决定三为容量预占与释放定下这两个词，由同一波的另一张票随
-- 它自己的迁移 0012 加入。新库按编号施加时本份在 0012 之后，不列这两种就会把它们删掉——与哪份先进 main 无关。多列此刻
-- Go 侧可能还没定义的词无害：授予册只经领域词表写入，库面这条 CHECK 只守「不是封闭集外的串」。

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
            'IDENTITY_CONFLICT_DISPOSITION',
            'BOOKING_REQUEST_SUBMISSION',
            'CAPACITY_RESERVATION', 'CAPACITY_RELEASE'
        ));

ALTER TABLE access_identity.integration_client_grant
    DROP CONSTRAINT integration_client_grant_fact_type_decided;

ALTER TABLE access_identity.integration_client_grant
    ADD CONSTRAINT integration_client_grant_fact_type_decided
        CHECK (fact_type IN (
            'CUSTOMS_EXTERNAL_RESULT', 'REGULATORY_CREDENTIAL', 'CARRIER_TRACKING',
            'CARRIER_FIRST_EFFECTIVE_PICKUP_EVIDENCE', 'EXTERNAL_FUNDS_FACT',
            'CARRIER_BOOKING_ANSWER'
        ));
