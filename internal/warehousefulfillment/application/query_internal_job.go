package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// QueryInternalJobsHandler 是库内作业事实与次品转良品的运营查阅。客户面不调用它。
type QueryInternalJobsHandler struct {
	jobs     ports.InternalJobStore
	restores ports.ConditionRestoreStore
}

func NewQueryInternalJobsHandler(jobs ports.InternalJobStore, restores ports.ConditionRestoreStore) *QueryInternalJobsHandler {
	return &QueryInternalJobsHandler{jobs: jobs, restores: restores}
}

func (handler *QueryInternalJobsHandler) ListInternalJobs(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[domain.RecordedInternalJob], error) {
	page, err := handler.jobs.List(ctx, tenant, limit, query)
	if err != nil {
		return cataloguepage.Result[domain.RecordedInternalJob]{}, fmt.Errorf("list internal jobs: %w", err)
	}
	return page, nil
}

func (handler *QueryInternalJobsHandler) GetInternalJob(
	ctx context.Context,
	tenant domain.TenantID,
	factID domain.InternalJobFactID,
) (domain.RecordedInternalJob, bool, error) {
	job, found, err := handler.jobs.Load(ctx, tenant, factID)
	if err != nil {
		return domain.RecordedInternalJob{}, false, fmt.Errorf("get internal job: %w", err)
	}
	return job, found, nil
}

func (handler *QueryInternalJobsHandler) GetConditionRestore(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ConditionRestoreID,
) (domain.ConditionRestore, bool, error) {
	restore, found, err := handler.restores.LoadDecision(ctx, tenant, id)
	if err != nil {
		return domain.ConditionRestore{}, false, fmt.Errorf("get condition restore: %w", err)
	}
	return restore, found, nil
}
