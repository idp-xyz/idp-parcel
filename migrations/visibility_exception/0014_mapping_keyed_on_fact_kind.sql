-- 里程碑映射按事实类型建键（MAP-KIND）。
--
-- 0009 条目表按 source_fact_ref 建键，那是实例引用，目录结构上填不满。本文件不改写
-- 0009：空库上重建条目主键，并给已接受事实补必填类型列。
--
-- ADD COLUMN NOT NULL 无默认（照 0003）。测试库与 CI 走到这里时表是空的；有行却填
-- 不出类型就让迁移失败，禁止填 UNTYPED。

ALTER TABLE visibility_exception.accepted_fact
    ADD COLUMN source_fact_kind text NOT NULL,
    ADD CONSTRAINT accepted_fact_kind_not_blank CHECK (btrim(source_fact_kind) <> '');

ALTER TABLE visibility_exception.milestone_mapping_entry
    DROP CONSTRAINT milestone_mapping_entry_pkey,
    DROP CONSTRAINT milestone_mapping_entry_not_blank;

ALTER TABLE visibility_exception.milestone_mapping_entry
    ADD COLUMN source_fact_kind text NOT NULL,
    DROP COLUMN source_fact_ref,
    ADD CONSTRAINT milestone_mapping_entry_pkey
        PRIMARY KEY (tenant_id, mapping_version, source_context, source_fact_kind),
    ADD CONSTRAINT milestone_mapping_entry_not_blank
        CHECK (btrim(source_fact_kind) <> '' AND btrim(milestone_ref) <> '');
