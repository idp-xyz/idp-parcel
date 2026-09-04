-- 「渠道择优决定」记录（票 label-channel-service-first-release/14 裁决，MCP-3 2026-09-04）。
--
-- 一次择优一条决定：对哪个租户、在哪个商业范围下按哪笔产品—渠道映射、以哪个时点装配的候选、
-- 按哪条规则、何时决定、结论是什么；逐候选各一行子表：候选引用、所用 BUY 评价引用（可缺）、
-- 四格之一、出局时的因由。**只引用不拷贝**：没有金额列、没有评价内容列——金额留在
-- parcel-pricing 的评价上，本表答「谁赢谁出局及为何」。
--
-- **只追加。** 没有乐观版本列、没有 state 列：记录落库后就是历史，重跑择优铸新记录，同一对象
-- 多条按 decided_at 构成择优历史。也没有保留期与清理：PAR-NET-16 的留痕要求待提供，属实例
-- 半边，机制不设默认值。
--
-- 头行与子行分表而不折进一份 jsonb：运营查阅面（另立票）要按结论筛「并列冲突待人工」、按出局
-- 因由分组，那些都是列面上的事；权威内容也就在列上——本记录没有需要整份重验的内部结构，与
-- label_transaction 那种「快照 + 列面」的纹样不同属。读回逐字段过领域构造函数再进重建门
-- （ADR-0028），跨字段命题（选中至多一个且与并列互斥、结论与结果互证）由门核，库内 CHECK
-- 只钉单行形状与封闭集。

CREATE TABLE parcel_shipment.channel_selection_decision (
    tenant_id       text        NOT NULL,
    decision_id     text        NOT NULL,

    scope_ref       text        NOT NULL,
    mapping_ref     text        NOT NULL,
    assembled_as_of timestamptz NOT NULL,
    rule            text        NOT NULL,
    decided_at      timestamptz NOT NULL,
    conclusion      text        NOT NULL,
    recorded_at     timestamptz NOT NULL DEFAULT now(),

    -- 主键即记录身份。租户维照 ADR-0003 的隔离边界，跨租户同号不是冲突。
    CONSTRAINT channel_selection_decision_pkey
        PRIMARY KEY (tenant_id, decision_id),

    CONSTRAINT channel_selection_decision_refs_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(decision_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(mapping_ref) <> ''
        ),
    -- 规则封闭集与领域 ChannelSelectionRule 是同一个集合的两份，同笔改。首发只有成本单维。
    CONSTRAINT channel_selection_decision_rule_closed
        CHECK (rule IN ('COST_ONLY')),
    -- 结论封闭集与领域 ChannelSelectionConclusion 同笔改。币种不齐不成记录，故没有那一格。
    CONSTRAINT channel_selection_decision_conclusion_closed
        CHECK (conclusion IN ('SELECTED', 'TIED', 'NONE_QUALIFIED'))
);

-- 按对象列历史：同一（租户 + 范围 + 映射）下按决定时刻取全部记录。
CREATE INDEX channel_selection_decision_by_subject
    ON parcel_shipment.channel_selection_decision (tenant_id, scope_ref, mapping_ref, decided_at, decision_id);

CREATE TABLE parcel_shipment.channel_selection_candidate (
    tenant_id      text    NOT NULL,
    decision_id    text    NOT NULL,
    -- 候选进入比较时的次序。读回按它排，逐候选结果的顺序与形成时一致。
    position       integer NOT NULL,

    candidate_ref  text    NOT NULL,
    evaluation_ref text,
    outcome        text    NOT NULL,
    exclusion      text,

    CONSTRAINT channel_selection_candidate_pkey
        PRIMARY KEY (tenant_id, decision_id, position),
    CONSTRAINT channel_selection_candidate_decision_fkey
        FOREIGN KEY (tenant_id, decision_id)
        REFERENCES parcel_shipment.channel_selection_decision (tenant_id, decision_id),
    -- 同一候选在一条决定里只出现一次（领域构造门同一条命题）。
    CONSTRAINT channel_selection_candidate_unique_per_decision
        UNIQUE (tenant_id, decision_id, candidate_ref),

    CONSTRAINT channel_selection_candidate_position_positive
        CHECK (position >= 1),
    CONSTRAINT channel_selection_candidate_refs_not_blank
        CHECK (
            btrim(candidate_ref) <> ''
            AND (evaluation_ref IS NULL OR btrim(evaluation_ref) <> '')
        ),
    -- 四格封闭集与领域 ChannelCandidateOutcome 同笔改。
    CONSTRAINT channel_selection_candidate_outcome_closed
        CHECK (outcome IN ('SELECTED', 'NOT_SELECTED', 'EXCLUDED', 'TIED')),
    -- 出局因由封闭集与领域 ChannelCostUnavailability 同笔改（四格一一对应 parcel-pricing 的四种
    -- 非完成结果，不合并——续办各不相同）。
    CONSTRAINT channel_selection_candidate_exclusion_closed
        CHECK (exclusion IS NULL OR exclusion IN ('PENDING_EVIDENCE', 'RATECARD_EXCLUSION', 'CONFLICT', 'NOT_FORMED')),
    -- 出局必带因由、非出局不得带因由：一行「出局却没说为何」答不出票 14 要答的那件事。
    CONSTRAINT channel_selection_candidate_exclusion_pairs_with_outcome
        CHECK ((outcome = 'EXCLUDED') = (exclusion IS NOT NULL))
);
