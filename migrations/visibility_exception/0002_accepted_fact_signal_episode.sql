-- 已接受源事实与信号发作期。
--
-- accepted_fact：投影派生的输入登记册。键=来源上下文+事实引用+来源版本（来源更正是
-- 新版本新键，事实只增不删）；业务发生、有效与接收三时间分存——合并成一个字段，迟到
-- 事实与更正就再也分不出「什么时候发生」与「什么时候才知道」（CONTEXT 硬句）。源上
-- 下文是封闭五值：来源消息、原始扫描或外部状态码未经业务所有者接受，在库里也没有入口。
--
-- signal_episode / triage_conclusion：发作期与它的分诊结论分表同库——SaveRaised 两表
-- 同一事务写入（只落发作期不落结论，重试会走进「已有活跃发作期」那一支去记命中，结论
-- 永远补不上）。结论表外键指回发作期：「结论不能没有发作期」结构性落库；反向的半截
-- （发作期没有结论）由同笔提交纪律承担，库无从判。

CREATE TABLE visibility_exception.accepted_fact (
    source_context text        NOT NULL,
    fact_ref       text        NOT NULL,
    fact_version   text        NOT NULL,

    parcel_ref     text        NOT NULL,
    content_digest text        NOT NULL,
    occurred_at    timestamptz NOT NULL,
    effective_at   timestamptz NOT NULL,
    received_at    timestamptz NOT NULL,
    recorded_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT accepted_fact_pkey PRIMARY KEY (source_context, fact_ref, fact_version),

    CONSTRAINT accepted_fact_source_closed
        CHECK (source_context IN (
            'PARCEL_SHIPMENT', 'NETWORK_ROUTING', 'NODE_OPERATIONS',
            'TRANSPORT_FULFILLMENT', 'CUSTOMS_COMPLIANCE'
        )),
    CONSTRAINT accepted_fact_not_blank
        CHECK (
            btrim(fact_ref) <> ''
            AND btrim(fact_version) <> ''
            AND btrim(parcel_ref) <> ''
            AND btrim(content_digest) <> ''
        )
);

-- FindByParcel 是投影派生的读面：按包裹取全部已接受事实。
CREATE INDEX accepted_fact_by_parcel
    ON visibility_exception.accepted_fact (parcel_ref, received_at);

CREATE TABLE visibility_exception.signal_episode (
    episode_id     text        NOT NULL,

    -- FindLatest 的并列裁决序。重开发作期的首命中允许与前期结束同刻，started_at
    -- 单列排不出先后；seq 只在这里当墙上时钟之外的到达序用，不是业务字段。
    seq            bigint      GENERATED ALWAYS AS IDENTITY,

    parcel_ref     text        NOT NULL,
    kind_ref       text        NOT NULL,
    rule_ref       text        NOT NULL,
    confidence_ref text        NOT NULL,
    hits           integer     NOT NULL,
    started_at     timestamptz NOT NULL,
    last_hit_at    timestamptz NOT NULL,
    release_basis  text,
    ended_at       timestamptz,
    prior_episode  text,

    CONSTRAINT signal_episode_pkey PRIMARY KEY (episode_id),

    CONSTRAINT signal_episode_not_blank
        CHECK (
            btrim(episode_id) <> ''
            AND btrim(parcel_ref) <> ''
            AND btrim(kind_ref) <> ''
            AND btrim(rule_ref) <> ''
            AND btrim(confidence_ref) <> ''
        ),
    CONSTRAINT signal_episode_hit_history
        CHECK (hits >= 1 AND last_hit_at >= started_at),
    -- 结束依据与结束时间同在场：只有一半的「已结束」是坏写入。
    CONSTRAINT signal_episode_release_shape
        CHECK (
            ((ended_at IS NULL) = (release_basis IS NULL))
            AND (release_basis IS NULL OR btrim(release_basis) <> '')
            AND (ended_at IS NULL OR ended_at >= last_hit_at)
        ),
    CONSTRAINT signal_episode_prior_not_self
        CHECK (prior_episode IS NULL OR (btrim(prior_episode) <> '' AND prior_episode <> episode_id))
);

CREATE INDEX signal_episode_latest
    ON visibility_exception.signal_episode (parcel_ref, kind_ref, started_at DESC, seq DESC);

CREATE TABLE visibility_exception.triage_conclusion (
    episode_id text        NOT NULL,

    outcome    text        NOT NULL,
    rule_ref   text        NOT NULL,
    triaged_at timestamptz NOT NULL,

    CONSTRAINT triage_conclusion_pkey PRIMARY KEY (episode_id),
    CONSTRAINT triage_conclusion_episode_exists
        FOREIGN KEY (episode_id) REFERENCES visibility_exception.signal_episode (episode_id),

    CONSTRAINT triage_conclusion_outcome_closed
        CHECK (outcome IN ('ATTACH_TO_EXISTING', 'AUTO_ESTABLISH', 'MANUAL_REVIEW', 'NO_CASE')),
    CONSTRAINT triage_conclusion_not_blank
        CHECK (btrim(rule_ref) <> '')
);
