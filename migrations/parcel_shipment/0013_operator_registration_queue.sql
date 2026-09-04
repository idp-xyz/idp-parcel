-- `等待运营登记`队列的部分索引（ADR-0094 Decision 五，票 first-tenant-runway/07）：形照 0009 的
-- shipment_request_manual_review_queue。队列只认「已提交且等运营登记」（state=1 即 SUBMITTED，
-- waiting=4 即 OPERATOR_REGISTRATION），其余行不进索引；按提交时间序出队。
--
-- 0011 只对齐了两条 CHECK 没建索引，理由写在那里：队列谓词属 Decision 五那一片。现在那一片到了——
-- 领域有 AwaitOperatorRegistration 在决定之前写下第四格，两条 as-of 编排在`判断时点未配置`时经
-- Save 落库，读口 ListWaitingOnOperatorRegistration 按这条谓词取行。取值仍镜像领域 ResumePath：
-- 4 = OPERATOR_REGISTRATION；编号的唯一来源是领域包，SQL 只比较不拥有编号。

CREATE INDEX shipment_request_operator_registration_queue
    ON parcel_shipment.shipment_request (tenant_id, submitted_at)
    WHERE task_waiting_on = 4 AND state = 1;
