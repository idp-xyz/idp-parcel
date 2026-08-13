-- 处置请求：异常案件向源业务上下文提出的结构化业务请求。请求已发送、源上下文已
-- 接受、取消/替代意图与目标方取消结果是不同结果（CONTEXT 语言）——判断与答复各占
-- 列，实际执行结果不在本表（那是目标上下文按事实返回的东西，行内没有它的字段）。
--
-- 替代关系走 superseded_by 指回列：原请求与其已有判断原样保留（硬句 148 替代不是
-- 删除）。「（案件+动作+范围）当前至多一份」由部分唯一索引结构性承担——替代时先更
-- 旧行再插新行（适配器纪律），同一事务内越过提交边界。
--
-- 带 NULL 列的 CHECK 一律走 IS NULL 显式分支（会话纪律：普通比较在 NULL 上会让
-- 整条约束按 NULL 放行）。

CREATE TABLE visibility_exception.disposition_request (
    tenant_id         text        NOT NULL,
    request_id        text        NOT NULL,

    case_id           text        NOT NULL,
    target_context    text        NOT NULL,
    action_ref        text        NOT NULL,
    scope_ref         text        NOT NULL,
    reason            text        NOT NULL,
    evidence_ref      text        NOT NULL,
    intent_version    integer     NOT NULL,
    sent_at           timestamptz NOT NULL,
    acceptance_window timestamptz,

    judgment          text,
    judged_at         timestamptz,
    cancellation      text,
    superseded_by     text,

    CONSTRAINT disposition_request_pkey PRIMARY KEY (tenant_id, request_id),

    CONSTRAINT disposition_request_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(request_id) <> ''
            AND btrim(case_id) <> ''
            AND btrim(action_ref) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(reason) <> ''
            AND btrim(evidence_ref) <> ''
        ),
    -- 接收上下文是源上下文封闭五值（与事实登记册同一集合）。
    CONSTRAINT disposition_request_target_closed
        CHECK (target_context IN (
            'PARCEL_SHIPMENT', 'NETWORK_ROUTING', 'NODE_OPERATIONS',
            'TRANSPORT_FULFILLMENT', 'CUSTOMS_COMPLIANCE'
        )),
    CONSTRAINT disposition_request_intent_positive
        CHECK (intent_version >= 1),
    CONSTRAINT disposition_request_window_after_sent
        CHECK (acceptance_window IS NULL OR acceptance_window > sent_at),
    -- 判断与判断时间同在场，判断是封闭四走向，时间不早于发送。
    CONSTRAINT disposition_request_judgment_shape
        CHECK (
            ((judgment IS NULL) = (judged_at IS NULL))
            AND (judgment IS NULL OR judgment IN
                ('ACCEPTED', 'PARTIALLY_ACCEPTED', 'REFUSED', 'SUPPLEMENT_REQUIRED'))
            AND (judged_at IS NULL OR judged_at >= sent_at)
        ),
    -- 未判断的请求谈不上取消（领域「未判断即无外部意图」的库面）；答复封闭四值。
    CONSTRAINT disposition_request_cancellation_shape
        CHECK (
            (cancellation IS NULL OR judgment IS NOT NULL)
            AND (cancellation IS NULL OR cancellation IN
                ('CANCELLATION_ACCEPTED', 'PARTIALLY_CANCELLED', 'NO_LONGER_CANCELLABLE', 'CANCELLATION_REFUSED'))
        ),
    CONSTRAINT disposition_request_supersession_not_self
        CHECK (superseded_by IS NULL OR (btrim(superseded_by) <> '' AND superseded_by <> request_id))
);

-- （案件+动作+范围）当前至多一份：未被替代的行进部分唯一索引，也是 FindCurrent 的读面。
CREATE UNIQUE INDEX disposition_request_current
    ON visibility_exception.disposition_request (tenant_id, case_id, action_ref, scope_ref)
    WHERE superseded_by IS NULL;
