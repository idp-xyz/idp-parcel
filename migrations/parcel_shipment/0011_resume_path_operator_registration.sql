-- 库面镜像跟上领域封闭集合：ResumePath 增了`等待运营登记`（ADR-0094 Decision 二，PS CONTEXT
-- 同笔加了这一等待态），而 0005 与 0009 各自那条 CHECK 的注释都说自己「镜像领域 ResumePath 的
-- 封闭集合」——集合变了，镜像没跟。
--
-- 不跟的后果在真库上量过（票 first-tenant-runway/07）：第四格的处理尝试一落库就撞
-- acceptance_processing_attempt_resume_path_closed（SQLSTATE 23514），而 recordAttempt 按设计
-- 不上抛这一失败，于是事务已被标为 aborted、消费门随后的提交失败，一次如实的未决落成
-- dispatch.publish_failed——码面指错方向，痕一条不留。所以这一份不是放宽约束，是把镜像对齐。
--
-- 只动约束，不建索引：`等待运营登记`的队列读面属 ADR-0094 Decision 五那一片，到那时再按队列谓词
-- 建部分索引，形照 shipment_request_manual_review_queue。
--
-- 取值域镜像领域 ResumePath：1 = CUSTOMER_SUPPLEMENT，2 = INTERNAL_RETRY，3 = MANUAL_REVIEW，
-- 4 = OPERATOR_REGISTRATION；task_waiting_on 的 0 仍是「无等待」。

ALTER TABLE parcel_shipment.acceptance_processing_attempt
    DROP CONSTRAINT acceptance_processing_attempt_resume_path_closed,
    ADD CONSTRAINT acceptance_processing_attempt_resume_path_closed
        CHECK (resume_path IN (
            'CUSTOMER_SUPPLEMENT', 'INTERNAL_RETRY', 'MANUAL_REVIEW', 'OPERATOR_REGISTRATION'));

ALTER TABLE parcel_shipment.shipment_request
    DROP CONSTRAINT shipment_request_task_waiting_on_known,
    ADD CONSTRAINT shipment_request_task_waiting_on_known
        CHECK (task_waiting_on BETWEEN 0 AND 4);
