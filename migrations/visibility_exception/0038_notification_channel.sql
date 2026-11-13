-- 通知渠道登记：通知策略指名的渠道引用，对到租户登记的推送端点（UC-VE-006 步 6）。
--
-- 产品随附的渠道连接器只有一种形态：把通知以 JSON POST 到客户系统的端点。端点与凭据是租户取值，本迁移不种任何行；
-- 策略指名了渠道而这里没有登记，通知如实答未配置。
--
-- credential_ref 是凭据在运行环境里的变量名，不是凭据本身；空串表示这个渠道不带凭据。同一（租户，渠道引用）重登即
-- 换端点——客户系统换地址是正常事，改的是往后的发送，已记下的过程节点不动。

CREATE TABLE visibility_exception.notification_channel (
    tenant_id      text        NOT NULL,
    channel_ref    text        NOT NULL,
    endpoint       text        NOT NULL,
    credential_ref text        NOT NULL DEFAULT '',
    registered_at  timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, channel_ref),
    CONSTRAINT notification_channel_filled
        CHECK (btrim(tenant_id) <> '' AND btrim(channel_ref) <> '' AND btrim(endpoint) <> '')
);
