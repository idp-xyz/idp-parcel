-- 解释规则登记册补版本维（ADR-0070 问一甲）：主键从（租户，结果层）扩为（租户，结果层，
-- 适用辖区，法定生效区间起），读口按半开区间解析。CONTEXT「监管规则版本与适用」要求关务
-- 规则版本记录适用辖区与法定生效区间（硬句 191）；一层一行的当前指针册子对迟到的外部结果
-- 只能拿消息到达时点当法定适用时点，正是该句点名禁止的替代。
--
-- 既有行拒绝迁移而不是代填：单版行上辖区与生效区间两维根本不存在，任何回填值都是本迁移
-- 替实例半边编造的默认（AGENTS.md 红线）。册子今天只可能装着脱敏合成值（尚无租户），
-- 重登比造假便宜。

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM customs_compliance.interpretation_rule) THEN
        RAISE EXCEPTION USING MESSAGE =
            'interpretation_rule 存在单版旧行：辖区与法定生效区间无从回填，不代填默认。'
            ' 请先导出旧行备查并清空该表，迁移后经 parcel-customs-register 的'
            ' interpretation-rule 命令带 jurisdictionRef 与 appliesFrom 重新登记。';
    END IF;
END
$$;

-- 区间不重叠靠排他约束交付（ADR-0070 问一甲给出的两条路之一；另一条是 ADR-0056 形状的
-- 写口串行化，但本册的首条登记没有可锁的既有行，约束是唯一不漏首插并发的那条路）。
-- gist 上的文本等值列需要 btree_gist；PostgreSQL 13 起它是受信扩展，库属主即可安装。
CREATE EXTENSION IF NOT EXISTS btree_gist;

DROP TABLE customs_compliance.interpretation_rule;

CREATE TABLE customs_compliance.interpretation_rule (
    tenant_id        text        NOT NULL,
    result_layer     text        NOT NULL,
    jurisdiction_ref text        NOT NULL,
    -- 法定生效区间：起点随登记给出并入键；终点不是登记输入——它在后继版本登记时落定
    -- （换版），NULL 即「尚无终点」。写口没有任何改写 rule_ref 或 applies_from 的路径，
    -- 终点从 NULL 到一个时刻是与就绪/授权撤销同款的状态推进，不是覆盖。
    applies_from     timestamptz NOT NULL,
    applies_until    timestamptz,

    rule_ref         text        NOT NULL,

    CONSTRAINT interpretation_rule_pkey
        PRIMARY KEY (tenant_id, result_layer, jurisdiction_ref, applies_from),

    CONSTRAINT interpretation_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(jurisdiction_ref) <> ''
            AND btrim(rule_ref) <> ''
        ),
    -- 外部监管结果的封闭六层（CONTEXT 硬句 185 分层保存），与 0007 原表同句。
    CONSTRAINT interpretation_rule_layer_closed
        CHECK (result_layer IN (
            'REGULATORY_RECEIPT',
            'BUSINESS_ACCEPTANCE',
            'PROCESS_DECISION',
            'ASSESSED_DUTY',
            'RELEASE_RESULT',
            'DISPOSITION_DECISION'
        )),
    CONSTRAINT interpretation_rule_interval_ordered
        CHECK (applies_until IS NULL OR applies_until > applies_from),
    -- 同（租户，层，辖区）下法定生效区间不重叠：一个评估时点至多解析出一版，选择侧
    -- 因此没有「到达顺序决定用哪版」的缝。tstzrange 默认半开 [)，与读口一致；上界
    -- NULL 即无界，开放版与任何更晚起点的登记天然相斥——那正是换版必须先让写口给
    -- 前版落终点的结构性理由。
    CONSTRAINT interpretation_rule_no_overlap
        EXCLUDE USING gist (
            tenant_id WITH =,
            result_layer WITH =,
            jurisdiction_ref WITH =,
            tstzrange(applies_from, applies_until) WITH &&
        )
);
