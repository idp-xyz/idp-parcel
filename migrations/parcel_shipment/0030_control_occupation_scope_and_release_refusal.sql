-- 控制结果存下占用所在的结算作用域，另立释放未受理册（ADR-0282；票 docs-consistency/code/71）。
--
-- acceptance_financial_control 加三列：提供方为这次控制形成占用时所在的结算作用域（责任法人 / 结算账户 / 币种）。
-- 释放按存下的这一份认领，不在释放那一刻从商业解析与账户登记重新推导——政策回显或账户登记在施加之后变了，推出
-- 来的是另一份作用域，提供方在那里找不到这一笔、答无可释放，原占用静默成孤儿。三列同进同出：缺一段的作用域认领
-- 不到任何一笔。`明确无控制`没有占用，也就没有作用域。存量行三列为 NULL，不回填：释放照样把空作用域交给提供方，
-- 由它答未受理、落进下面那本册（ADR-0282 决定四）。
--
-- control_release_refusal 是释放未受理册：提供方确定不受理的释放（身份或作用域立不起来，或本上下文译不动它的
-- 答复）落在这里交人处理，业务决定照常成立、不回滚，也不留一条没有东西推得动的补偿续办引用（ADR-0282 决定二）。
-- 只追加，一笔占用一行：主键是（租户，委托，提交版本，控制结果）。重放撤回、或暂停与撤回先后遇到同一笔，拒收的
-- 都是同一笔占用，撞主键即已有记录。作用域三列照抄请求里交出的那一份，存量结果没有就是 NULL——读的人据此分得出
-- 是作用域没存下还是存下了也被拒收。
--
-- 全部 CHECK 走 IS NULL / IS NOT NULL 缝（f822e1d 入册的三值逻辑纪律）。

ALTER TABLE parcel_shipment.acceptance_financial_control
    ADD COLUMN occupation_legal_entity       text,
    ADD COLUMN occupation_settlement_account text,
    ADD COLUMN occupation_currency           text,
    ADD CONSTRAINT acceptance_financial_control_occupation_scope_shape
        CHECK (
            (occupation_legal_entity IS NULL
                AND occupation_settlement_account IS NULL
                AND occupation_currency IS NULL)
            OR (occupation_legal_entity IS NOT NULL AND btrim(occupation_legal_entity) <> ''
                AND occupation_settlement_account IS NOT NULL AND btrim(occupation_settlement_account) <> ''
                AND occupation_currency IS NOT NULL AND btrim(occupation_currency) <> ''
                AND control_outcome <> 'NOT_APPLICABLE')
        );

CREATE TABLE parcel_shipment.control_release_refusal (
    tenant_id                     text        NOT NULL,
    shipment_request_id           text        NOT NULL,
    submission_version            text        NOT NULL,
    control_result_id             text        NOT NULL,

    customer_account_id           text        NOT NULL,
    occupation_legal_entity       text,
    occupation_settlement_account text,
    occupation_currency           text,
    reason_ref                    text        NOT NULL,
    recorded_at                   timestamptz NOT NULL,

    CONSTRAINT control_release_refusal_pkey
        PRIMARY KEY (tenant_id, shipment_request_id, submission_version, control_result_id),

    CONSTRAINT control_release_refusal_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(submission_version) <> ''
            AND btrim(control_result_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(reason_ref) <> ''
        ),

    CONSTRAINT control_release_refusal_occupation_scope_shape
        CHECK (
            (occupation_legal_entity IS NULL
                AND occupation_settlement_account IS NULL
                AND occupation_currency IS NULL)
            OR (occupation_legal_entity IS NOT NULL AND btrim(occupation_legal_entity) <> ''
                AND occupation_settlement_account IS NOT NULL AND btrim(occupation_settlement_account) <> ''
                AND occupation_currency IS NOT NULL AND btrim(occupation_currency) <> '')
        )
);

-- 按委托查册（ControlReleaseRefusalRegister.ListByShipmentRequest）由主键前缀承担，不另建索引。
