-- 参与方关系的终止事实 CHECK 跟上领域的两处收紧（票 ps-external-mark-relations/17：关系的撤销、到期、替代有了写口）。
--
-- 一、到期也必带终止依据。domain.PartyRelationship.Expire 自本票起收依据：登记到期的人凭什么认定它到了期，事后要辩护的
--     是这一笔，与撤销同理。0015 的 end_coherent 只要求撤销带依据，这里把到期并进去。
-- 二、替代的接替方不能是原持有方，也不能是相对方（domain.PartyRelationship.SupersededBy）：前者不是接替，后者接替之后要
--     对自己持有这个角色。
--
-- 0015 起的那一版已施加、按校验和守着，不改 0015，收紧只能是一份新迁移（判据同 0039）。本票之前没有任何写口落过终止
-- 修订，册上没有会被新 CHECK 拒的既有行。

ALTER TABLE party_commercial.party_relationship_registration
    DROP CONSTRAINT party_relationship_registration_end_coherent;

ALTER TABLE party_commercial.party_relationship_registration
    ADD CONSTRAINT party_relationship_registration_end_coherent
        CHECK (
            (ended_at IS NULL AND end_basis IS NULL AND successor_party_id IS NULL
                AND status IN ('CANDIDATE', 'EFFECTIVE'))
            OR (ended_at IS NOT NULL AND status IN ('EXPIRED', 'REVOKED', 'SUPERSEDED')
                AND (status NOT IN ('EXPIRED', 'REVOKED') OR (end_basis IS NOT NULL AND btrim(end_basis) <> ''))
                AND (status <> 'SUPERSEDED'
                    OR (successor_party_id IS NOT NULL AND btrim(successor_party_id) <> ''
                        AND successor_party_id <> holder_party_id
                        AND successor_party_id <> counterparty_party_id)))
        );
