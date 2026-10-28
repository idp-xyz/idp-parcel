-- 申报发送通道、发送事实与技术回执（ADR-0342；票 product-strategy-boundary/10 第 1 项）。提交尝试行登记之后不改：越过发送边界
-- 另记一笔发送事实，后到的技术回执逐份追加，尝试此刻的技术判断由这几份折出（ADR-0242 决定三）。
--
-- 申报通道：按（租户、发送目标）登一份端点与凭据引用。端点与凭据是租户取值，不登任何租户的行；凭据只存运行环境里的变量名，
-- 密钥不进库。一个目标一份，同内容重登是重放，异内容不覆盖。
CREATE TABLE customs_compliance.declaration_channel (
    tenant_id      text        NOT NULL,
    target         text        NOT NULL,
    endpoint       text        NOT NULL,
    credential_ref text,
    registered_at  timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT declaration_channel_pkey PRIMARY KEY (tenant_id, target),
    CONSTRAINT declaration_channel_text_present
        CHECK (btrim(tenant_id) <> '' AND btrim(target) <> '' AND btrim(endpoint) <> ''),
    CONSTRAINT declaration_channel_credential_not_blank
        CHECK (credential_ref IS NULL OR btrim(credential_ref) <> '')
);

-- 发送事实：一份尝试至多一笔，结果只收越过边界之后的三种。通道给的消息号用来把异步技术回执关联回这一份尝试，一个租户内唯一；
-- 没给消息号留空，那份尝试的异步回执关联不上。
CREATE TABLE customs_compliance.submission_send_fact (
    tenant_id       text        NOT NULL,
    version_id      text        NOT NULL,
    sequence        integer     NOT NULL,
    sent_at         timestamptz NOT NULL,
    outcome         text        NOT NULL,
    channel_message text,
    response_ref    text,
    recorded_at     timestamptz NOT NULL,
    inserted_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT submission_send_fact_pkey PRIMARY KEY (tenant_id, version_id, sequence),
    CONSTRAINT submission_send_fact_attempt_exists
        FOREIGN KEY (tenant_id, version_id, sequence)
        REFERENCES customs_compliance.submission_attempt (tenant_id, version_id, sequence),
    CONSTRAINT submission_send_fact_outcome
        CHECK (outcome IN ('ACKNOWLEDGED', 'FAILED', 'PENDING_CONFIRMATION')),
    CONSTRAINT submission_send_fact_message_not_blank
        CHECK (channel_message IS NULL OR btrim(channel_message) <> '')
);

CREATE UNIQUE INDEX submission_send_fact_channel_message_unique
    ON customs_compliance.submission_send_fact (tenant_id, channel_message)
    WHERE channel_message IS NOT NULL;

-- 技术回执：按通道给这份回执的来源身份登一份，只收明确的两种。同身份同内容重到答已有，同身份异内容是来源响应冲突，都不覆盖。
CREATE TABLE customs_compliance.technical_receipt (
    tenant_id      text        NOT NULL,
    source_id      text        NOT NULL,
    version_id     text        NOT NULL,
    sequence       integer     NOT NULL,
    outcome        text        NOT NULL,
    received_at    timestamptz NOT NULL,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT technical_receipt_pkey PRIMARY KEY (tenant_id, source_id),
    CONSTRAINT technical_receipt_attempt_exists
        FOREIGN KEY (tenant_id, version_id, sequence)
        REFERENCES customs_compliance.submission_attempt (tenant_id, version_id, sequence),
    CONSTRAINT technical_receipt_outcome CHECK (outcome IN ('ACKNOWLEDGED', 'FAILED')),
    CONSTRAINT technical_receipt_text_present CHECK (btrim(source_id) <> '' AND btrim(content_digest) <> '')
);
