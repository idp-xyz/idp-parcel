-- 集运作业的来源事实层：单元与封装快照带上来源与执行方，另立按来源身份幂等的作业
-- 事实登记（票 no-consolidation-fact-provenance/01）。
--
-- 集运六口此前直接改派生状态，没有 ADR-0005 那一层来源事实：封装与开封的时刻取的是
-- 处理时的系统时钟，而 ADR-0023 把作业事实的身份与时间判给设备——服务端代铸的时间
-- 正是它禁止的那种。UC-NO-003 结果契约「作业事实已形成」要求保存`执行方、来源和证据`，
-- CONTEXT「封签记录」要求保存`施封依据和观察历史`，受控开封一条要求保存`授权来源、
-- 执行人、时间`；下面两处分别接住它们。
--
-- **opened_source 有意不带 DEFAULT，也不回填。** 既有集运单元行是在没有来源那一层的
-- 口径下写下的，它们表达不出谁开的、依据哪条来源开的——与 0013 在客户费用上那次不同，
-- 那里「原币即结算币」是唯一合法解读，这里没有任何解读能从行里读出来。因此本迁移在
-- 表非空时会直接失败并点名该列，而不是编一个默认值让读的人以为那是记下来的事实。
-- 产品尚无租户，生产上不可能存在这样的行；开发库若被拦下，正确动作是清掉那些无来源
-- 的试验行，不是给这一列补默认。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：jsonb 取键可能给 NULL，
-- 比较前一律先 COALESCE 或 IS NOT NULL，否则整条按 NULL 放行。

ALTER TABLE node_operations.consolidation_unit
    ADD COLUMN opened_source jsonb NOT NULL;

ALTER TABLE node_operations.consolidation_unit
    ADD CONSTRAINT consolidation_unit_opened_source_shaped
        CHECK (
            jsonb_typeof(opened_source) = 'object'
            AND btrim(COALESCE(opened_source ->> 'sourceId', '')) <> ''
            AND btrim(COALESCE(opened_source ->> 'performedBy', '')) <> ''
            AND btrim(COALESCE(opened_source ->> 'evidence', '')) <> ''
            AND btrim(COALESCE(opened_source ->> 'occurredAt', '')) <> ''
        );

-- 封装快照的来源同样必备。CHECK 里用不了子查询，因此反向问一次 jsonpath：
-- 「存不存在某一份缺了四格之一的快照」，存在即拒。空数组恒不匹配，尚未封装过的
-- 开放实例照常放行。
--
-- 这一条只判在场，判不了`"  "`那种空白值——jsonpath 没有 btrim。空白由领域构造器
-- 与重建门守（两处都要求非空白），库面这一条守的是写入方整个漏掉 source 的那一格。
ALTER TABLE node_operations.consolidation_unit
    ADD CONSTRAINT consolidation_unit_snapshot_sources_present
        CHECK (
            NOT jsonb_path_exists(
                snapshots,
                '$[*] ? (!exists(@.source.sourceId) || !exists(@.source.performedBy) || !exists(@.source.evidence) || !exists(@.source.occurredAt))'
            )
        );

-- 集运作业事实登记：同一来源身份只有一份作业结果越过提交边界（幂等键=租户+来源标识，
-- 形状同 0001 的收寄登记）。同键第二份由主键拦住，适配器以 ON CONFLICT DO NOTHING
-- 译成`已有记录`；同键异内容的冲突分界靠内容指纹列。两者都不覆盖先到者（AT-NO-043）。
--
-- 它与单元行分开：单元行是派生状态，同一个单元被许多次作业推进，把来源挤进单元行只
-- 留得下最后一次。occurred_at 是现场自带的业务时间，recorded_at 是服务端的记录时刻，
-- 两列分立正是 ADR-0023 要的那条分界。
CREATE TABLE node_operations.consolidation_fact (
    tenant_id      text        NOT NULL,
    source_id      text        NOT NULL,

    content_digest text        NOT NULL,
    action         text        NOT NULL,
    unit_id        text        NOT NULL,
    member_id      text,
    seal_ref       text,
    performed_by   text        NOT NULL,
    evidence       text        NOT NULL,
    occurred_at    timestamptz NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT consolidation_fact_pkey
        PRIMARY KEY (tenant_id, source_id),

    CONSTRAINT consolidation_fact_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_id) <> ''
            AND btrim(content_digest) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(performed_by) <> ''
            AND btrim(evidence) <> ''
        ),

    CONSTRAINT consolidation_fact_action_closed
        CHECK (action IN ('OPEN_UNIT', 'ADD_MEMBER', 'REMOVE_MEMBER', 'SEAL_UNIT', 'UNSEAL_UNIT', 'CLOSE_UNIT')),

    -- 成员只在移入/移出两格在场，封签只在封装格在场；其余格上缺席是真话不是漏填。
    CONSTRAINT consolidation_fact_member_presence
        CHECK ((action IN ('ADD_MEMBER', 'REMOVE_MEMBER')) = (member_id IS NOT NULL)),

    CONSTRAINT consolidation_fact_seal_presence
        CHECK ((action = 'SEAL_UNIT') = (seal_ref IS NOT NULL)),

    -- 在场时不得空白。等号碰上 NULL 给 NULL，必须先 IS NULL 分掉缺席那一支。
    CONSTRAINT consolidation_fact_present_columns_not_blank
        CHECK (
            (member_id IS NULL OR btrim(member_id) <> '')
            AND (seal_ref IS NULL OR btrim(seal_ref) <> '')
        ),

    CONSTRAINT consolidation_fact_unit_fk
        FOREIGN KEY (tenant_id, unit_id)
        REFERENCES node_operations.consolidation_unit (tenant_id, unit_id)
);

CREATE INDEX consolidation_fact_by_unit
    ON node_operations.consolidation_fact (tenant_id, unit_id, occurred_at);
