-- 提交尝试的`已登记待发送`一格（ADR-0227 决定四；UC-CC-005「提交已登记待发送」；票 product-strategy-boundary/18）。
--
-- 提交执行口在固定提交版本的同一笔里登记首次尝试的身份与目标，外部发送还没开始：0002 的结果集合只有越过发送边界之后的三种
-- 已知结果，sent_at 又不许空，一份没发出去的尝试在这张表上只能被写成「待确认 + 某个发送时间」——那正是假装已发送。
--
-- 结果集合加 REGISTERED_PENDING_SEND；sent_at 放开为可空，并与结果配对：待发送那一格没有发送时间，其余三格必有。受控重发
-- （sequence > 1）本身是一次发送，不会停在待发送。既有行都是三种已知结果且 sent_at 非空，新两条 CHECK 对它们恒成立，无需回填。

ALTER TABLE customs_compliance.submission_attempt
    DROP CONSTRAINT submission_attempt_result_closed;

ALTER TABLE customs_compliance.submission_attempt
    ADD CONSTRAINT submission_attempt_result_closed
        CHECK (result IN ('ACKNOWLEDGED', 'FAILED', 'PENDING_CONFIRMATION', 'REGISTERED_PENDING_SEND'));

ALTER TABLE customs_compliance.submission_attempt
    ALTER COLUMN sent_at DROP NOT NULL;

ALTER TABLE customs_compliance.submission_attempt
    ADD CONSTRAINT submission_attempt_sent_shape
        CHECK ((result = 'REGISTERED_PENDING_SEND') = (sent_at IS NULL));

ALTER TABLE customs_compliance.submission_attempt
    ADD CONSTRAINT submission_attempt_resend_is_a_send
        CHECK (sequence = 1 OR result <> 'REGISTERED_PENDING_SEND');
