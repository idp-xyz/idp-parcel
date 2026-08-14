-- 通知策略目录（`PAR-VIS-07`）：一份披露决定该走什么渠道、限时多少、按哪条判据
-- 算满足通知义务。「客户异常通知必须保存通知对象、内容快照、披露依据、目标客户、
-- 要求时限和适用渠道」（CONTEXT 硬句）——后三样都来自这里，编排不补默认值。
--
-- 内容属实例半边（真实渠道、时限与合同义务待登记），表在首发是空的。空表时视图
-- 交回「未配置」，编排停在未决等租户登记——没有渠道的通知不存在一个如实的空白格，
-- 造一个占位渠道是虚构。
--
-- 键取（租户 + 披露策略引用），且**不另立版本表**——与同目录下里程碑映射、分诊
-- 规则那两份目录的两级形状不同，理由是它们要分的那两件事在这里不存在：
--   * 那两份要分开「目录整个没配」与「目录配了但这条没命中」，因为后者是一次已经
--     作出的判断（未归类／人工复核），必须带着版本号进结论；
--   * 这一份没有第二格。查不到行就是没有渠道，而没有渠道的通知作不出来，只有一条
--     续办路：等登记。因此 found=false 一格足够，多立一张版本表只会造出一个永远
--     走不到的分支。
-- 版本化本身没有丢：键里的披露策略引用指名的就是版本化信息披露规则，同一策略换版
-- 换的是引用值，新旧两行并存、各自被自己那批披露决定命中，天然不覆盖。

CREATE TABLE visibility_exception.notification_policy (
    tenant_id             text     NOT NULL,
    disclosure_policy_ref text     NOT NULL,

    channel_ref           text     NOT NULL,

    -- 时限存**相对量**而不是绝对时间：合同写的是「披露后 N 小时内」，而披露决定
    -- 时间逐份不同。存绝对时间等于给整个目录钉死一个截止点。绝对截止时间由查询
    -- 期用披露决定时间加出来，interval 的加法归 Postgres——月与日的进位语义在它
    -- 那里，Go 侧自己折算会在跨月与夏令时上和库不一致。
    deadline_after        interval NOT NULL,

    -- 义务判据：哪个过程节点算满足义务（送达？客户确认？）由合同说了算，本上下文
    -- 只带引用。CONTEXT：「合同要求送达或确认时，只有相应结果成立才满足通知义务」。
    obligation_ref        text     NOT NULL,

    CONSTRAINT notification_policy_pkey PRIMARY KEY (tenant_id, disclosure_policy_ref),

    CONSTRAINT notification_policy_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(disclosure_policy_ref) <> ''
            AND btrim(channel_ref) <> ''
            AND btrim(obligation_ref) <> ''
        ),

    -- 非正时限会把截止时间算到披露决定之前或与之相等，那样的通知一生成就已逾期。
    CONSTRAINT notification_policy_deadline_after_positive
        CHECK (deadline_after > interval '0')
);
