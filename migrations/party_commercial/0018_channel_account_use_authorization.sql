-- 渠道账号使用授权登记册（CONTEXT「渠道账号使用授权」+ ADR-0039 + ADR-0093）：
-- 渠道账号持有人允许运营企业在明确范围与期间使用该账号的业务授权，按修订版本化。
--
-- 本册不进 commercial_version。ADR-0093 裁定它不是商业版本：集内成员均持有一个
-- CommercialVersion，而它自带状态与发布时刻、走自己的生命周期，与参与方身份四表同族
-- （0015）。因此这里也没有版本壳可指，授权正文整个落在本表——没有第二处可放。
--
-- 键=租户+登记标识+修订，修订从 1 起连续递增；撤销形成新修订，绝不 UPDATE
-- （CONTEXT「不删除已经形成的授权证据和交易快照」，同 0015 四册与 0016 的登记面纪律）。
--
-- authorization_id 不是 channel_account_id：同一个渠道账号可以在不同期间授给不同的
-- 被授权人，那是两笔授权而不是一笔的两个修订（ADR-0093 决定二）。拿账号当键会把它们
-- 挤成一条链，最新修订于是覆盖掉一段仍需追溯的历史。
--
-- 册间不设外键——登记册各自只增，悬空引用由写入用例把门（同 0015 legal_entity_registration
-- 的裁决）。渠道本体更是不在本上下文预造（ADR-0072 否决预拟渠道表）。
--
-- 有一条本表**守不住**、只能由领域信封守的：后继修订不得改换账号或授权双方。主键只守键
-- 唯一，一条修订二把关系整个换掉照样入库，而它在册上仍像同一笔授权的后继
-- （ADR-0093 决定六，实现在 domain.ChannelAccountUseAuthorizationRegistration.Succeed）。
-- 写在这里是因为读表的人会以为约束齐了。

CREATE TABLE party_commercial.channel_account_use_authorization (
    tenant_id           text        NOT NULL,
    authorization_id    text        NOT NULL,
    revision            integer     NOT NULL,

    channel_account_id  text        NOT NULL,
    grantor_party_id    text        NOT NULL,
    grantee_party_id    text        NOT NULL,
    channel_ref         text        NOT NULL,
    scope_ref           text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,

    status              text        NOT NULL,
    published_at        timestamptz NOT NULL,
    revoked_at          timestamptz,
    revoked_on          text,

    content_digest      text        NOT NULL,
    snapshot            jsonb       NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT channel_account_use_authorization_pkey
        PRIMARY KEY (tenant_id, authorization_id, revision),

    CONSTRAINT channel_account_use_authorization_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(authorization_id) <> ''
            AND btrim(channel_account_id) <> ''
            AND btrim(grantor_party_id) <> ''
            AND btrim(grantee_party_id) <> ''
            AND btrim(channel_ref) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT channel_account_use_authorization_revision_positive
        CHECK (revision >= 1),

    -- 授权双方必须是两方。自授不是授权，它绕过的正是 ADR-0039 要守的那件事：
    -- 持有人的许可必须来自持有人。领域构造门同判（grantor == grantee 即无效）。
    CONSTRAINT channel_account_use_authorization_parties_distinct
        CHECK (grantor_party_id <> grantee_party_id),

    -- 自然到期**不在**状态集里：它由有效区间对时点导出，存成状态就是存一份会过期的
    -- 推导（ADR-0093 决定三）。撤销必须是取值，因为它是一次外部决定，无处可导。
    CONSTRAINT channel_account_use_authorization_status_closed
        CHECK (status IN ('PUBLISHED', 'REVOKED')),

    CONSTRAINT channel_account_use_authorization_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at),

    -- 撤销时刻与撤销依据同在或同缺，且与状态互为充要（ADR-0093 决定五）。只记时刻答不出
    -- 凭什么撤，只记依据答不出从哪一刻起不能用——而后续新使用的判定要的正是那一刻。
    -- 三者拆成两条独立 CHECK 会放过「状态 REVOKED 但两列皆空」，那种行读起来像已撤销，
    -- 却举不出任何撤销事实。
    CONSTRAINT channel_account_use_authorization_revocation_coupled
        CHECK (
            (status = 'PUBLISHED' AND revoked_at IS NULL AND revoked_on IS NULL)
            OR (
                status = 'REVOKED'
                AND revoked_at IS NOT NULL
                AND revoked_on IS NOT NULL
                AND btrim(revoked_on) <> ''
            )
        ),

    -- 撤销不能早于发布。早于发布的撤销时刻会让 AllowsUseAt 对整个有效期答假，把一笔
    -- 从未生效过的授权伪装成曾经有效又被撤——两者在册上本该分得开。
    CONSTRAINT channel_account_use_authorization_revoked_after_published
        CHECK (revoked_at IS NULL OR revoked_at >= published_at),

    CONSTRAINT channel_account_use_authorization_snapshot_object
        CHECK (snapshot IS NOT NULL AND jsonb_typeof(snapshot) = 'object')
);

-- 目录按（租户+登记时间倒序）上列；最新修订经主键即可定位，不另建索引（同 0015、0016）。
CREATE INDEX channel_account_use_authorization_by_tenant
    ON party_commercial.channel_account_use_authorization
        (tenant_id, recorded_at DESC);

-- 按渠道账号回查「此刻谁被授权用这个账号」。这条不是目录列表用的，是委托接受与面单交易
-- 两处在决定时点重新校验当前授权时走的路（CONTEXT 要求形成新交易时独立校验，不复用
-- 委托接受时的快照），那条路以账号而非登记标识发问。
CREATE INDEX channel_account_use_authorization_by_account
    ON party_commercial.channel_account_use_authorization
        (tenant_id, channel_account_id, recorded_at DESC);
