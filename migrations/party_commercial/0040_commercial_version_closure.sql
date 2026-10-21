-- 按后继收尾旧版的两张册（ADR-0233，票 demo-chain-gaps/50）。
--
-- 一、区间更正册的对象类别约束对齐到版本册。0019 把 commercial_version 的上界放到 10、收进 CUSTOMER_SERVICE_RULE 时，
-- 0009 在本表上写下的同名约束停在 1..9：两张表镜像同一个领域封闭集，却一宽一窄——客户服务规则版本能发布、能在同一
-- 范围里与另一版重叠成`适用冲突`，却写不进一条截止。截止入口让这张表第一次有了生产写入方，这一格不再只是潜伏的陷阱。
-- 不改 0009：已落库的迁移正文按校验和守着（见 migrations.Asset），放宽只能是一份新的不可变迁移，形照 0004、0019。
--
-- 二、同起点退役册。一行记一次「这一版让位给同一对象的哪一版、凭哪份决定、何时起不再参与新的选择」。版本行的快照
-- 一字不动：只增仓储没有事后翻状态的口（发布用例「到界即取效」那段注释），退役另册保存、装载时施加到版本上，与区间
-- 更正同一种落法（ADR-0038）。一版只退一次，主键即版本四元组；同一个决定重放撞主键，适配器读回比对后答`已登记`。
-- 后继也以外键指向版本册，且与旧版同租户、同类、同对象：跨对象的「后继」在这张表里存不下。

ALTER TABLE party_commercial.commercial_validity_correction
    DROP CONSTRAINT commercial_validity_correction_kind_known;

ALTER TABLE party_commercial.commercial_validity_correction
    ADD CONSTRAINT commercial_validity_correction_kind_known
        CHECK (object_kind BETWEEN 1 AND 10);

CREATE TABLE party_commercial.commercial_version_retirement (
    tenant_id       text        NOT NULL,
    object_kind     smallint    NOT NULL,
    object_id       text        NOT NULL,
    version_label   text        NOT NULL,

    successor_label text        NOT NULL,
    retirement_ref  text        NOT NULL,
    retired_at      timestamptz NOT NULL,
    registered_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commercial_version_retirement_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT commercial_version_retirement_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT commercial_version_retirement_successor_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, successor_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT commercial_version_retirement_successor_is_another_version
        CHECK (successor_label <> version_label),

    CONSTRAINT commercial_version_retirement_not_blank
        CHECK (btrim(retirement_ref) <> '' AND btrim(successor_label) <> '')
);
