-- 冲突信号规则（`PAR-VIS-04` 首发异常类型清单里「事实冲突待确认」那一类）：无法按业务
-- 时间裁决的替代链分叉形成异常信号时，用哪个信号类型、哪一版识别规则、记什么可信度依据。
-- 在此之前 ResolveByBusinessTime 与 RaiseConflictSignal 没有生产调用方：投影派生把分叉
-- 双方留在场，却不裁决也不发信号。
--
-- 三样全部属实例半边：事实冲突该算哪一类异常、依据哪一版规则、可信度怎么记，都是租户
-- 按其异常类型清单登记的内容。表在首发是空的；空表时读口交回「未配置」，编排把冲突记为
-- 「无适用信号规则」——冲突照常留在投影里按信息待确认表达，只是没有可形成的适用信号，
-- 不虚构一种异常类型送进分诊。
--
-- 一租户一条、不可覆盖：键只有租户，撞既有行写入口交回 AlreadyRegistered。换规则版本是
-- 一次治理动作——已据这条规则形成过的信号发作期指着它的规则版本，静默换掉会让历史发作期
-- 「按哪版判的」对不上；要换就先裁一条换版路径（另立票），这里不预留。
CREATE TABLE visibility_exception.conflict_signal_rule (
    tenant_id      text        NOT NULL,

    signal_kind    text        NOT NULL,
    rule_version   text        NOT NULL,
    confidence_ref text        NOT NULL,
    approved_by    text        NOT NULL,
    registered_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT conflict_signal_rule_pkey PRIMARY KEY (tenant_id),

    CONSTRAINT conflict_signal_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(signal_kind) <> ''
            AND btrim(rule_version) <> ''
            AND btrim(confidence_ref) <> ''
            AND btrim(approved_by) <> ''
        )
);
