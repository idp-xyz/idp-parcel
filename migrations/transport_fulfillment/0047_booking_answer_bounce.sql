-- 承运答复挡回册（ADR-0239 决定二、三；票 demo-chain-gaps/145）。
--
-- 承运答复口对「订舱在本租户下不在册」与「答复方不是该订舱所属委托的服务提供方」答同一格 `BOOKING_NOT_FOUND`：同一租户的承运商
-- 之间常是竞争方，分两格就让一个承运商探得出别人的订舱标识在不在。对外合并，对内不合并——每一次挡回凭的是哪一种，记在这里，审计
-- 与运营据此分清「这个承运商答了不归它答的订舱」与「运营侧还没把订舱录进来」。写它的只有答复编排，与那一次答复同笔。
--
-- **一个（租户，答复方，订舱，答复标识，依据）一行。** 同键再来不另起一行，只加次数、推后最近一次时刻：键里没有答复的内容，同一
-- 答复标识换了载荷再交也并进同一个计数。一次一行会让一个按重试策略自动重交的承运系统把册写满。依据进键：订舱起先不在册、后来
-- 录在别的承运商名下，同一份答复前后两次被挡回的依据不同，两行都是实情。
--
-- **依据定宽存。** 两条挡回路写进同一张表、同一个键，键上唯一由路决定的那一格就是依据；存成长短不一的词，键长就随路而变，挑一个
-- 恰好卡在索引行上限边上的答复标识，一条路写得进、一条写不进，合并掉的那一位又从答复里漏回来。存成 smallint，两条路
-- 写的键逐字节等长。1 是订舱不在册，2 是不是服务提供方；词由 ports.BookingAnswerBounceBasis 交，库里只存码。
--
-- **本表是台账，不是事实来源。** 挡回不形成答复，订舱的当前读法（`待确认`、`候补`）不读本表；答复的内容（五值、接受量、时刻）不在
-- 这里——那是一份没有被收下的报文，收下的那一份在答复册。
--
-- 订舱标识与答复标识从本迁移起有字节界（domain 的 storedReferenceMaxBytes）：订舱标识进订舱册、答复册与本表的键，答复标识进
-- 答复册与本表的键，不设界就会在写入那一刻整笔回滚、答不出业务结论。界只由领域构造门守，库上不另加 CHECK，与 parcel-shipment
-- 外部标识号码的字节界同一先例：一个数只定义在一处。界立起来之前已经落库、超过界的行，此后过不了那道门——承运总单读回它指名的
-- 订舱、答复册读回答复标识都要再过一遍——所以有这样的行就拒绝施加，不替任何人截短一个标识，那是提交它的一方才改得了的。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM transport_fulfillment.booking_request WHERE octet_length(booking_id) > 512)
       OR EXISTS (SELECT 1 FROM transport_fulfillment.booking_answer
                   WHERE octet_length(booking_id) > 512
                      OR octet_length(acceptance_id) > 512)
       OR EXISTS (SELECT 1 FROM transport_fulfillment.carrier_master_document WHERE octet_length(booking_ref) > 512) THEN
        RAISE EXCEPTION USING MESSAGE =
            '订舱册、答复册或承运总单里有超过 512 字节的订舱标识或答复标识：请导出这些行备查，由提交它们的一方以不超过'
            || ' 512 字节的标识重新提交后删去原行，再重跑本迁移。';
    END IF;
END
$$;

CREATE TABLE transport_fulfillment.booking_answer_bounce (
    tenant_id        text        NOT NULL,
    -- 答复方的来源身份，取自集成客户端绑定（ADR-0202 决定二），不是载荷自报。
    source_ref       text        NOT NULL,
    -- 载荷指名的订舱与答复标识，照原样记：依据是「订舱不在册」时，这个订舱在本租户下本就没有。
    booking_id       text        NOT NULL,
    acceptance_id    text        NOT NULL,
    basis            smallint    NOT NULL,
    first_bounced_at timestamptz NOT NULL,
    last_bounced_at  timestamptz NOT NULL,
    bounce_count     bigint      NOT NULL,

    CONSTRAINT booking_answer_bounce_pkey
        PRIMARY KEY (tenant_id, source_ref, booking_id, acceptance_id, basis),

    CONSTRAINT booking_answer_bounce_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(booking_id) <> ''
            AND btrim(acceptance_id) <> ''
        ),

    CONSTRAINT booking_answer_bounce_basis_closed
        CHECK (basis IN (1, 2)),

    CONSTRAINT booking_answer_bounce_count_positive
        CHECK (bounce_count >= 1),

    CONSTRAINT booking_answer_bounce_times_ordered
        CHECK (last_bounced_at >= first_bounced_at)
);
