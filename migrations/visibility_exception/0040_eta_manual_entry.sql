-- 人工录入的 ETA 预测留痕（dox/decisions.md「产品形态」）。
--
-- ETA 的形成此前没有任何生产入口：没有预测来源对接。现在操作员可以把承运商告知的、或自己估的预计到达录进来。预测
-- 版本与预测来源送来的落在同一张表、走同一套规则（按包裹、里程碑、来源成线，输入版本变了就换版、历史不覆盖）；这本册
-- 只记「这一版是谁照着哪份原件录的」。一版至多一条，与预测版本同一事务落。

CREATE TABLE visibility_exception.eta_manual_entry (
    tenant_id    text        NOT NULL,
    version_id   text        NOT NULL,
    entered_by   text        NOT NULL,
    original_ref text        NOT NULL,
    entered_at   timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, version_id),
    CONSTRAINT eta_manual_entry_filled
        CHECK (btrim(tenant_id) <> '' AND btrim(version_id) <> '' AND btrim(entered_by) <> '' AND btrim(original_ref) <> '')
);
