package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
)

const (
	defaultAddress  = ":8080"
	shutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("Parcel API stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	address := os.Getenv("IDP_PARCEL_HTTP_ADDR")
	if address == "" {
		address = defaultAddress
	}

	// 隔离读面准入（ADR-0078）在开池之前解析：门禁不合规要在启动最早处带原因退出。
	// 放行必须出声（Decision 三）——启用与所用合成租户写进启动日志，事后可查。
	isolatedRead, err := buildIsolatedReadIntakes(os.Getenv)
	if err != nil {
		return err
	}
	if isolatedRead != nil {
		logger.Info("Isolated read admission enabled (ADR-0078): operations query endpoints answer with injected synthetic scope",
			"tenant", os.Getenv(isolatedReadTenantEnv))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 开池在起服务之前：部署坏了（DSN 缺失、库不可达、缺框架 schema）要让进程带原因
	// 退出，而不是先挂上监听端口再让每个请求各报一次错。ctx 由停机信号驱动，启动途中
	// 收到 SIGTERM 就当场停下。
	db, closeDB, err := openDatabase(ctx, os.Getenv)
	if err != nil {
		return err
	}
	defer closeDB()

	submission, err := buildSubmissionOrchestration(db)
	if err != nil {
		return err
	}
	withdrawal, err := buildWithdrawalOrchestration(db)
	if err != nil {
		return err
	}
	requestViews, err := pspostgres.NewShipmentRequestViews(db)
	if err != nil {
		return err
	}
	cancellation, err := buildCancellationOrchestration(db)
	if err != nil {
		return err
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		return err
	}
	delivery, err := buildDeliveryOrchestration(db)
	if err != nil {
		return err
	}
	trackingViews, err := vepostgres.NewCustomerViews(db)
	if err != nil {
		return err
	}
	// 运营追踪查阅读的就是投影库本身（ADR-0076）：同一适配器同时是派生编排的
	// ProjectionStore 与查阅端点的读面，不造第二份数据。
	projectionViews, err := vepostgres.NewProjections(db)
	if err != nil {
		return err
	}
	claims, err := buildClaimsOrchestration(db)
	if err != nil {
		return err
	}
	results, err := buildExternalResultsOrchestration(db)
	if err != nil {
		return err
	}

	// 主数据目录查阅直接接所属上下文的存储读面（ADR-0077），不绕进应用编排。
	// 同一上下文的多个端点共享同一只读适配器，读的仍是各登记写口背后的那份库。
	pricingCatalog, err := pppostgres.NewOperationsCatalogue(db)
	if err != nil {
		return err
	}
	networkCatalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		return err
	}
	complianceRules, err := ccpostgres.NewRuleCatalogue(db)
	if err != nil {
		return err
	}
	caseRegisters, err := ccpostgres.NewCaseRegisterCatalogue(db)
	if err != nil {
		return err
	}
	gateConditions, err := ccpostgres.NewGateConditionCatalogue(db)
	if err != nil {
		return err
	}
	commercialCatalog, err := pcpostgres.NewOperationsCatalogue(db)
	if err != nil {
		return err
	}
	visibilityCatalogues, err := vepostgres.NewOperationsCatalogue(db)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr: address,
		Handler: httpapi.NewWithEndpoints(buildinfo.Current(), assembleBusinessEndpoints(
			submission,
			withdrawal,
			requestViews,
			cancellation,
			reception,
			delivery,
			trackingViews,
			projectionViews,
			claims,
			results,
			pricingCatalog,
			pricingCatalog,
			networkCatalog,
			complianceRules,
			caseRegisters,
			gateConditions,
			commercialCatalog,
			commercialCatalog,
			commercialCatalog,
			visibilityCatalogues,
			isolatedRead,
		)),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveResult := make(chan error, 1)
	go func() {
		logger.Info("Parcel API listening", "address", address)
		serveResult <- server.ListenAndServe()
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveResult; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
