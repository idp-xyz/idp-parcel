-- Parcel shipment owns the immutable source preservation and the submitted
-- shipment request skeleton. Every table carries the group tenant and the
-- shipper customer account explicitly; no query may rely on in-memory filtering.

-- Immutable preservation of one raw intake request. Later parsing or dependency
-- failures never delete a row here, and a repeated request never overwrites the
-- preserved content or digest.
CREATE TABLE parcel_shipment.source_submission (
    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    source              text        NOT NULL,
    source_request_key  text        NOT NULL,

    raw_content_ref     text        NOT NULL,
    payload_digest      text        NOT NULL,
    source_occurred_at  timestamptz NOT NULL,
    system_received_at  timestamptz NOT NULL,
    correlation_id      text,

    intake_outcome      text,
    not_admitted_reason text,
    parse_rule_version  text,
    submission_batch_id text,

    CONSTRAINT source_submission_pkey
        PRIMARY KEY (tenant_id, customer_account_id, source, source_request_key),
    CONSTRAINT source_submission_outcome_known
        CHECK (intake_outcome IS NULL OR intake_outcome IN ('ACCEPTED', 'NOT_ADMITTED', 'CONFLICT')),
    -- A not admitted intake must carry a deterministic reason and must not
    -- reference a batch: no placeholder shipment request exists.
    CONSTRAINT source_submission_not_admitted_shape
        CHECK (
            intake_outcome <> 'NOT_ADMITTED'
            OR (not_admitted_reason IS NOT NULL AND submission_batch_id IS NULL)
        ),
    CONSTRAINT source_submission_accepted_shape
        CHECK (intake_outcome <> 'ACCEPTED' OR submission_batch_id IS NOT NULL)
);

-- Batch grouping only. A batch never owns service responsibility, and one
-- request failing never rolls back a sibling that legitimately reached
-- SUBMITTED.
CREATE TABLE parcel_shipment.submission_batch (
    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    submission_batch_id text        NOT NULL,
    source              text        NOT NULL,
    source_request_key  text        NOT NULL,
    recorded_at         timestamptz NOT NULL,

    CONSTRAINT submission_batch_pkey
        PRIMARY KEY (tenant_id, customer_account_id, submission_batch_id)
);

CREATE TABLE parcel_shipment.shipment_request (
    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    shipment_request_id text        NOT NULL,

    submission_batch_id text        NOT NULL,
    source              text        NOT NULL,
    source_request_key  text        NOT NULL,

    lifecycle_state     text        NOT NULL,
    submission_version  integer     NOT NULL,
    event_id            text        NOT NULL,

    source_occurred_at  timestamptz NOT NULL,
    submitted_at        timestamptz NOT NULL,
    revision            bigint      NOT NULL,

    CONSTRAINT shipment_request_pkey
        PRIMARY KEY (tenant_id, customer_account_id, shipment_request_id),
    -- This slice only produces SUBMITTED. Acceptance and rejection belong to a
    -- later slice and must not be writable by unimplemented code.
    CONSTRAINT shipment_request_state_implemented
        CHECK (lifecycle_state = 'SUBMITTED'),
    CONSTRAINT shipment_request_version_positive
        CHECK (submission_version > 0),
    CONSTRAINT shipment_request_revision_not_negative
        CHECK (revision >= 0),
    CONSTRAINT shipment_request_batch_fkey
        FOREIGN KEY (tenant_id, customer_account_id, submission_batch_id)
        REFERENCES parcel_shipment.submission_batch
            (tenant_id, customer_account_id, submission_batch_id),
    -- The event id is generated once per logical request, so a retry cannot
    -- create a second semantically equal event.
    CONSTRAINT shipment_request_event_id_unique
        UNIQUE (tenant_id, event_id)
);

CREATE INDEX shipment_request_source_idx
    ON parcel_shipment.shipment_request
        (tenant_id, customer_account_id, source, source_request_key);

CREATE TABLE parcel_shipment.declared_parcel (
    tenant_id           text     NOT NULL,
    customer_account_id text     NOT NULL,
    shipment_request_id text     NOT NULL,
    declared_parcel_id  text     NOT NULL,
    customer_reference  text,
    declared_position   integer  NOT NULL,

    CONSTRAINT declared_parcel_pkey
        PRIMARY KEY (tenant_id, customer_account_id, shipment_request_id, declared_parcel_id),
    CONSTRAINT declared_parcel_request_fkey
        FOREIGN KEY (tenant_id, customer_account_id, shipment_request_id)
        REFERENCES parcel_shipment.shipment_request
            (tenant_id, customer_account_id, shipment_request_id)
        ON DELETE CASCADE,
    CONSTRAINT declared_parcel_position_positive
        CHECK (declared_position > 0),
    CONSTRAINT declared_parcel_position_unique
        UNIQUE (tenant_id, customer_account_id, shipment_request_id, declared_position)
        DEFERRABLE INITIALLY IMMEDIATE
);

-- Per request results inside one batch. It holds references and outcomes, never
-- the request content.
CREATE TABLE parcel_shipment.submission_batch_request (
    tenant_id           text NOT NULL,
    customer_account_id text NOT NULL,
    submission_batch_id text NOT NULL,
    shipment_request_id text NOT NULL,
    outcome             text NOT NULL,
    reason              text,

    CONSTRAINT submission_batch_request_pkey
        PRIMARY KEY (tenant_id, customer_account_id, submission_batch_id, shipment_request_id),
    CONSTRAINT submission_batch_request_outcome_implemented
        CHECK (outcome IN ('SUBMITTED', 'NOT_ADMITTED')),
    CONSTRAINT submission_batch_request_batch_fkey
        FOREIGN KEY (tenant_id, customer_account_id, submission_batch_id)
        REFERENCES parcel_shipment.submission_batch
            (tenant_id, customer_account_id, submission_batch_id)
);
---- create above / drop below ----

DROP TABLE parcel_shipment.submission_batch_request;
DROP TABLE parcel_shipment.declared_parcel;
DROP TABLE parcel_shipment.shipment_request;
DROP TABLE parcel_shipment.submission_batch;
DROP TABLE parcel_shipment.source_submission;
