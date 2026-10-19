package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// AuthorizedDeliveryJudgments 实现 ports.AuthorizedDeliveryJudgmentView：从客户账户册与参与方关系册（0015、0039 迁移）取齐
// 判断要的册面，交 domain.JudgeAuthorizedDelivery 判。只读。
type AuthorizedDeliveryJudgments struct {
	db *bentopg.DB
}

func NewAuthorizedDeliveryJudgments(db *bentopg.DB) (*AuthorizedDeliveryJudgments, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &AuthorizedDeliveryJudgments{db: db}, nil
}

var _ ports.AuthorizedDeliveryJudgmentView = (*AuthorizedDeliveryJudgments)(nil)

// JudgeAuthorizedDelivery 用一条语句取三样册面：账户的最新修订、本租户登没登过授权交付方关系、交付方名下每段这种关系的
// 最新修订。一条语句即一个快照：分三条读，读与读之间落进来的一笔撤销会让「登过」与「名下的关系」对不上。
//
// 名下的关系先按「任一修订以这个角色由交付方持有」圈出关系标识，再取每段的最新修订、留下最新修订仍是这个角色由交付方
// 持有的那些：判断只认最新修订（domain.AuthorizedDeliveryEvidence 的 Held 一格），撤销、到期与替代都是下一修订。
func (judgments *AuthorizedDeliveryJudgments) JudgeAuthorizedDelivery(
	ctx context.Context,
	query domain.AuthorizedDeliveryQuery,
) (domain.AuthorizedDeliveryAnswer, error) {
	const operation = "judge authorized delivery"
	none := domain.AuthorizedDeliveryAnswerInvalid
	if err := query.Validate(); err != nil {
		return none, err
	}
	querier, err := judgments.db.ReadExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("%s: %w", operation, err)
	}

	var accountSnapshot, heldSnapshots []byte
	var configured bool
	err = querier.QueryRow(ctx,
		`SELECT
		        (SELECT snapshot
		           FROM party_commercial.customer_account_registration
		          WHERE tenant_id = $1 AND account_id = $2
		          ORDER BY revision DESC
		          LIMIT 1),
		        EXISTS (SELECT 1
		                  FROM party_commercial.party_relationship_registration
		                 WHERE tenant_id = $1 AND role = $4),
		        (SELECT jsonb_agg(latest.snapshot ORDER BY latest.relationship_id)
		           FROM (SELECT DISTINCT ON (relationship_id) relationship_id, role, holder_party_id, snapshot
		                   FROM party_commercial.party_relationship_registration
		                  WHERE tenant_id = $1
		                    AND relationship_id IN (
		                        SELECT relationship_id
		                          FROM party_commercial.party_relationship_registration
		                         WHERE tenant_id = $1 AND role = $4 AND holder_party_id = $3)
		                  ORDER BY relationship_id, revision DESC) AS latest
		          WHERE latest.role = $4 AND latest.holder_party_id = $3)`,
		query.Tenant.String(), query.CustomerAccount.String(), query.DeliveringParty.String(),
		domain.AuthorizedDeliveringPartyRole.String(),
	).Scan(&accountSnapshot, &configured, &heldSnapshots)
	if err != nil {
		return none, fmt.Errorf("%s: %w", operation, err)
	}

	evidence := domain.AuthorizedDeliveryEvidence{TenantConfigured: configured}
	if accountSnapshot != nil {
		registration, err := customerAccountFromSnapshot(accountSnapshot)
		if err != nil {
			return none, fmt.Errorf("%s: customer account: %w", operation, err)
		}
		evidence.Account = registration.Account()
	}
	if heldSnapshots != nil {
		var snapshots []json.RawMessage
		if err := json.Unmarshal(heldSnapshots, &snapshots); err != nil {
			return none, fmt.Errorf("%s: relationships: %w", operation, err)
		}
		for _, raw := range snapshots {
			registration, err := relationshipFromSnapshot(raw)
			if err != nil {
				return none, fmt.Errorf("%s: relationship: %w", operation, err)
			}
			evidence.Held = append(evidence.Held, registration)
		}
	}
	return domain.JudgeAuthorizedDelivery(query, evidence)
}
