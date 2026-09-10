-- 授权动作封闭集加「受控关闭」「重开」两格（票 party-commercial-context-gaps/13；PC CONTEXT Rules「受控关闭与重开是与
-- 前述各动作并列的两个授权动作……关闭权不蕴含重开权」）：CONTROLLED_CLOSURE 与 REOPENING，原词取 parcel-shipment
-- 的 ContinuedAttemptDecisionKind 同词。0003 立表、0025 重建过一次的 authorization_grant_action_closed 把当时的字面量钉在
-- CHECK 里，租户因此无处登记关闭 / 重开授权；不改已施加的 0003 / 0025，这里 DROP 再 ADD 同名约束，镜像
-- domain.AuthorizedAction 此刻的 valid()（先例：0017 / 0019 / 0025 / 0031 同一写法）。
--
-- 只放宽可选值：主键、快照、区间 CHECK 不动；不预填任何行——哪个法人、哪个等级、哪个范围可关可重开属实例半边
--（PAR-COM-14），今天没有租户因而两格全空，缺省落 ErrAuthorityRulesNotConfigured。
--
-- contract_delegation 的 action CHECK **不扩**：关闭 / 重开是运营侧凭授权规则形成的决定，不是客户拥有的决定，
-- 合同委派不得挂它们（ADR-0116 决定二；票面裁决 ③ 例外支首发不开、留位）。
--
-- 既有行全在旧字面量之内，新 CHECK 对它们恒真；ADD CONSTRAINT 默认校验存量行，这一点由它自己证。

ALTER TABLE party_commercial.authorization_grant
    DROP CONSTRAINT authorization_grant_action_closed;

ALTER TABLE party_commercial.authorization_grant
    ADD CONSTRAINT authorization_grant_action_closed
        CHECK (action IN (
            'MANUAL_REVIEW',
            'ACTIVE_REJECTION',
            'SOURCE_DATA_AMENDMENT',
            'CONTROLLED_CLOSURE',
            'REOPENING'
        ));
