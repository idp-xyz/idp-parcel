-- 人工录入的承运商轨迹留痕（dox/decisions.md「产品形态」）。
--
-- 没有对接承运商或轨迹聚合平台的租户，轨迹由操作员照承运商给的材料录入或批量导入。认领出的事实版本与轨迹源适配器
-- 认领的落在同一张表、走同一套规则；这本册只记「这一版是谁照着哪份原件录的」。事实表上没有这两格：源来的版本永远
-- 没有录入人，靠这本册里有没有它分开。一版至多一条，与事实版本同一事务落。

CREATE TABLE transport_fulfillment.external_tracking_manual_entry (
    tenant_id    text        NOT NULL,
    fact_id      text        NOT NULL,
    version_id   text        NOT NULL,
    entered_by   text        NOT NULL,
    original_ref text        NOT NULL,
    entered_at   timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, fact_id, version_id),
    CONSTRAINT external_tracking_manual_entry_filled
        CHECK (btrim(tenant_id) <> '' AND btrim(fact_id) <> '' AND btrim(version_id) <> ''
               AND btrim(entered_by) <> '' AND btrim(original_ref) <> '')
);
