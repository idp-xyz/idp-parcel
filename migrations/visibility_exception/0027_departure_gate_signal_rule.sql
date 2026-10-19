-- 出发门禁阻断信号规则（`PAR-VIS-04` 首发异常类型清单里「违规出发」那一类；ADR-0205 决定五与文末补记）：出发登记时关务
-- 对某一件答阻断、车照样开走了，全程追踪据此形成异常信号时，用哪个信号类型、哪一版识别规则、记什么可信度依据。
--
-- 识别条件本身归产品（「出发门禁读数里这一件是阻断」不看任何租户就答得出）；类型、规则版本、可信度与启不启用这一类，
-- 属租户按其异常类型清单登记的内容。表在首发是空的；空表时读口交回「未配置」，阻断那一件照常进投影，只是没有可形成的
-- 适用信号——不虚构一种异常类型送进分诊。演示租户经 parcel-ve-register 采用，证据只记 S。
--
-- 一租户一条、不可覆盖，理由与冲突信号规则（0025）同：已据这条规则形成过的发作期指着它的规则版本，静默换掉会让历史
-- 发作期「按哪版判的」对不上；要换就先裁一条换版路径（另立票），这里不预留。
CREATE TABLE visibility_exception.departure_gate_signal_rule (
    tenant_id      text        NOT NULL,

    signal_kind    text        NOT NULL,
    rule_version   text        NOT NULL,
    confidence_ref text        NOT NULL,
    approved_by    text        NOT NULL,
    registered_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT departure_gate_signal_rule_pkey PRIMARY KEY (tenant_id),

    CONSTRAINT departure_gate_signal_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(signal_kind) <> ''
            AND btrim(rule_version) <> ''
            AND btrim(confidence_ref) <> ''
            AND btrim(approved_by) <> ''
        )
);
