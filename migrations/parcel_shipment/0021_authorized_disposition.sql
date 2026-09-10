-- 授权处置进库面（ADR-0132，票 sa-preacceptance-policy-view/04）。三件事，各自镜像领域里已经落地的一格。
--
-- 一、库面镜像跟上领域封闭集合：ResumePath 增了`等待授权处置`（ADR-0132 决定二，PS CONTEXT 同笔加了这一
-- 等待态）。0005 与 0009 那两条 CHECK 各自注明「镜像领域 ResumePath 的封闭集合」，0011 对齐过一次第四格，
-- 这里对齐第五格——不对齐的后果 0011 的头注写过（第五格的处理尝试一落库就撞 CHECK，一次如实的未决落成
-- dispatch.publish_failed），adapters/postgres 那两条逐格遍历用例（TestEveryResumePathLandsInTheAttemptTable /
-- TestTaskWaitingOnProjectionMirrorsEveryResumePath）就是为在这里红而写的。
-- 取值域：1 = CUSTOMER_SUPPLEMENT，2 = INTERNAL_RETRY，3 = MANUAL_REVIEW，4 = OPERATOR_REGISTRATION，
-- 5 = AUTHORIZED_DISPOSITION；task_waiting_on 的 0 仍是「无等待」。
--
-- 二、授权处置队列的部分索引，形照 0009 / 0013 / 0016：队列的定义就是投影列上「已提交且等授权处置」
-- （state=1 即 SUBMITTED，waiting=5 即 AUTHORIZED_DISPOSITION），按提交时间序出队；读口照登记过滤，不在读侧重推。
--
-- 三、受限项的采用引用两列（ADR-0132 决定三、四）：失败处置与责任引用在形成控制判断那一步经 PS 自己的商业缝
-- 从策略正文读回、记在受限项上，Decide 与读面从已记录判断取、不重读正文。两列成对：处置在集内（镜像 PC 词汇，
-- 放宽时只改 CHECK 不给既有行默认）、责任引用非空。形状照领域 WithAdoptedDisposition：成立项两列必空（成立项
-- 没有去向可言）；受限项两列**同在或同缺**——同缺是 0021 之前的存量行与采用之前记下的行，领域把它们如实读回
-- 「未采用」、按 ADR-0125 的过渡口径译`未通过`，不补、不拒（重建门只校验不重算，ADR-0028）。半截（只有一列）
-- 不是任何一条路写得出的，拒。全部 CHECK 走 IS NULL / IS NOT NULL 缝（f822e1d 入册的三值逻辑纪律）。
--
-- 处置记录本身不另立表：它随接受判断任务住在 shipment_request.snapshot 里（acceptanceTask.authorizedDisposition），
-- 落法照复核完成留痕 reviewCompletion——一版至多一次由领域守，快照整份写回。

ALTER TABLE parcel_shipment.acceptance_processing_attempt
    DROP CONSTRAINT acceptance_processing_attempt_resume_path_closed,
    ADD CONSTRAINT acceptance_processing_attempt_resume_path_closed
        CHECK (resume_path IN (
            'CUSTOMER_SUPPLEMENT', 'INTERNAL_RETRY', 'MANUAL_REVIEW', 'OPERATOR_REGISTRATION', 'AUTHORIZED_DISPOSITION'));

ALTER TABLE parcel_shipment.shipment_request
    DROP CONSTRAINT shipment_request_task_waiting_on_known,
    ADD CONSTRAINT shipment_request_task_waiting_on_known
        CHECK (task_waiting_on BETWEEN 0 AND 5);

CREATE INDEX shipment_request_authorized_disposition_queue
    ON parcel_shipment.shipment_request (tenant_id, submitted_at)
    WHERE task_waiting_on = 5 AND state = 1;

ALTER TABLE parcel_shipment.acceptance_financial_control_item
    ADD COLUMN failure_disposition text,
    ADD COLUMN responsibility_ref text,
    ADD CONSTRAINT acceptance_financial_control_item_disposition_closed
        CHECK (failure_disposition IS NULL OR failure_disposition IN ('REJECT', 'AUTHORIZED_DISPOSITION')),
    ADD CONSTRAINT acceptance_financial_control_item_disposition_shape
        CHECK (
            (failure_disposition IS NULL AND responsibility_ref IS NULL)
            OR (item_conclusion = 'RESTRICTED'
                AND failure_disposition IS NOT NULL
                AND responsibility_ref IS NOT NULL AND btrim(responsibility_ref) <> '')
        );
