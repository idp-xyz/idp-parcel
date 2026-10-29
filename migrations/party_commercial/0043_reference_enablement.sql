-- 启用范围登记册（ADR-0345；CONTEXT Rules「启用范围登记册」）：某租户运营哪些国家 / 地区、哪些币种，由租户登记；
-- 没登记就是“未配置”，不套默认。登记的码只能取自产品内置的基础数据目录（ADR-0335，Go 里的只读数据，库里没有它的表）。
--
-- 键=租户+种类+码+修订，修订从 1 起连续递增；启用与停用都是新修订行，绝不 UPDATE、不 DELETE（ADR-0031 登记面纪律，
-- 同 0033 注册号类型目录）。当前是启用还是停用不存列，由“生效时点不晚于此刻的最新一行”导出
-- （domain.EnablementStandingAt）。本迁移只建结构、不种任何行：启用范围是租户取值（ADR-0146），产品不带任何默认。
--
-- 码是否在内置目录里是领域构造门的事（目录随发版变，SQL 里写不进一份会过期的码表），这里只锁形状：国家 / 地区两位、
-- 币种三位大写字母。已登记的码若日后被 ISO 废止，册上的行照旧在、照旧读得回。
--
-- registered_by 是登记责任方（认证信封里操作者的“发行方#sub”，ADR-0303 同写法），recorded_at 是登记时刻；两样都是审计
-- 事实，不进重放比对（同修订同内容的重放答“已登记”，在册的仍是先到的那一行）。source_ref 是登记者写下的来源单据，
-- 可以不写，不写时是空串。
--
-- 全部 CHECK 的列都 NOT NULL，没有 SQL 三值逻辑那一眼要看。

CREATE TABLE party_commercial.reference_enablement_registration (
    tenant_id      text        NOT NULL,
    kind           text        NOT NULL,
    code           text        NOT NULL,
    revision       integer     NOT NULL,

    status         text        NOT NULL,
    effective_from timestamptz NOT NULL,
    source_ref     text        NOT NULL,
    registered_by  text        NOT NULL,
    recorded_at    timestamptz NOT NULL,

    CONSTRAINT reference_enablement_registration_pkey
        PRIMARY KEY (tenant_id, kind, code, revision),

    CONSTRAINT reference_enablement_registration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(registered_by) <> ''
        ),

    CONSTRAINT reference_enablement_registration_revision_positive
        CHECK (revision >= 1),

    -- 镜像 domain.ReferenceKind 与 domain.ReferenceEnablementStatus 的封闭集。新增一词先改领域封闭集，再改这里。
    CONSTRAINT reference_enablement_registration_kind_closed
        CHECK (kind IN ('COUNTRY', 'CURRENCY')),
    CONSTRAINT reference_enablement_registration_status_closed
        CHECK (status IN ('ENABLED', 'DISABLED')),

    -- 形状镜像 ISO 3166-1 alpha-2 与 ISO 4217；成员资格不在此判。
    CONSTRAINT reference_enablement_registration_code_shape
        CHECK (
            (kind = 'COUNTRY' AND code ~ '^[A-Z]{2}$')
            OR (kind = 'CURRENCY' AND code ~ '^[A-Z]{3}$')
        ),

    -- 与 domain 的来源单据字数上限同一个数。
    CONSTRAINT reference_enablement_registration_source_bounded
        CHECK (char_length(source_ref) <= 200 AND source_ref = btrim(source_ref))
);

-- 读面按（租户，种类）一次取回全部码的全部修订；写面按主键前缀取某个码的最新修订。主键已覆盖两者，不另建索引。
