package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件在 ShipmentRequestViews 上补齐 ports.AuthorizedDispositionQueue（ADR-0132 决定二，票
// sa-preacceptance-policy-view/04）。放在查阅适配器而不是仓储上，理由与 customer_supplement_queue
// 逐字相同：它是「等授权处置的都有谁」的读面，与另几个队列读的是同一张表、同一套作用域纪律。
//
// 过滤在投影列上（迁移 0009 立列、0021 建部分索引）：谓词 `task_waiting_on = $3 AND state = $4` 与部分索引
// 逐字吻合，数字由 Go 侧常量传参，编号的唯一来源始终是领域包。
//
// 受限项从判断表取（同包 loadFinancialControl，与形成决定那一步读的是同一批行）：每行一次点读。队列
// 行数受 limit 约束，且停在等处置的委托本就少——那是策略正文登了`进入授权处置`的合同才走得到的一格。
var _ ports.AuthorizedDispositionQueue = (*ShipmentRequestViews)(nil)

var authorizedDispositionQueuePages = ports.AuthorizedDispositionQueuePages.MustBind(shipmentRequestQueueColumns)

// ListAwaitingAuthorizedDisposition 按作用域取当前停在`等待授权处置`的委托的一页：缺省老的在前（先停的先处置），同刻按委托
// 标识正序。受限项只对本页的委托点读。limit 非正是调用方编程错误，判据同 ListVisible。
func (views *ShipmentRequestViews) ListAwaitingAuthorizedDisposition(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.AuthorizedDispositionQueueRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.AuthorizedDispositionQueueRecord], error) {
		return cataloguepage.Result[ports.AuthorizedDispositionQueueRecord]{}, fmt.Errorf("list authorized disposition queue: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := authorizedDispositionQueuePages.Plan(query, waitingRequests, []any{
		scope.TenantID().String(), visibleAccounts(scope),
		uint8(domain.ResumeByAuthorizedDisposition), uint8(domain.ShipmentRequestSubmitted),
	}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(shipmentRequestTable), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(summaryColumns+", snapshot->'acceptanceTask'", shipmentRequestTable), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	summaries, sources, err := scanQueueRows(rows)
	if err != nil {
		return fail(err)
	}
	taskViews, err := taskViewRecords(ctx, querier, scope.TenantID(), sources)
	if err != nil {
		return fail(err)
	}
	records := make([]ports.AuthorizedDispositionQueueRecord, 0, len(summaries))
	for index, summary := range summaries {
		taskView := taskViews[index]
		records = append(records, ports.AuthorizedDispositionQueueRecord{
			ShipmentRequestSummaryRecord: summary,
			HasAttempt:                   taskView.HasAttempt,
			LastAttemptReason:            taskView.LastAttemptReason,
			LastAttemptContinuation:      taskView.LastAttemptContinuation,
			LastAttemptedAt:              taskView.LastAttemptedAt,
		})
	}
	page, err := cataloguepage.Finish(query, records, limit, total,
		func(record ports.AuthorizedDispositionQueueRecord, sortField string) cataloguepage.Position {
			return summaryPosition(record.ShipmentRequestSummaryRecord, sortField)
		})
	if err != nil {
		return fail(err)
	}
	// 游标关了再逐行点读控制结果：同一个执行器上不套第二个游标。
	for index := range page.Rows {
		control, err := loadFinancialControl(ctx, querier,
			scope.TenantID(), page.Rows[index].ShipmentRequestID, page.Rows[index].SubmissionVersionID)
		if err != nil {
			return fail(err)
		}
		page.Rows[index].ControlResultID = control.ResultID()
		page.Rows[index].RestrictedItems = restrictedItemsOf(control)
	}
	return page, nil
}

// restrictedItemsOf 只取受限项——成立项没有去向可看。处置与责任引用照采用引用透出，没采用的项两格为零值：
// 一份停在等处置的委托不该有那样的项（进这一格的前提是全部受限项都采用了`进入授权处置`），读到即如实透出，
// 不补造。
func restrictedItemsOf(control domain.FinancialControlResult) []ports.RestrictedControlItemRecord {
	var restricted []ports.RestrictedControlItemRecord
	for _, item := range control.Items() {
		if item.Satisfied() {
			continue
		}
		record := ports.RestrictedControlItemRecord{
			Kind:  item.Kind(),
			Order: item.Order(),
			Basis: item.Basis(),
		}
		if adopted, present := item.AdoptedDisposition(); present {
			record.FailureDisposition = adopted.FailureDisposition()
			record.Responsibility = adopted.Responsibility()
		}
		restricted = append(restricted, record)
	}
	return restricted
}
