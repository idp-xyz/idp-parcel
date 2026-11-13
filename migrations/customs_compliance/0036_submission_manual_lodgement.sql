-- 申报的线下递交留痕（dox/decisions.md「产品形态」）。
--
-- 没有对接报关行或单一窗口的租户，申报包由关务人员导出后在线下递交，回来记一笔。发送事实与系统发送的落在同一张表、
-- 守同一组门；这本册只记「这一份尝试是谁在线下递交的、凭的是哪份递交凭据」。系统发送的尝试没有这一条。一份尝试
-- 至多一条，与发送事实同一事务落；外键钉住它只能跟着一笔发送事实存在。

CREATE TABLE customs_compliance.submission_manual_lodgement (
    tenant_id    text        NOT NULL,
    version_id   text        NOT NULL,
    sequence     integer     NOT NULL,
    lodged_by    text        NOT NULL,
    original_ref text        NOT NULL,
    recorded_at  timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, version_id, sequence),
    CONSTRAINT submission_manual_lodgement_send_fact_exists
        FOREIGN KEY (tenant_id, version_id, sequence)
        REFERENCES customs_compliance.submission_send_fact (tenant_id, version_id, sequence),
    CONSTRAINT submission_manual_lodgement_filled
        CHECK (btrim(lodged_by) <> '' AND btrim(original_ref) <> '')
);
