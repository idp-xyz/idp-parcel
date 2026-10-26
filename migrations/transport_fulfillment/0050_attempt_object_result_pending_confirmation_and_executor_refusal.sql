-- 履约尝试的逐对象结果加「待确认」一格，揽收另加「执行方拒收」一格（票 docs-consistency/code/01；TF CONTEXT「揽收、派送与
-- 交付」：每个对象分别保存成功、失败、拒收、待确认或其他适用结果）。
--
-- 待确认（PENDING_CONFIRMATION）在揽收与派送两张结果表上同词：已有来源不足以证明接收、控制、交付或对象范围。它带两样，
-- 都必备——source_ref 是所凭的已有来源，basis 是缺口或冲突；确定结果不带 source_ref（揽收到手与妥投的证据在 OffsitePickup
-- 与 POD 上，失败与拒收的证据在 basis 上）。待确认不接揽收行：pickup_attempt_pickup 的外键只接 outcome='PICKED_UP'，
-- 待确认行接不上，「待确认不形成揽收到手」在库面照旧成立。
--
-- 执行方拒收（EXECUTOR_REFUSED）只在揽收表上：执行方查验后明确拒收该对象，原因与证据在 basis 上，与失败同形必带。派送
-- 一侧的拒收（REFUSED）是收件人拒收，早已在表上。
--
-- 存量行共存：已有的行结果都在旧的封闭集里、source_ref 新加为空，新的三条 CHECK 对它们恒成立，直接校验，不必 NOT VALID。
-- 这一格只放开不收紧：旧写入方写出的行形状（不写 source_ref）照旧合法。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：basis 与 source_ref 可空，先 IS NULL / IS NOT NULL 再比；outcome
-- 非空。

ALTER TABLE transport_fulfillment.pickup_attempt_result
    ADD COLUMN source_ref text;

ALTER TABLE transport_fulfillment.pickup_attempt_result
    DROP CONSTRAINT pickup_attempt_result_outcome_closed;

ALTER TABLE transport_fulfillment.pickup_attempt_result
    ADD CONSTRAINT pickup_attempt_result_outcome_closed
        CHECK (outcome IN (
            'PICKED_UP', 'CUSTOMER_ABSENT', 'GOODS_NOT_READY', 'PACKAGING_UNACCEPTABLE',
            'EXECUTOR_REFUSED', 'PENDING_CONFIRMATION'
        ));

-- 成功不得带依据；失败、拒收与待确认必须带（原因、拒收原因与证据、缺口或冲突）。
ALTER TABLE transport_fulfillment.pickup_attempt_result
    DROP CONSTRAINT pickup_attempt_result_outcome_shape;

ALTER TABLE transport_fulfillment.pickup_attempt_result
    ADD CONSTRAINT pickup_attempt_result_outcome_shape
        CHECK (
            (outcome = 'PICKED_UP' AND basis IS NULL)
            OR (outcome <> 'PICKED_UP' AND basis IS NOT NULL AND btrim(basis) <> '')
        );

-- 来源只属待确认，且待确认必带来源。
ALTER TABLE transport_fulfillment.pickup_attempt_result
    ADD CONSTRAINT pickup_attempt_result_source_only_when_pending
        CHECK (
            (outcome = 'PENDING_CONFIRMATION' AND source_ref IS NOT NULL AND btrim(source_ref) <> '')
            OR (outcome <> 'PENDING_CONFIRMATION' AND source_ref IS NULL)
        );

ALTER TABLE transport_fulfillment.delivery_attempt_result
    ADD COLUMN source_ref text;

ALTER TABLE transport_fulfillment.delivery_attempt_result
    DROP CONSTRAINT delivery_attempt_result_outcome_closed;

-- 五值结果在库里也封闭（domain.DeliveryObjectOutcome）。delivery_attempt_result_basis_coupled 不动：它本就写成「妥投不带、
-- 其余必带」，待确认落在「其余」里。
ALTER TABLE transport_fulfillment.delivery_attempt_result
    ADD CONSTRAINT delivery_attempt_result_outcome_closed
        CHECK (outcome IN ('DELIVERED', 'REFUSED', 'NO_ONE_TO_RECEIVE', 'WRONG_ADDRESS', 'PENDING_CONFIRMATION'));

ALTER TABLE transport_fulfillment.delivery_attempt_result
    ADD CONSTRAINT delivery_attempt_result_source_only_when_pending
        CHECK (
            (outcome = 'PENDING_CONFIRMATION' AND source_ref IS NOT NULL AND btrim(source_ref) <> '')
            OR (outcome <> 'PENDING_CONFIRMATION' AND source_ref IS NULL)
        );
