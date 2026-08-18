-- 区间更正册（ADR-0038）：一行记一次对某已发布版本有效区间的更正。
--
-- 原区间留在 commercial_version 一字不动。本表只存更正区间、更正引用与更正时刻——
-- 这是 ADR-0038「有效性更正是登记册上的独立事实，不改写原 CommercialVersion 键下的
-- 正文、批准与原区间」的落库形态。
--
-- 行不自带版本快照，也不存 scope_ref：范围是版本壳上的事实，装载按版本行的 scope_ref
-- 过滤。存第二份会让「更正行说 A 范围、版本行说 B 范围」这种行存得下来。
--
-- D-5：一个版本可有多条更正，两侧同为只增；选用区间取登记顺序上的最后一条。四元组
-- 因此只是外键，不是主键。登记序轴是 registration_id（INSERT 时分配的 bigserial）：
-- 它不是 corrected_at（那是更正事实自己的时间，调用方可填得比前一条更早），也不是
-- 事务提交序。同一版本上并发写入两条不同更正时，「最后」以本列分配先后为准。更正是
-- 管理命令，不是高并发路径。
--
-- 同内容重放靠 UNIQUE NULLS NOT DISTINCT：开放结束的区间 ends_at 为 NULL，普通 UNIQUE
-- 会把两行 NULL 当成不同键，重放会再插一行。PostgreSQL 16 的 NULLS NOT DISTINCT 把
-- NULL 当相等，于是同内容第二次写入撞这个约束，适配器译`已登记`。
--
-- object_kind 不钉死某一类：九类都可能被更正。CHECK 镜像版本册放宽后的 1..9。
--
-- 行属实例半边；模式属机制半边。今天表空。族 A 不设「未配置」标记列：没更正就是
-- 查无此行，解析继续用版本原区间。

CREATE TABLE party_commercial.commercial_validity_correction (
    registration_id      bigserial   NOT NULL,
    tenant_id            text        NOT NULL,
    object_kind          smallint    NOT NULL,
    object_id            text        NOT NULL,
    version_label        text        NOT NULL,

    corrected_starts_at  timestamptz NOT NULL,
    corrected_ends_at    timestamptz,
    correction_ref       text        NOT NULL,
    corrected_at         timestamptz NOT NULL,
    registered_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commercial_validity_correction_pkey
        PRIMARY KEY (registration_id),

    CONSTRAINT commercial_validity_correction_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT commercial_validity_correction_same_content
        UNIQUE NULLS NOT DISTINCT (
            tenant_id, object_kind, object_id, version_label,
            correction_ref, corrected_at, corrected_starts_at, corrected_ends_at
        ),

    CONSTRAINT commercial_validity_correction_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(correction_ref) <> ''
        ),

    CONSTRAINT commercial_validity_correction_kind_known
        CHECK (object_kind BETWEEN 1 AND 9),

    CONSTRAINT commercial_validity_correction_interval_coherent
        CHECK (corrected_ends_at IS NULL OR corrected_ends_at > corrected_starts_at)
);

-- 装载按版本四元组取该版本的全部更正，再按 registration_id 保序。命中的正是这条索引。
CREATE INDEX commercial_validity_correction_by_version
    ON party_commercial.commercial_validity_correction
        (tenant_id, object_kind, object_id, version_label, registration_id);
