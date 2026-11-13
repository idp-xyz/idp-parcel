-- 人工录入的外部监管舱单引用留痕（dox/decisions.md「产品形态」）。
--
-- 出口与进口监管舱单由责任承运商在外部形成并提交，本产品只保存引用。没有对接承运商系统的租户，引用由操作员照承运商
-- 给的材料录入。引用本体与承运商系统送来的落在同一张表、走同一套规则（同版本重放、异版本走版本推进、同版本异内容
-- 冲突）；这本册只记「这一次录入是谁照着哪份原件录的」。承运商系统送来的版本没有这一条。与引用同一事务落。

CREATE TABLE customs_compliance.external_manifest_manual_entry (
    tenant_id    text        NOT NULL,
    carrier_ref  text        NOT NULL,
    manifest_id  text        NOT NULL,
    version_id   text        NOT NULL,
    entered_by   text        NOT NULL,
    original_ref text        NOT NULL,
    entered_at   timestamptz NOT NULL,
    -- 主键带录入时刻：留痕是一次录入一条，不是一个版本号一条。同一个版本号可以先后被录两次（推进到别的版本又推回来），
    -- 按版本号成行的话第二次会撞键。读的人按版本号加时刻对上引用上的接受时刻，才知道在册的那一版是不是人录的。
    PRIMARY KEY (tenant_id, carrier_ref, manifest_id, version_id, entered_at),
    CONSTRAINT external_manifest_manual_entry_filled
        CHECK (btrim(entered_by) <> '' AND btrim(original_ref) <> '')
);
