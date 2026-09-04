-- 轨迹源有效时间规则目录（label-channel/19，落文 ADR-0102 决定三）。
--
-- CONTEXT「轨迹源」：有效时间规则属该源的实例参数，本上下文只引用已登记的轨迹源；规则节：有效时间
-- 「可以依据该轨迹源已登记并带版本的规则形成，不得默默等于发生时间」。本表是登记那份参数的机制；
-- **表里不存任何一家真实轨迹源的规则取值**（`PAR-INT-02`），留空。
--
-- **一源一链，主键带版本，写入只插不改。** 规则按（租户，轨迹源）登记，没有独立的规则名——事实上记的
-- effective_rule 就是 source_ref。换版是新版本回指前版，原版本一字不动；「当前版」按回指派生（没有任何
-- 行回指它），表上没有 current 列。
--
-- **正文三列**各自对应 ADR-0102 Consequences 点名的那道判断（该源报来的时间字段是事件发生时间还是源方
-- 处理时间）、有效时间相对哪一个时间、偏移多少。偏移允许为零或为负。三列里没有状态词——规则不解释
-- 状态词（ADR-0102 Consequences 第四条，那归里程碑映射登记册）。
CREATE TABLE transport_fulfillment.effective_time_rule (
    tenant_id           text        NOT NULL,
    source_ref          text        NOT NULL,
    version             text        NOT NULL,

    source_time_meaning text        NOT NULL,
    anchor              text        NOT NULL,
    offset_seconds      bigint      NOT NULL,

    supersedes_version  text,

    recorded_at         timestamptz NOT NULL,

    CONSTRAINT effective_time_rule_pkey
        PRIMARY KEY (tenant_id, source_ref, version),

    CONSTRAINT effective_time_rule_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(version) <> ''
        ),

    CONSTRAINT effective_time_rule_source_time_meaning_closed
        CHECK (source_time_meaning IN ('EVENT_OCCURRENCE', 'SOURCE_PROCESSING')),

    CONSTRAINT effective_time_rule_anchor_closed
        CHECK (anchor IN ('OCCURRED_AT', 'RECEIVED_AT')),

    CONSTRAINT effective_time_rule_supersedes_not_blank
        CHECK (supersedes_version IS NULL OR btrim(supersedes_version) <> ''),

    -- 前版引用指向自己就是一条读不动的链。
    CONSTRAINT effective_time_rule_supersedes_not_self
        CHECK (supersedes_version IS NULL OR supersedes_version <> version)
);

-- 版本链的形状由库面守，不靠编排里那次「先读当前版再写」：同一源只能有一个首版（不回指任何前版的
-- 行），同一版本只能有一个后继。编排读当前版与写新版之间另一方落进来时，只有这两道拦得住——撞上的
-- 一方按主键撞键同一格答`已登记`，读回不到自己那一版即`未决`，重放会得到正当答案。
CREATE UNIQUE INDEX effective_time_rule_single_first_version
    ON transport_fulfillment.effective_time_rule (tenant_id, source_ref)
    WHERE supersedes_version IS NULL;

CREATE UNIQUE INDEX effective_time_rule_single_successor
    ON transport_fulfillment.effective_time_rule (tenant_id, source_ref, supersedes_version)
    WHERE supersedes_version IS NOT NULL;
