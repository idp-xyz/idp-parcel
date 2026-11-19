-- 测量与作废共用的来源身份登记。
--
-- 一个租户之内，一个来源身份不是测量就是作废，不能两者都是：变化事件的标识与下游续办的依据都由（租户，来源身份）定，
-- 两类带同一个来源身份，后到的那次变化会被当成已经办过。两张事实表各有各的主键，挡不住同一个来源身份既在这张又在那张；
-- 编排先取咨询锁再读另一张表，那只管得住走编排的写入。本迁移把这条规则落到表上：登记表的主键（租户，来源身份）保证一个
-- 来源身份只属于一类，两张事实表各以（租户，来源身份，种类）外键指向它，所以任何写入方都得先在登记表占住来源身份才写得进
-- 事实行。用声明式约束，不用触发器：种类列恒为自己那一种（检查约束钉死），外键再把它对到登记表上占的那一类。
--
-- 已有的行先回填进登记表、再加外键；回填只来自已有的事实行，不种任何租户的行。迁移之前这条规则只由编排守，若已有数据里同一
-- 个（租户，来源身份）既是测量又是作废，回填会撞登记表主键、整条迁移回滚，留给人处理，不替人挑哪一类。

CREATE TABLE node_operations.measurement_source_identity (
    tenant_id   text        NOT NULL,
    source_id   text        NOT NULL,
    kind        text        NOT NULL,
    inserted_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT measurement_source_identity_pkey
        PRIMARY KEY (tenant_id, source_id),

    -- 供两张事实表的外键引用：外键要指向一个唯一约束，主键只含（租户，来源身份）两列。
    CONSTRAINT measurement_source_identity_with_kind_unique
        UNIQUE (tenant_id, source_id, kind),

    CONSTRAINT measurement_source_identity_kind_closed
        CHECK (kind IN ('MEASUREMENT', 'INVALIDATION')),

    CONSTRAINT measurement_source_identity_text_present
        CHECK (btrim(tenant_id) <> '' AND btrim(source_id) <> '')
);

INSERT INTO node_operations.measurement_source_identity (tenant_id, source_id, kind)
SELECT tenant_id, source_id, 'MEASUREMENT'
  FROM node_operations.actual_measurement;

INSERT INTO node_operations.measurement_source_identity (tenant_id, source_id, kind)
SELECT tenant_id, source_id, 'INVALIDATION'
  FROM node_operations.measurement_invalidation;

ALTER TABLE node_operations.actual_measurement
    ADD COLUMN source_kind text NOT NULL DEFAULT 'MEASUREMENT',
    ADD CONSTRAINT actual_measurement_source_kind_is_measurement
        CHECK (source_kind = 'MEASUREMENT'),
    ADD CONSTRAINT actual_measurement_source_identity_fk
        FOREIGN KEY (tenant_id, source_id, source_kind)
        REFERENCES node_operations.measurement_source_identity (tenant_id, source_id, kind);

ALTER TABLE node_operations.measurement_invalidation
    ADD COLUMN source_kind text NOT NULL DEFAULT 'INVALIDATION',
    ADD CONSTRAINT measurement_invalidation_source_kind_is_invalidation
        CHECK (source_kind = 'INVALIDATION'),
    ADD CONSTRAINT measurement_invalidation_source_identity_fk
        FOREIGN KEY (tenant_id, source_id, source_kind)
        REFERENCES node_operations.measurement_source_identity (tenant_id, source_id, kind);
