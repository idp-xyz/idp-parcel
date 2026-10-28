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

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
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
		logger.Info("Isolated read admission enabled (ADR-0078): shipment request views answer with injected synthetic scope",
			"tenant", os.Getenv(isolatedReadTenantEnv))
	}

	// 隔离写路径准入（ADR-0091）与读面同处最早：它也带一道启动即拒的前缀门禁，且
	// 多一条两开关一致性校验。放行同样必须出声——写路径会落行，事后要查得出这些行
	// 是在哪一次启动、以哪个合成租户写下的。
	isolatedWrite, err := buildIsolatedWriteAdmission(os.Getenv)
	if err != nil {
		return err
	}
	if isolatedWrite != nil {
		// 放行什么也要出声（ADR-0078 决定三、ADR-0091 决定四）：写开关只剩委托受理的归属坐标，且只给开关租户的拟受理范围，
		// 不放行任何命令面——提交口走首方客户渠道，要带首方客户端的令牌（ADR-0150 决定三）。
		logger.Info("Isolated write admission enabled (ADR-0091): production ownership resolves against the governance register with injected synthetic coordinates for the switch tenant's admission scopes only; no command line is admitted, submissions go through the first-party customer channel",
			"tenant", isolatedWrite.tenant,
			"selfAuthority", isolatedWrite.selfAuthority)
	}

	// 本产品在治理登记册里的权威串（ADR-0180 决定三）与两道隔离开关同处最早：隔离写开关开着而两值不等，上面那道门已经
	// 带原因退出了。配没配都出声——没配时运营决定与外部事实各口答 OUTSIDE_ADMISSION_SCOPE，与「配了但租户的对照或区间
	// 没登」在页面上一样进不去，启动日志是分辨两者的第一处。
	selfAuthority := governanceSelfAuthority(os.Getenv)
	if selfAuthority != "" {
		logger.Info("Governance self authority configured (ADR-0180): admission scope reads tenant mappings and this product's authority intervals",
			"selfAuthority", selfAuthority)
	} else {
		logger.Info("Governance self authority not configured (ADR-0180): production writes through the operator and integration client channels answer OUTSIDE_ADMISSION_SCOPE")
	}

	// 操作者渠道的信任锚（ADR-0100 决定二）与两道隔离开关同处最早：参数只设了一半要在开池之前
	// 带原因退出。配没配都出声——操作者族未配置时整族答 403，而那与「配了但授予没登」在页面上同样
	// 进不去，启动日志是分辨两者的第一处。核验方交给运营决定口的操作者铸造器（票 operator-channel/15），
	// 在开池之后连同操作者册一起装。
	operatorVerifier, operatorChannelConfigured, err := buildOperatorCredentialVerifier(os.Getenv)
	if err != nil {
		return err
	}
	if operatorChannelConfigured {
		logger.Info("Operator channel issuer configured (ADR-0100): operator tokens verified against the issuer's JWKS",
			"issuer", os.Getenv(operatorIssuerEnv),
			"jwksURL", os.Getenv(operatorJWKSURLEnv),
			"audience", os.Getenv(operatorAudienceEnv))
	} else {
		logger.Info("Operator channel not configured (ADR-0100): issuer parameters unset, operator family answers ACCESS_CHANNEL_NOT_CONFIGURED")
	}

	// 集成客户端族的信任锚与操作者族同处开池之前：参数只设了一半要带原因退出，不先连上库再发现发行方配错。
	integrationClientVerifier, integrationClientChannelConfigured, err := buildIntegrationClientCredentialVerifier(os.Getenv)
	if err != nil {
		return err
	}
	if integrationClientChannelConfigured {
		logger.Info("Integration client channel issuer configured (ADR-0149): client tokens verified against the issuer's JWKS",
			"issuer", os.Getenv(integrationClientIssuerEnv),
			"jwksURL", os.Getenv(integrationClientJWKSURLEnv),
			"audience", os.Getenv(integrationClientAudienceEnv))
	} else {
		logger.Info("Integration client channel not configured (ADR-0149): issuer parameters unset, integration client family answers ACCESS_CHANNEL_NOT_CONFIGURED")
	}

	// 首方客户族的信任锚（ADR-0139 决定二）与操作者族、集成客户端族同处开池之前：参数只设一部分、或受众撞了另一族，都要
	// 带原因退出。
	customerClientVerifier, customerClientChannelConfigured, err := buildCustomerClientCredentialVerifier(os.Getenv)
	if err != nil {
		return err
	}
	if customerClientChannelConfigured {
		logger.Info("First-party customer channel issuer configured (ADR-0139): customer client tokens verified against the issuer's JWKS",
			"issuer", os.Getenv(customerAPIIssuerEnv),
			"jwksURL", os.Getenv(customerAPIJWKSURLEnv),
			"audience", os.Getenv(customerAPIAudienceEnv))
	} else {
		logger.Info("First-party customer channel not configured (ADR-0139): issuer parameters unset, customer-facing lines on the first-party channel answer ACCESS_CHANNEL_NOT_CONFIGURED")
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

	endpoints, err := assembleProductionEndpoints(db, startupChannels{
		isolatedRead:              isolatedRead,
		isolatedWrite:             isolatedWrite,
		selfAuthority:             selfAuthority,
		operatorVerifier:          operatorVerifier,
		integrationClientVerifier: integrationClientVerifier,
		customerClientVerifier:    customerClientVerifier,
	})
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.NewWithEndpoints(buildinfo.Current(), endpoints),
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

// startupChannels 是开池之前就解析好的部署参数：两道隔离开关、本产品权威串、三族核验方。它们在 run 里带启动日志解析，
// 参数不合规时在开池之前就带原因退出。
type startupChannels struct {
	isolatedRead              *isolatedReadIntakes
	isolatedWrite             *isolatedWriteAdmission
	selfAuthority             string
	operatorVerifier          accessidentity.OperatorCredentialVerifier
	integrationClientVerifier accessidentity.IntegrationClientCredentialVerifier
	customerClientVerifier    accessidentity.CustomerClientCredentialVerifier
}

// assembleProductionEndpoints 是开池之后的那一段生产装配：先建各族接入渠道，再装只为启动即查的几只编排，最后展开端点
// 登记处（ADR-0222）。任一步失败原样交回，run 带着原因退出——部署坏了不先挂上监听端口。它与 run 分开，是为了让生产装配
// 能对着真库整体验一遍。
func assembleProductionEndpoints(db *bentopg.DB, startup startupChannels) ([]httpapi.BusinessEndpoint, error) {
	// 运营决定口的操作者 Intake（ADR-0151；票 operator-channel/15）：委托侧的寻址读口就是委托查阅适配器
	// （ports.OperatorDecisionTargets 在其上补齐，同表、整租户可见）。受控关闭与重开按包裹寻址，读口是委托仓储上
	// 那只按包裹反查当前已接受委托的口（ADR-0183 决定一）。
	requestViews, err := pspostgres.NewShipmentRequestViews(db)
	if err != nil {
		return nil, err
	}
	parcelTargets, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, err
	}
	operatorRegistry, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		return nil, err
	}
	// 准入范围操作者与集成客户端两族共用一只（ADR-0180）：权威串没设时是未配置那一只，对照与区间一行都不读。
	admission, err := buildAdmissionScope(db, startup.selfAuthority)
	if err != nil {
		return nil, err
	}
	operatorMinter, err := buildOperatorMinter(startup.operatorVerifier, operatorRegistry, admission)
	if err != nil {
		return nil, err
	}
	deviceRegistry, err := aipg.NewDeviceRegistry(db)
	if err != nil {
		return nil, err
	}
	workFactReception, workFactTransport, err := buildWorkFactIntakes(startup.operatorVerifier, operatorRegistry, deviceRegistry, admission)
	if err != nil {
		return nil, err
	}
	operatorDecisions, err := buildOperatorDecisionIntakes(operatorMinter, requestViews, parcelTargets)
	if err != nil {
		return nil, err
	}
	operatorRegistries, err := buildOperatorRegistryIntakes(operatorMinter)
	if err != nil {
		return nil, err
	}
	integrationClientRegistry, err := aipg.NewIntegrationClientRegistry(db)
	if err != nil {
		return nil, err
	}
	integrationClientMinter, err := buildIntegrationClientMinter(startup.integrationClientVerifier, integrationClientRegistry, admission)
	if err != nil {
		return nil, err
	}
	integrationClients, err := buildIntegrationClientIntakes(integrationClientMinter)
	if err != nil {
		return nil, err
	}
	// 提交口的首方 Intake（ADR-0139 决定四；票 first-party-api/00）：预取期望修订用的归属权威与提交编排同一组坐标。
	customerClientRegistry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		return nil, err
	}
	customerClientMinter, err := buildCustomerClientMinter(startup.customerClientVerifier, customerClientRegistry, admission)
	if err != nil {
		return nil, err
	}
	submissionOwnership, err := buildProductionOwnership(db, startup.isolatedWrite, startup.selfAuthority)
	if err != nil {
		return nil, err
	}
	customerSubmission, err := buildCustomerSubmissionIntake(customerClientMinter, submissionOwnership)
	if err != nil {
		return nil, err
	}

	// 面单渠道写链（票 label-channel/34）：启动时装配**只为 fail-fast**——组合根里 PC / PS 的读适配器与 outbox
	// 任一构造失败，进程在此带原因退出（同 run 开池段那条纪律），而不是等到某天接上触发面才发现。产物今天没有
	// 消费者（乙路无运营端点、无进程内触发面；谁在什么业务时点发起一笔面单交易的建立是产品题，触发面另票），
	// 构造即丢；实例半边缝全部显式未配置，接受时解析回指装接受决定读口，不为此变绿种任何行。
	if _, err := buildLabelChannelOrchestration(db); err != nil {
		return nil, err
	}
	// 请求评价编排（票 sa-cc/08）：启动时装配只为 fail-fast。触发面是编排上的 Trigger：发生项原因没登记答未配置，
	// 不发起请求。构造结果不挂端点；三件来源引用仍由调用方交进来，不为此种任何行。
	if _, err := buildEvaluationRequestOrchestration(db); err != nil {
		return nil, err
	}
	return assembleBusinessEndpoints(endpointSources{
		db:                   db,
		isolatedWrite:        startup.isolatedWrite,
		selfAuthority:        startup.selfAuthority,
		operatorVerifier:     startup.operatorVerifier,
		operatorRegistry:     operatorRegistry,
		deviceRegistry:       deviceRegistry,
		admission:            admission,
		operatorMinter:       operatorMinter,
		customerClientMinter: customerClientMinter,
	}, endpointChannels{
		isolatedRead:       startup.isolatedRead,
		customerSubmission: customerSubmission,
		operatorDecisions:  operatorDecisions,
		operatorRegistries: operatorRegistries,
		integrationClients: integrationClients,
		workFactReception:  workFactReception,
		workFactTransport:  workFactTransport,
	})
}
