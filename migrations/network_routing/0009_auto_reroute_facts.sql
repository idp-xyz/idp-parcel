-- 自动改路四条件事实目录（审计票 05，W10）。
--
-- ReassessRouteHandler 的 7B/7C 段按判断键取「政策允许自动、位于受控节点、仅未执行
-- 部分受影响」三个折算结论与「未解除硬限制、未处理既有责任」两份引用清单
-- （domain.AutoRerouteFacts 五件）。折算所依的改善阈值、改路条件与权限、冻结边界全属
-- PAR-NET-14 实例半边——本表因此只存**登记方折算完的事实陈述与其折算依据**，不存
-- 阈值本身，也不种任何默认行（AGENTS.md：机制现在就做，实例留空并拒绝默认值）。
--
-- 同一判断键的事实随包裹移动与限制解除而变化，属「当下陈述」不是「计划生效」——
-- 版本机制照 availability_adjustment 先例走**历史链**：同键版本行只增不改，当前陈述
-- 取最大版本；不用 [effective_from, effective_to) 区间制，那一套表达的是「自某明确
-- 时刻起参与新判断」，对一份现场事实陈述没有对应语义。
--
-- 「未配置」由零行表达：该键从未登记即目录未配置（端口第二格），装载口据此答
-- found=false，复核编排整段不做改路评估——与登记过之后的任何一版都分得开，不需要
-- 0008 那样的修订锚（历史链取最大版没有「登记过但此刻无适用版」的中间态）。
CREATE TABLE network_routing.auto_reroute_facts (
    -- 判断键六维（domain.InitialRouteJudgmentKey），逐包裹独立判断范围。
    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    shipment_request_id text        NOT NULL,
    acceptance_baseline text        NOT NULL,
    declared_parcel_id  text        NOT NULL,
    service_purpose     text        NOT NULL,
    version             integer     NOT NULL,

    -- 四条件的三个折算结论（第四件「不存在未解除硬限制/未处理责任」由两份清单表达）。
    policy_allows_automatic  boolean NOT NULL,
    at_controlled_node       boolean NOT NULL,
    only_unexecuted_affected boolean NOT NULL,

    -- 引用清单（text 数组的 jsonb 形态）。空数组是有效陈述——「无未解除限制」与
    -- 「未登记」是两种真话，前者在行内、后者是零行。
    unresolved_restrictions      jsonb NOT NULL,
    outstanding_responsibilities jsonb NOT NULL,

    -- 折算依据：这份陈述按哪个策略版本折出来（domain.AutoRerouteFacts 注释「由取数侧
    -- 按策略版本……折成布尔」）。没有出处的事实陈述审计答不出「按什么折的」。
    strategy_basis text        NOT NULL,

    registered_at  timestamptz NOT NULL,

    CONSTRAINT auto_reroute_facts_pkey
        PRIMARY KEY (
            tenant_id, customer_account_id, shipment_request_id,
            acceptance_baseline, declared_parcel_id, service_purpose, version
        ),

    CONSTRAINT auto_reroute_facts_version_positive
        CHECK (version >= 1),
    CONSTRAINT auto_reroute_facts_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(acceptance_baseline) <> ''
            AND btrim(declared_parcel_id) <> ''
            AND btrim(service_purpose) <> ''
            AND btrim(strategy_basis) <> ''
        ),
    CONSTRAINT auto_reroute_facts_lists_are_arrays
        CHECK (
            jsonb_typeof(unresolved_restrictions) = 'array'
            AND jsonb_typeof(outstanding_responsibilities) = 'array'
        )
);
