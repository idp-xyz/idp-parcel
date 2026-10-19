-- 「运营决定」能力面加两种决定：容量预占与容量释放（票 demo-chain-gaps/20，ADR-0203 决定三）。与
-- accessidentity.DecisionCapacityReservation、DecisionCapacityRelease 同笔放开（0001 头注那条约定）。本迁移不种任何行：
-- 谁被授予这两种决定是租户取值。
--
-- 重建 0002 立的决定种类 CHECK，列**此前各份重建的并集再加本份两种**：0007、0008、0009、0011 各重建过一次，每一份都是
-- 完整清单，不是增量。迁移不按编号先后施加（执行器按迁移标识补施加未施加的步），本份只列编号在前的几种，就会在补施加
-- 的那一刻删掉后来者的种类。
--
-- 全集另含 BOOKING_REQUEST_SUBMISSION：ADR-0202 决定一已为订舱提交定下这个词，由同一波的另一张票随它自己的迁移加入，
-- 那一份可能先于本份施加。多列一个此刻 Go 侧还没定义的词无害：授予册只经领域词表写入，库面这条 CHECK 只守「不是封闭集
-- 外的串」。之后谁再重建这条 CHECK，都取此前各份的并集。

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
