-- 承运答复拒收册（ADR-0239 决定二；票 demo-chain-gaps/145）。
--
-- 承运答复口对「订舱在本租户下不在册」与「答复方不是该订舱所属委托的服务提供方」答同一格 `BOOKING_NOT_FOUND`：同一租户的承运商
-- 之间常是竞争方，分两格就让一个承运商探得出别人的订舱标识在不在。对外合并，对内不合并——每一次拒收凭什么拒，记在这里，审计与
-- 运营据此分清「这个承运商答了不归它答的订舱」与「运营侧还没把订舱录进来」。写它的只有答复编排，与那一次答复同笔。
--
-- **一个（租户，答复方，订舱，答复标识，依据）一行。** 同一份答复原样重交不另起一行，只加次数、推后最近一次时刻：重交本身不是
-- 新的信息，一次一行会让一个按重试策略自动重交的承运系统把册写满。依据进键：订舱起先不在册、后来录在别的承运商名下，同一份答复
-- 前后两次被拒的依据不同，两行都是实情。
--
-- **本表是台账，不是事实来源。** 拒收不形成答复，订舱的当前读法（`待确认`、`候补`）不读本表；答复的内容（五值、接受量、时刻）不在
-- 这里——那是一份没有被收下的报文，收下的那一份在答复册。
CREATE TABLE transport_fulfillment.booking_answer_refusal (
    tenant_id        text        NOT NULL,
    -- 答复方的来源身份，取自集成客户端绑定（ADR-0202 决定二），不是载荷自报。
    source_ref       text        NOT NULL,
    -- 载荷指名的订舱与答复标识，照原样记：依据是「订舱不在册」时，这个订舱在本租户下本就没有。
    booking_id       text        NOT NULL,
    acceptance_id    text        NOT NULL,
    refusal_basis    text        NOT NULL,
    first_refused_at timestamptz NOT NULL,
    last_refused_at  timestamptz NOT NULL,
    refusal_count    bigint      NOT NULL,

    CONSTRAINT booking_answer_refusal_pkey
        PRIMARY KEY (tenant_id, source_ref, booking_id, acceptance_id, refusal_basis),

    CONSTRAINT booking_answer_refusal_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(booking_id) <> ''
            AND btrim(acceptance_id) <> ''
        ),

    CONSTRAINT booking_answer_refusal_basis_closed
        CHECK (refusal_basis IN ('BOOKING_NOT_REGISTERED', 'SOURCE_NOT_SERVICE_PROVIDER')),

    CONSTRAINT booking_answer_refusal_count_positive
        CHECK (refusal_count >= 1),

    CONSTRAINT booking_answer_refusal_times_ordered
        CHECK (last_refused_at >= first_refused_at)
);
