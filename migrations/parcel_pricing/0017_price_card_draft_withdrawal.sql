-- 价卡草稿加终态 WITHDRAWN（已撤下）。只从 APPROVED 进入，不回填旧行。
--
-- 0011 的文件一个字节都不改。状态与批准痕迹两条 CHECK 在这里拆掉再扩成含 WITHDRAWN：已撤下仍带着批准者与
-- 批准时刻。发布时刻那条 CHECK 不用动——它已经要求非 PUBLISHED 的行 published_at 为空，已撤下就在这一支里。
-- 新列可空，且只在 WITHDRAWN 在场；withdrawn_at 不早于 approved_at。迁移之前已在册的行三列保持空，读回不变。

ALTER TABLE parcel_pricing.price_card_draft
    ADD COLUMN withdrawn_by text,
    ADD COLUMN withdrawn_at timestamptz,
    ADD COLUMN withdrawal_reason text;

ALTER TABLE parcel_pricing.price_card_draft
    DROP CONSTRAINT price_card_draft_status_closed,
    ADD CONSTRAINT price_card_draft_status_closed
        CHECK (status IN ('DRAFT', 'VALIDATED', 'APPROVED', 'PUBLISHED', 'WITHDRAWN'));

ALTER TABLE parcel_pricing.price_card_draft
    DROP CONSTRAINT price_card_draft_approval_traces,
    ADD CONSTRAINT price_card_draft_approval_traces
        CHECK (
            (status IN ('DRAFT', 'VALIDATED') AND approver IS NULL AND approved_at IS NULL)
            OR (status IN ('APPROVED', 'PUBLISHED', 'WITHDRAWN')
                AND approver IS NOT NULL AND btrim(approver) <> ''
                AND approved_at IS NOT NULL AND approved_at >= submitted_at)
        );

ALTER TABLE parcel_pricing.price_card_draft
    ADD CONSTRAINT price_card_draft_withdrawal_traces
        CHECK (
            (status <> 'WITHDRAWN'
                AND withdrawn_by IS NULL
                AND withdrawn_at IS NULL
                AND withdrawal_reason IS NULL)
            OR (status = 'WITHDRAWN'
                AND withdrawn_by IS NOT NULL AND btrim(withdrawn_by) <> ''
                AND withdrawn_at IS NOT NULL AND withdrawn_at >= approved_at
                AND withdrawal_reason IS NOT NULL AND btrim(withdrawal_reason) <> ''
                AND withdrawal_reason !~ '[[:cntrl:]]')
        );
