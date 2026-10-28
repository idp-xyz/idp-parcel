-- 收寄后服务处置决定（ADR-0332；票 docs-consistency/code/37）。
--
-- 一、待处置记录保全请求方。越过取消边界的取消请求落成 DISPOSITION_PENDING（0004），原先只带越过的收寄版本，请求方、授权依据、原因和请求的业务时间都没有
-- 保全，决定人看不到自己在回应谁的什么请求。本迁移放宽待处置那一支的在场件矩阵：这四列要么全空（本迁移之前落下的记录，当时没有保全）、要么全在且非空白。
-- 取消成立与拒绝两支不动。
--
-- 二、post_intake_disposition_decision 记下有权主体对一件越界包裹形成的处置决定。每个包裹一条线性的决定链：版本从 1 起，后一份取代前一份为当前决定，前面的
-- 行不改不删——撤回与更正都是在链尾追加一份。主键（租户，包裹，版本）；supersedes_version 指向前一版（外键，首份为空），CHECK 把链钉成线性
-- （version = supersedes_version + 1），所以链不分叉。同一份请求至多被一份决定回应：(租户，请求键，包裹) 唯一，外键指向取消记录——回应的请求必须在册；它是
-- 待处置记录还是被认定不成立的取消，库里不验（外键验不了），由编排判。
--
-- direction 封闭五值；source 封闭两值，且与 request_key 的在场互为充要；destination_ref 只在改送上在场，改送必带。responsibility_start 是决定形成时在册的责任起点版本，
-- 决定的责任范围。decided_by 是（发行方，sub）这一对的引用，decision_basis 是决定人核对后留下的依据，两样必填。
--
-- 全部 CHECK 走 IS NULL / IS NOT NULL 缝（f822e1d 入册的三值逻辑纪律）。

ALTER TABLE parcel_shipment.parcel_cancellation
    DROP CONSTRAINT parcel_cancellation_kind_shape;

ALTER TABLE parcel_shipment.parcel_cancellation
    ADD CONSTRAINT parcel_cancellation_kind_shape
        CHECK (
            (kind = 'PARCEL_CANCELLED'
                AND cancellation_id IS NOT NULL AND btrim(cancellation_id) <> ''
                AND requester_ref IS NOT NULL AND btrim(requester_ref) <> ''
                AND authority_ref IS NOT NULL AND btrim(authority_ref) <> ''
                AND reason_ref IS NOT NULL AND btrim(reason_ref) <> ''
                AND requested_at IS NOT NULL
                AND crossed_intake_version IS NULL
                AND refusal_basis IS NULL)
            OR (kind = 'DISPOSITION_PENDING'
                AND crossed_intake_version IS NOT NULL AND btrim(crossed_intake_version) <> ''
                AND cancellation_id IS NULL
                AND refusal_basis IS NULL
                AND (
                    (requester_ref IS NULL AND authority_ref IS NULL AND reason_ref IS NULL AND requested_at IS NULL)
                    OR (requester_ref IS NOT NULL AND btrim(requester_ref) <> ''
                        AND authority_ref IS NOT NULL AND btrim(authority_ref) <> ''
                        AND reason_ref IS NOT NULL AND btrim(reason_ref) <> ''
                        AND requested_at IS NOT NULL)
                ))
            OR (kind = 'CANCELLATION_REFUSED'
                AND refusal_basis IS NOT NULL AND btrim(refusal_basis) <> ''
                AND cancellation_id IS NULL
                AND requester_ref IS NULL
                AND authority_ref IS NULL
                AND reason_ref IS NULL
                AND requested_at IS NULL
                AND crossed_intake_version IS NULL)
        );

-- 运营查阅「等回应的请求」按租户取最早的待处置记录；主键与 parcel_cancellation_by_parcel 的前缀都按包裹排，按租户扫用不上，这条部分索引只收待处置那一种。
CREATE INDEX parcel_cancellation_pending_by_tenant
    ON parcel_shipment.parcel_cancellation (tenant_id, decided_at)
    WHERE kind = 'DISPOSITION_PENDING';

CREATE TABLE parcel_shipment.post_intake_disposition_decision (
    tenant_id            text        NOT NULL,
    parcel_id            text        NOT NULL,
    version              integer     NOT NULL,
    supersedes_version   integer,

    direction            text        NOT NULL,
    source               text        NOT NULL,
    request_key          text,
    destination_ref      text,
    responsibility_start text        NOT NULL,

    decided_by           text        NOT NULL,
    decision_basis       text        NOT NULL,
    decided_at           timestamptz NOT NULL,

    CONSTRAINT post_intake_disposition_decision_pkey
        PRIMARY KEY (tenant_id, parcel_id, version),

    CONSTRAINT post_intake_disposition_decision_chain_fkey
        FOREIGN KEY (tenant_id, parcel_id, supersedes_version)
        REFERENCES parcel_shipment.post_intake_disposition_decision (tenant_id, parcel_id, version),

    CONSTRAINT post_intake_disposition_decision_request_fkey
        FOREIGN KEY (tenant_id, request_key, parcel_id)
        REFERENCES parcel_shipment.parcel_cancellation (tenant_id, request_key, parcel_id),

    -- 一份请求至多被一份决定回应（request_key 为空的运营发起决定不受限，NULL 之间互不相等）。
    CONSTRAINT post_intake_disposition_decision_request_answered_once
        UNIQUE (tenant_id, request_key, parcel_id),

    CONSTRAINT post_intake_disposition_decision_direction_closed
        CHECK (direction IN ('CONTINUE', 'INTERCEPT', 'REDIRECT', 'RETURN', 'TERMINATE_SERVICE')),

    CONSTRAINT post_intake_disposition_decision_source_closed
        CHECK (source IN ('CUSTOMER_REQUEST', 'OPERATOR_INITIATED')),

    -- 来源与请求引用互为充要；改送带目的地、别的去向不带。
    CONSTRAINT post_intake_disposition_decision_shape
        CHECK (
            ((source = 'CUSTOMER_REQUEST' AND request_key IS NOT NULL AND btrim(request_key) <> '')
                OR (source = 'OPERATOR_INITIATED' AND request_key IS NULL))
            AND ((direction = 'REDIRECT' AND destination_ref IS NOT NULL AND btrim(destination_ref) <> '')
                OR (direction <> 'REDIRECT' AND destination_ref IS NULL))
        ),

    -- 链是线性的：首份没有前版，其余恰好取代紧挨着的前一版。
    CONSTRAINT post_intake_disposition_decision_chain_linear
        CHECK (
            version >= 1
            AND ((version = 1 AND supersedes_version IS NULL)
                OR (version > 1 AND supersedes_version IS NOT NULL AND supersedes_version = version - 1))
        ),

    CONSTRAINT post_intake_disposition_decision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(responsibility_start) <> ''
            AND btrim(decided_by) <> ''
            AND btrim(decision_basis) <> ''
        )
);
