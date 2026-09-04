-- 放行结果（票 mechanism-executor-triage/07 CC-b）。所有权取 CONTEXT「放行结果」语言：监管
-- 机构对明确范围作出的全部/部分/附条件放行事实；放行不能由技术成功、业务受理、税费支付
-- 或内部合规解除推导。此前放行层的外部响应只以 external_result 的分层事实落库，领域工厂
-- ReceiveReleaseOutcome 只被测试调过——本迁移补的是放行事实自己的落点。
--
-- 一行即同一次接收在放行层拆出的放行事实，与 external_result 同键（租户，来源标识）、同事务
-- 落地：放行结果不是从分层事实推导出来的第二份东西，是同一次接收按来源权威语义拆出的
-- 另一面；分表只因为它的形状（种类/机构/条件）不是每一层都有。外键钉住「没有分层事实就
-- 没有放行事实」——放行事实不能凭空出现（UC-CC-006：缺失层不补造，这里反向也成立）。
--
-- 条件列与种类的耦合在库内再守一遍：附条件放行必带条件说明，全部放行不带（与领域构造
-- 同判据）；部分放行的条件可有可无——它的「范围说明」就是 scope_ref。真实放行种类的
-- 代码映射属实例半边（PAR-CUS-01/02），本迁移不含任何实例默认值。
CREATE TABLE customs_compliance.release_outcome (
    tenant_id     text        NOT NULL,
    source_id     text        NOT NULL,

    kind          text        NOT NULL,
    authority_ref text        NOT NULL,
    scope_ref     text        NOT NULL,
    -- 空串即「无条件」：与领域一致，条件只在附条件放行上有意义。
    condition     text        NOT NULL DEFAULT '',
    received_at   timestamptz NOT NULL,

    CONSTRAINT release_outcome_pkey
        PRIMARY KEY (tenant_id, source_id),

    CONSTRAINT release_outcome_belongs_to_a_layer_fact
        FOREIGN KEY (tenant_id, source_id)
        REFERENCES customs_compliance.external_result (tenant_id, source_id),

    CONSTRAINT release_outcome_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_id) <> ''
            AND btrim(authority_ref) <> ''
            AND btrim(scope_ref) <> ''
        ),

    -- 放行种类封闭三值，与 domain.ReleaseKind 的 String 同词。
    CONSTRAINT release_outcome_kind_closed
        CHECK (kind IN ('FULL', 'PARTIAL', 'CONDITIONAL')),

    CONSTRAINT release_outcome_condition_matches_kind
        CHECK (
            (kind = 'CONDITIONAL' AND btrim(condition) <> '')
            OR (kind = 'FULL' AND condition = '')
            OR kind = 'PARTIAL'
        )
);
