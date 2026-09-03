-- 信用政策册（票 party-commercial-context-gaps/03）：一行记一个信用政策版本的正文。
--
-- 与版本册分表，判据同商业价格政策册（0010）：版本回答「有没有这份信用政策对象」，正文
-- 回答「为哪个责任法人、权限等级与费用类型授权多少额度」。CONTEXT：「信用政策……按责任
-- 法人、业务角色、费用类型、金额或比例形成版本。政策只提供业务判断依据，不直接修改结算
-- 余额或形成调整金额」——所以表上没有任何余额、已占用或调整列，那些归 settlement-accounting。
--
-- 额度是**并存两列 + 恰一非空**，不是「一个数加一列标记」。额度 100 作金额与作比例在同一列
-- 里长得一模一样，标记设错时没有任何东西能分辨，两种读法都产出一个合法的额度、只是差几个
-- 数量级；两列并存时值落在哪一列本身就是判别式，它不可能与自己不一致。CHECK 再把「都空」与
-- 「都有」挡在外面——库上不留任何一行两种读法都合法的记录。比例取万分比整数，与领域
-- domain.CreditLimit 同刻度。
--
-- object_kind CHECK = 8（CreditPolicyObject）。形态只归信用政策。
--
-- 本表不进整册装载（LoadForScope）：今天没有任何解析在信用政策之间选，消费方按已选中的
-- 版本点读正文（ports.CreditPolicyContentView）。缺行由查无此行表达——缺政策既不是无限
-- 信用也不是零额度，该是哪一种只有拥有商业依据的一方能说，本表不预设。
--
-- 行属实例半边：真实额度由租户登记，今天没有租户因而本表为空。

CREATE TABLE party_commercial.credit_policy (
    tenant_id             text        NOT NULL,
    object_kind           smallint    NOT NULL,
    object_id             text        NOT NULL,
    version_label         text        NOT NULL,

    legal_entity_ref      text        NOT NULL,
    authority_level_ref   text        NOT NULL,
    charge_type_ref       text        NOT NULL,
    limit_minor           bigint,
    limit_ratio_bps       bigint,
    effective_starts_at   timestamptz NOT NULL,
    effective_ends_at     timestamptz,
    registered_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT credit_policy_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT credit_policy_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT credit_policy_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(legal_entity_ref) <> ''
            AND btrim(authority_level_ref) <> ''
            AND btrim(charge_type_ref) <> ''
        ),

    CONSTRAINT credit_policy_credit_only
        CHECK (object_kind = 8),

    -- 镜像 domain.CreditLimit 的两格封闭：恰一列在场。
    CONSTRAINT credit_policy_limit_exactly_one
        CHECK ((limit_minor IS NULL) <> (limit_ratio_bps IS NULL)),

    -- 零是「授予零额度」这句合法的商业声明，负值在哪一格都无意义（domain.NewCredit*Limit 同判据）。
    CONSTRAINT credit_policy_limit_non_negative
        CHECK (
            (limit_minor IS NULL OR limit_minor >= 0)
            AND (limit_ratio_bps IS NULL OR limit_ratio_bps >= 0)
        ),

    CONSTRAINT credit_policy_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);
