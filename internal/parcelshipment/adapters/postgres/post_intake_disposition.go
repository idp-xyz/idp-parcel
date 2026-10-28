package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// PostIntakeDispositions 实现 ports.PostIntakeDispositions：收寄后服务处置决定的留痕（ADR-0332）。每个包裹一条线性的决定链，只追加，当前决定是链尾；
// 主键（租户，包裹，版本），租户条件进每条语句。
type PostIntakeDispositions struct {
	db *bentopg.DB
}

func NewPostIntakeDispositions(db *bentopg.DB) (*PostIntakeDispositions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &PostIntakeDispositions{db: db}, nil
}

var _ ports.PostIntakeDispositions = (*PostIntakeDispositions)(nil)

const postIntakeDispositionColumns = `tenant_id, parcel_id, version, supersedes_version, direction, source,
		        request_key, destination_ref, responsibility_start, decided_by, decision_basis, decided_at`

// AppendPostIntakeDisposition 追加一份决定。要环境事务：决定留痕与信封同笔落地。
//
// 撞任何唯一约束（版本被占、同一份请求已被回应）都答 `版本已被占` 而不报错：编排已在包裹取消边界锁里核对过链尾与请求，锁内撞上说明有锁之外的写入，
// 由编排重读后按「前一份已变」或「请求已被回应」作答。
func (dispositions *PostIntakeDispositions) AppendPostIntakeDisposition(
	ctx context.Context,
	disposition domain.PostIntakeDisposition,
) (ports.PostIntakeDispositionAppendOutcome, error) {
	executor, err := dispositions.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PostIntakeDispositionAppendOutcomeInvalid, fmt.Errorf("append post-intake disposition: %w", err)
	}
	var supersedes *int
	if disposition.Supersedes() > 0 {
		previous := disposition.Supersedes()
		supersedes = &previous
	}
	var requestKey, destination *string
	if disposition.RequestKey().String() != "" {
		raw := disposition.RequestKey().String()
		requestKey = &raw
	}
	if disposition.Destination().String() != "" {
		raw := disposition.Destination().String()
		destination = &raw
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.post_intake_disposition_decision
			(tenant_id, parcel_id, version, supersedes_version, direction, source,
			 request_key, destination_ref, responsibility_start, decided_by, decision_basis, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		disposition.Tenant().String(), disposition.Parcel().String(), disposition.Version(), supersedes,
		disposition.Direction().String(), disposition.Source().String(),
		requestKey, destination, disposition.ResponsibilityStart().String(),
		disposition.Decider().String(), disposition.Basis().String(), disposition.DecidedAt().UTC(),
	)
	if err != nil {
		return ports.PostIntakeDispositionAppendOutcomeInvalid, fmt.Errorf("append post-intake disposition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.PostIntakeDispositionVersionTaken, nil
	}
	return ports.PostIntakeDispositionAppended, nil
}

func (dispositions *PostIntakeDispositions) FindCurrentPostIntakeDisposition(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.PostIntakeDisposition, bool, error) {
	querier, err := dispositions.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PostIntakeDisposition{}, false, fmt.Errorf("find current post-intake disposition: %w", err)
	}
	return scanOptionalDisposition(querier.QueryRow(ctx,
		`SELECT `+postIntakeDispositionColumns+`
		   FROM parcel_shipment.post_intake_disposition_decision
		  WHERE tenant_id = $1 AND parcel_id = $2
		  ORDER BY version DESC
		  LIMIT 1`,
		tenant.String(), parcel.String()), "find current post-intake disposition")
}

func (dispositions *PostIntakeDispositions) FindPostIntakeDispositionAt(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
	version int,
) (domain.PostIntakeDisposition, bool, error) {
	querier, err := dispositions.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PostIntakeDisposition{}, false, fmt.Errorf("find post-intake disposition: %w", err)
	}
	return scanOptionalDisposition(querier.QueryRow(ctx,
		`SELECT `+postIntakeDispositionColumns+`
		   FROM parcel_shipment.post_intake_disposition_decision
		  WHERE tenant_id = $1 AND parcel_id = $2 AND version = $3`,
		tenant.String(), parcel.String(), version), "find post-intake disposition")
}

func (dispositions *PostIntakeDispositions) FindPostIntakeDispositionAnswering(
	ctx context.Context,
	key ports.CancellationRequestKey,
) (domain.PostIntakeDisposition, bool, error) {
	querier, err := dispositions.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PostIntakeDisposition{}, false, fmt.Errorf("find answering post-intake disposition: %w", err)
	}
	return scanOptionalDisposition(querier.QueryRow(ctx,
		`SELECT `+postIntakeDispositionColumns+`
		   FROM parcel_shipment.post_intake_disposition_decision
		  WHERE tenant_id = $1 AND request_key = $2 AND parcel_id = $3`,
		key.TenantID.String(), key.RequestKey.String(), key.Parcel.String()), "find answering post-intake disposition")
}

func (dispositions *PostIntakeDispositions) ListPostIntakeDispositionChain(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) ([]domain.PostIntakeDisposition, error) {
	querier, err := dispositions.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list post-intake disposition chain: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+postIntakeDispositionColumns+`
		   FROM parcel_shipment.post_intake_disposition_decision
		  WHERE tenant_id = $1 AND parcel_id = $2
		  ORDER BY version`,
		tenant.String(), parcel.String())
	if err != nil {
		return nil, fmt.Errorf("list post-intake disposition chain: %w", err)
	}
	defer rows.Close()
	chain := []domain.PostIntakeDisposition{}
	for rows.Next() {
		disposition, err := scanDisposition(rows)
		if err != nil {
			return nil, fmt.Errorf("list post-intake disposition chain: %w", err)
		}
		chain = append(chain, disposition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list post-intake disposition chain: %w", err)
	}
	return chain, nil
}

type dispositionScanner interface {
	Scan(dest ...any) error
}

func scanOptionalDisposition(row pgx.Row, operation string) (domain.PostIntakeDisposition, bool, error) {
	disposition, err := scanDisposition(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PostIntakeDisposition{}, false, nil
	}
	if err != nil {
		return domain.PostIntakeDisposition{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return disposition, true, nil
}

// scanDisposition 读一行，经领域构造器重走决定门：库里的行是已通过构造器的决定，读回再走一遍是为了让读不回的坏行响亮失败，不悄悄交出半截决定。
func scanDisposition(row dispositionScanner) (domain.PostIntakeDisposition, error) {
	var (
		tenantRaw, parcelRaw, directionRaw, sourceRaw string
		startRaw, deciderRaw, basisRaw                string
		version                                       int
		supersedes                                    *int
		requestKey, destination                       *string
		decidedAt                                     time.Time
	)
	if err := row.Scan(&tenantRaw, &parcelRaw, &version, &supersedes, &directionRaw, &sourceRaw,
		&requestKey, &destination, &startRaw, &deciderRaw, &basisRaw, &decidedAt); err != nil {
		return domain.PostIntakeDisposition{}, err
	}
	spec := domain.PostIntakeDispositionSpec{Version: version, DecidedAt: decidedAt.UTC()}
	if supersedes != nil {
		spec.Supersedes = *supersedes
	}
	var err error
	if spec.Tenant, err = domain.NewTenantID(tenantRaw); err != nil {
		return domain.PostIntakeDisposition{}, err
	}
	if spec.Parcel, err = domain.NewDeclaredParcelID(parcelRaw); err != nil {
		return domain.PostIntakeDisposition{}, err
	}
	if spec.Direction, err = domain.ParsePostIntakeDispositionDirection(directionRaw); err != nil {
		return domain.PostIntakeDisposition{}, err
	}
	if spec.Source, err = domain.ParsePostIntakeDispositionSource(sourceRaw); err != nil {
		return domain.PostIntakeDisposition{}, err
	}
	if requestKey != nil {
		if spec.RequestKey, err = domain.NewSourceRequestKey(*requestKey); err != nil {
			return domain.PostIntakeDisposition{}, err
		}
	}
	if destination != nil {
		if spec.Destination, err = domain.NewPostIntakeDispositionDestination(*destination); err != nil {
			return domain.PostIntakeDisposition{}, err
		}
	}
	if spec.ResponsibilityStart, err = domain.NewSourceResultVersion(startRaw); err != nil {
		return domain.PostIntakeDisposition{}, err
	}
	if spec.Decider, err = domain.NewPostIntakeDispositionDeciderReference(deciderRaw); err != nil {
		return domain.PostIntakeDisposition{}, err
	}
	if spec.Basis, err = domain.NewPostIntakeDispositionBasis(basisRaw); err != nil {
		return domain.PostIntakeDisposition{}, err
	}
	return domain.DecidePostIntakeDisposition(spec)
}

// PostIntakeDispositionRequests 实现 ports.PostIntakeDispositionRequests：等收寄后服务处置决定回应的客户停止请求（ADR-0332 决定一）。请求有两个来路——
// 越界待处置的取消记录，以及被取消先后冲突的解开决定认定依据不成立的取消（ADR-0295）；「已被回应」是有一份决定的 request_key 指名它，不是请求记录上的状态列。
type PostIntakeDispositionRequests struct {
	db *bentopg.DB
}

func NewPostIntakeDispositionRequests(db *bentopg.DB) (*PostIntakeDispositionRequests, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &PostIntakeDispositionRequests{db: db}, nil
}

var _ ports.PostIntakeDispositionRequests = (*PostIntakeDispositionRequests)(nil)

// postIntakeRequestSource 是两个来路的并集，租户由 $1 切。被认定不成立的取消从决定表出发取不重复的（包裹，取消）：一份取消名下可以有多份冲突、各自被决定，
// 不去重会让同一请求在清单里出现几次。
const postIntakeRequestSource = `
	SELECT c.tenant_id, c.request_key, c.parcel_id, c.kind, c.crossed_intake_version, c.cancellation_id,
	       c.requester_ref, c.authority_ref, c.reason_ref, c.requested_at, c.decided_at
	  FROM parcel_shipment.parcel_cancellation c
	 WHERE c.tenant_id = $1 AND c.kind = 'DISPOSITION_PENDING'
	UNION ALL
	SELECT c.tenant_id, c.request_key, c.parcel_id, c.kind, c.crossed_intake_version, c.cancellation_id,
	       c.requester_ref, c.authority_ref, c.reason_ref, c.requested_at, c.decided_at
	  FROM (SELECT DISTINCT tenant_id, parcel_id, cancellation_id
	          FROM parcel_shipment.cancellation_order_conflict_decision
	         WHERE tenant_id = $1 AND decision_kind = 'CANCELLATION_NOT_UPHELD') n
	  JOIN parcel_shipment.parcel_cancellation c
	    ON c.tenant_id = n.tenant_id AND c.parcel_id = n.parcel_id
	   AND c.cancellation_id = n.cancellation_id AND c.kind = 'PARCEL_CANCELLED'`

const unansweredPostIntakeRequestFilter = `
	 WHERE NOT EXISTS (
	           SELECT 1 FROM parcel_shipment.post_intake_disposition_decision a
	            WHERE a.tenant_id = r.tenant_id AND a.request_key = r.request_key AND a.parcel_id = r.parcel_id)
	   AND NOT EXISTS (
	           SELECT 1 FROM parcel_shipment.final_outcome f
	            WHERE f.tenant_id = r.tenant_id AND f.parcel_id = r.parcel_id AND f.is_current)`

const postIntakeCurrentVersion = `COALESCE((SELECT max(d.version)
	                          FROM parcel_shipment.post_intake_disposition_decision d
	                         WHERE d.tenant_id = r.tenant_id AND d.parcel_id = r.parcel_id), 0)`

func (requests *PostIntakeDispositionRequests) FindPostIntakeDispositionRequest(
	ctx context.Context,
	key ports.CancellationRequestKey,
) (ports.PostIntakeDispositionRequest, bool, error) {
	querier, err := requests.db.ReadExecutor(ctx)
	if err != nil {
		return ports.PostIntakeDispositionRequest{}, false, fmt.Errorf("find post-intake disposition request: %w", err)
	}
	row := querier.QueryRow(ctx,
		`WITH r AS (`+postIntakeRequestSource+`)
		 SELECT r.request_key, r.parcel_id, r.kind, r.crossed_intake_version, r.cancellation_id,
		        r.requester_ref, r.authority_ref, r.reason_ref, r.requested_at, r.decided_at,
		        `+postIntakeCurrentVersion+`
		   FROM r
		  WHERE r.request_key = $2 AND r.parcel_id = $3`,
		key.TenantID.String(), key.RequestKey.String(), key.Parcel.String())
	request, err := scanPostIntakeRequest(key.TenantID, row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PostIntakeDispositionRequest{}, false, nil
	}
	if err != nil {
		return ports.PostIntakeDispositionRequest{}, false, fmt.Errorf("find post-intake disposition request: %w", err)
	}
	return request, true, nil
}

// ListUnansweredPostIntakeDispositionRequests 上列本租户还没有被回应的请求，最早记下的在前、同刻按请求键与包裹收尾，至多 limit 行；Count 是全部行数，不受
// limit 截断。两句不在同一快照里：计数与清单是此刻的大概，决定的成败由决定口自己在锁里判。
func (requests *PostIntakeDispositionRequests) ListUnansweredPostIntakeDispositionRequests(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) (ports.UnansweredPostIntakeDispositionRequests, error) {
	if limit <= 0 {
		return ports.UnansweredPostIntakeDispositionRequests{}, fmt.Errorf(
			"list unanswered post-intake disposition requests: limit %d is not a positive page size", limit)
	}
	querier, err := requests.db.ReadExecutor(ctx)
	if err != nil {
		return ports.UnansweredPostIntakeDispositionRequests{}, fmt.Errorf("list unanswered post-intake disposition requests: %w", err)
	}
	var count int
	if err := querier.QueryRow(ctx,
		`WITH r AS (`+postIntakeRequestSource+`) SELECT count(*) FROM r`+unansweredPostIntakeRequestFilter,
		tenant.String()).Scan(&count); err != nil {
		return ports.UnansweredPostIntakeDispositionRequests{}, fmt.Errorf("list unanswered post-intake disposition requests: count: %w", err)
	}
	rows, err := querier.Query(ctx,
		`WITH r AS (`+postIntakeRequestSource+`)
		 SELECT r.request_key, r.parcel_id, r.kind, r.crossed_intake_version, r.cancellation_id,
		        r.requester_ref, r.authority_ref, r.reason_ref, r.requested_at, r.decided_at,
		        `+postIntakeCurrentVersion+`
		   FROM r`+unansweredPostIntakeRequestFilter+`
		  ORDER BY r.decided_at, r.request_key, r.parcel_id
		  LIMIT $2`,
		tenant.String(), limit)
	if err != nil {
		return ports.UnansweredPostIntakeDispositionRequests{}, fmt.Errorf("list unanswered post-intake disposition requests: %w", err)
	}
	defer rows.Close()

	listed := ports.UnansweredPostIntakeDispositionRequests{Requests: []ports.PostIntakeDispositionRequest{}, Count: count}
	for rows.Next() {
		request, err := scanPostIntakeRequest(tenant, rows)
		if err != nil {
			return ports.UnansweredPostIntakeDispositionRequests{}, fmt.Errorf("list unanswered post-intake disposition requests: %w", err)
		}
		listed.Requests = append(listed.Requests, request)
	}
	if err := rows.Err(); err != nil {
		return ports.UnansweredPostIntakeDispositionRequests{}, fmt.Errorf("list unanswered post-intake disposition requests: %w", err)
	}
	return listed, nil
}

func scanPostIntakeRequest(tenant domain.TenantID, row dispositionScanner) (ports.PostIntakeDispositionRequest, error) {
	var (
		requestKeyRaw, parcelRaw, kindRaw string
		crossed, cancellationID           *string
		requester, authority, reason      *string
		requestedAt                       *time.Time
		recordedAt                        time.Time
		currentVersion                    int
		request                           ports.PostIntakeDispositionRequest
		err                               error
		requestKey                        domain.SourceRequestKey
		parcel                            domain.DeclaredParcelID
	)
	if err = row.Scan(&requestKeyRaw, &parcelRaw, &kindRaw, &crossed, &cancellationID,
		&requester, &authority, &reason, &requestedAt, &recordedAt, &currentVersion); err != nil {
		return ports.PostIntakeDispositionRequest{}, err
	}
	if requestKey, err = domain.NewSourceRequestKey(requestKeyRaw); err != nil {
		return ports.PostIntakeDispositionRequest{}, err
	}
	if parcel, err = domain.NewDeclaredParcelID(parcelRaw); err != nil {
		return ports.PostIntakeDispositionRequest{}, err
	}
	request.Key = ports.CancellationRequestKey{TenantID: tenant, RequestKey: requestKey, Parcel: parcel}
	request.RecordedAt = recordedAt.UTC()
	request.CurrentVersion = currentVersion
	switch kindRaw {
	case ports.RecordDispositionPending.String():
		request.Origin = ports.PostIntakeRequestBoundaryCrossed
		if crossed == nil {
			return ports.PostIntakeDispositionRequest{}, errors.New("a pending row arrived without the crossed intake version")
		}
		if request.IntakeBefore, err = domain.NewSourceResultVersion(*crossed); err != nil {
			return ports.PostIntakeDispositionRequest{}, err
		}
	case ports.RecordParcelCancelled.String():
		request.Origin = ports.PostIntakeRequestCancellationNotUpheld
		if cancellationID == nil {
			return ports.PostIntakeDispositionRequest{}, errors.New("a cancelled row arrived without its cancellation id")
		}
		if request.Cancellation, err = domain.NewParcelCancellationID(*cancellationID); err != nil {
			return ports.PostIntakeDispositionRequest{}, err
		}
	default:
		return ports.PostIntakeDispositionRequest{}, fmt.Errorf("a %s row is not a disposition request", kindRaw)
	}
	if requester != nil && authority != nil && reason != nil && requestedAt != nil {
		stop, err := rebuildStopRequest(cancelledColumns{
			requester: *requester, authority: *authority, reason: *reason, requestedAt: *requestedAt,
		})
		if err != nil {
			return ports.PostIntakeDispositionRequest{}, err
		}
		request.Requester, request.Authority, request.Reason, request.RequestedAt =
			stop.Requester, stop.Authority, stop.Reason, stop.RequestedAt
	}
	return request, nil
}
