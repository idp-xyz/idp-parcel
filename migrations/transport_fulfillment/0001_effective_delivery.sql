-- 交付生效登记：一行一版本，更正是新行指回前版，历史行只增不删。
--
-- 「当前版」用 is_current 加部分唯一索引表达：同一（租户+对象+尝试）任何时点只有
-- 一行当前——首登撞它译已登记，更正在同一事务里翻旧行再插新行。POD 必备与版本链
-- 完整性入库内 CHECK：一行没有交付证明的「交付」在领域是立不起来的，让数据库同样
-- 拦住（领域重建门是第二道，两道互补不互替）。

CREATE TABLE transport_fulfillment.effective_delivery (
    tenant_id        text        NOT NULL,
    object_ref       text        NOT NULL,
    attempt_ref      text        NOT NULL,
    delivery_version text        NOT NULL,

    place_ref        text        NOT NULL,
    method_ref       text        NOT NULL,
    recipient_ref    text        NOT NULL,
    proof_ref        text        NOT NULL,
    corrects_version text,
    corrected_at     timestamptz,
    occurred_at      timestamptz NOT NULL,
    content_digest   text        NOT NULL,
    is_current       boolean     NOT NULL,
    recorded_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT effective_delivery_pkey
        PRIMARY KEY (tenant_id, object_ref, attempt_ref, delivery_version),

    CONSTRAINT effective_delivery_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(attempt_ref) <> ''
            AND btrim(delivery_version) <> ''
        ),
    -- POD 必备：没有符合当时规则的交付证明就没有生效交付（方式与接收方同罪）。
    CONSTRAINT effective_delivery_pod_required
        CHECK (
            btrim(proof_ref) <> ''
            AND btrim(method_ref) <> ''
            AND btrim(recipient_ref) <> ''
            AND btrim(place_ref) <> ''
        ),
    CONSTRAINT effective_delivery_digest_not_blank
        CHECK (btrim(content_digest) <> ''),
    -- 版本链两形态互斥：首登无前版无更正时间，更正版两者齐且不自指、不早于交付。
    CONSTRAINT effective_delivery_chain_coherent
        CHECK (
            (corrects_version IS NULL AND corrected_at IS NULL)
            OR (
                corrects_version IS NOT NULL
                AND btrim(corrects_version) <> ''
                AND corrects_version <> delivery_version
                AND corrected_at IS NOT NULL
                AND corrected_at >= occurred_at
            )
        )
);

-- 同一（租户+对象+尝试）只有一行当前版：首登的幂等由它拦住，更正翻旧插新也由它
-- 保证不出现双当前。
CREATE UNIQUE INDEX effective_delivery_current_unique
    ON transport_fulfillment.effective_delivery (tenant_id, object_ref, attempt_ref)
    WHERE is_current;

-- 按对象尝试取回版本链，顺序稳定。
CREATE INDEX effective_delivery_by_key
    ON transport_fulfillment.effective_delivery
        (tenant_id, object_ref, attempt_ref, recorded_at, delivery_version);
