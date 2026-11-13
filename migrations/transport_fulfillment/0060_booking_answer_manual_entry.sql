-- 人工录入的承运订舱答复留痕（dox/decisions.md「产品形态」）。
--
-- 没有对接承运商系统的租户，承运方用电话或邮件答复订舱，操作员照着录进来。答复本体与承运系统交来的落在同一张表、
-- 走同一套规则（一次订舱至多一份定局、候补之后可接定局）；这本册只记「这一份答复是谁照着哪份原件录的」。承运系统
-- 交来的答复没有这一条。一份答复至多一条，与答复同一事务落。

CREATE TABLE transport_fulfillment.booking_answer_manual_entry (
    tenant_id      text        NOT NULL,
    booking_id     text        NOT NULL,
    acceptance_ref text        NOT NULL,
    entered_by     text        NOT NULL,
    original_ref   text        NOT NULL,
    entered_at     timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, booking_id, acceptance_ref),
    CONSTRAINT booking_answer_manual_entry_filled
        CHECK (btrim(tenant_id) <> '' AND btrim(booking_id) <> '' AND btrim(acceptance_ref) <> ''
               AND btrim(entered_by) <> '' AND btrim(original_ref) <> '')
);
