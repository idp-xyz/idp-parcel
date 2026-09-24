-- 注册号类型目录（ADR-0145 决定一；CONTEXT Rules「注册号的类型与格式按注册国家 / 地区取自本上下文
-- 拥有的注册号类型目录」）：按租户登记每个注册国家 / 地区下的注册号类型——类型代码、名称、格式
-- 校验与所属层（身份层的终身注册号 / 资料层的税务登记号）。
--
-- 键=租户+注册国家 / 地区+类型代码+修订，修订从 1 起连续递增；内容更正与停用都是新修订行，绝不
-- UPDATE（ADR-0031 登记面纪律，同 0015 身份册）。本迁移只建结构、不种任何行：目录内容是实施时
-- 登记的配置，产品不带任何国家 / 地区的生产默认条目（ADR-0145 决定一、七）。
--
-- 不存状态列：已登记 / 已生效 / 已停用由 effective_from 与 deactivated_at 对时点导出
-- （domain.RegistrationNumberTypeLifecycle.StatusAt），判据同 0015 身份三表。
--
-- format_pattern 存登记原文。按整串匹配、先单独编译等语义由领域构造门把守，SQL 只锁非空。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（同 0015）：可空列使用前先 IS NULL / IS NOT NULL。

CREATE TABLE party_commercial.registration_number_type_registration (
    tenant_id          text        NOT NULL,
    country_code       text        NOT NULL,
    type_code          text        NOT NULL,
    revision           integer     NOT NULL,

    type_name          text        NOT NULL,
    layer              text        NOT NULL,
    format_pattern     text        NOT NULL,
    basis_ref          text        NOT NULL,
    effective_from     timestamptz NOT NULL,
    deactivated_at     timestamptz,
    deactivation_basis text,
    content_digest     text        NOT NULL,
    snapshot           jsonb       NOT NULL,
    recorded_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT registration_number_type_registration_pkey
        PRIMARY KEY (tenant_id, country_code, type_code, revision),

    CONSTRAINT registration_number_type_registration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(type_code) <> ''
            AND btrim(type_name) <> ''
            AND btrim(format_pattern) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    -- 镜像 domain.RegistrationCountryCode 的形状门：一个国家 / 地区在目录里只有一个键。
    CONSTRAINT registration_number_type_registration_country_shape
        CHECK (country_code ~ '^[A-Z]{2}$'),

    CONSTRAINT registration_number_type_registration_revision_positive
        CHECK (revision >= 1),

    -- 镜像 domain.RegistrationNumberLayer 封闭集。新增层先改领域封闭集，再改这一条。
    CONSTRAINT registration_number_type_registration_layer_closed
        CHECK (layer IN ('IDENTITY', 'PROFILE')),

    -- 停用两件（时点+依据）同进同出：只有其一的行说不出「依据什么停用」或「何时起停用」。
    CONSTRAINT registration_number_type_registration_deactivation_paired
        CHECK (
            (deactivated_at IS NULL AND deactivation_basis IS NULL)
            OR (deactivated_at IS NOT NULL
                AND deactivation_basis IS NOT NULL
                AND btrim(deactivation_basis) <> '')
        ),

    CONSTRAINT registration_number_type_registration_snapshot_object
        CHECK (snapshot IS NOT NULL AND jsonb_typeof(snapshot) = 'object')
);

-- 目录按（租户+登记时间倒序）上列；按国家 / 地区取整份目录与取某类型最新修订都经主键前缀定位，
-- 不另建索引。
CREATE INDEX registration_number_type_registration_by_tenant
    ON party_commercial.registration_number_type_registration
        (tenant_id, recorded_at DESC);
