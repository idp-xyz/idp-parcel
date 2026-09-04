-- `等待受控补充`队列的部分索引（ADR-0106 Consequences，票 first-tenant-runway/09）：形照 0009 的
-- shipment_request_manual_review_queue 与 0013 的 shipment_request_operator_registration_queue。队列只认
-- 「已提交且等受控补充」（state=1 即 SUBMITTED，waiting=1 即 CUSTOMER_SUPPLEMENT），其余行不进索引；
-- 按提交时间序出队。
--
-- 这一格此前不需要索引，不是漏建：`ResumeByCustomerSupplement` 在消费门上整笔回滚，等待态随本轮蒸发，
-- 谓词在库里结构上恒空（票 09 第三问）。ADR-0106 Decision 二把它并回「先 Save 再交回」那一组之后，
-- 读口 ListWaitingOnCustomerSupplement 按这条谓词取行。取值仍镜像领域 ResumePath：1 = CUSTOMER_SUPPLEMENT；
-- 编号的唯一来源是领域包，SQL 只比较不拥有编号。0009 的 CHECK 区间已含 1，不必再放宽。

CREATE INDEX shipment_request_customer_supplement_queue
    ON parcel_shipment.shipment_request (tenant_id, submitted_at)
    WHERE task_waiting_on = 1 AND state = 1;
