-- 人工通知留痕：客户异常通知由操作员在线下（电话、邮件……）通知客户后，在产品里记一笔（dox/decisions.md「产品形态」）。
--
-- 没有对接客户系统的租户，通知靠人。通知本体、过程节点与系统推送的那一路是同一张表、同一套规则；这本册只记「这份
-- 通知是谁、用什么方式通知的、凭的是哪份原件」。系统推送的通知没有这一条。一份通知至多一条，与「已提交」那个过程
-- 节点同一事务落。

CREATE TABLE visibility_exception.customer_notification_manual_record (
    tenant_id       text        NOT NULL,
    notification_id text        NOT NULL,
    notified_by     text        NOT NULL,
    method          text        NOT NULL,
    original_ref    text        NOT NULL,
    recorded_at     timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, notification_id),
    CONSTRAINT customer_notification_manual_record_filled
        CHECK (btrim(tenant_id) <> '' AND btrim(notification_id) <> '' AND btrim(notified_by) <> ''
               AND btrim(method) <> '' AND btrim(original_ref) <> '')
);
