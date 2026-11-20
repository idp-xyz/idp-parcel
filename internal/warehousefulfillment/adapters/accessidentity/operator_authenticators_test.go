package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	whaccess "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// 租户、操作者与发行方都是隔离合成值，只记 `S`。

const operatorTenant = "SYN-TENANT-01"

var operatorNow = time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)

type operatorVerifier struct{ err error }

func (verifier operatorVerifier) VerifyOperatorCredential(_ context.Context, credential identity.OperatorCredential) (identity.OperatorSubject, error) {
	if verifier.err != nil {
		return identity.OperatorSubject{}, verifier.err
	}
	if credential.Token() == "" {
		return identity.OperatorSubject{}, identity.ErrCredentialRejected
	}
	return identity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-01")
}

type operatorRegister struct {
	faces []identity.CapabilityFace
	err   error
}

func (register operatorRegister) FindOperator(_ context.Context, subject identity.OperatorSubject) (identity.OperatorStanding, bool, error) {
	if register.err != nil {
		return identity.OperatorStanding{}, false, register.err
	}
	binding, err := identity.NewOperatorBinding(subject, operatorTenant, "SYN-BASIS-BINDING")
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(operatorNow.Add(-time.Hour), time.Time{})
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	var grants []identity.RecordedGrant
	for index, face := range register.faces {
		grant, err := identity.NewOperatorGrant(operatorTenant, "SYN-GRANT-"+string(rune('A'+index)), subject, face, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return identity.OperatorStanding{}, false, err
		}
		recorded, err := identity.NewRecordedGrant(grant, nil)
		if err != nil {
			return identity.OperatorStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := identity.NewOperatorStanding(binding, grants)
	return standing, err == nil, err
}

func minterOf(t *testing.T, verifier identity.OperatorCredentialVerifier, register operatorRegister) *identity.OperatorMinter {
	t.Helper()
	// 准入范围是未配置那一只：这两个面若判了准入，下面的用例就会答不在准入范围。
	minter, err := identity.NewOperatorMinter(verifier, register, identity.UnconfiguredAdmissionScope{}, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	return minter
}

func registryWriter(t *testing.T, verifier identity.OperatorCredentialVerifier, register operatorRegister) *whaccess.OperatorRegistryAuthenticator {
	t.Helper()
	built, err := whaccess.NewOperatorRegistryAuthenticator(minterOf(t, verifier, register))
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func operationsReader(t *testing.T, verifier identity.OperatorCredentialVerifier, register operatorRegister) *whaccess.OperatorOperationsReadAuthenticator {
	t.Helper()
	built, err := whaccess.NewOperatorOperationsReadAuthenticator(minterOf(t, verifier, register))
	if err != nil {
		t.Fatal(err)
	}
	return built
}

var (
	writeGrant = operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityRegistryConfigurationWrite}}
	readGrant  = operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityMasterDataAndOperationsRead}}
	bothGrants = operatorRegister{faces: []identity.CapabilityFace{identity.CapabilityRegistryConfigurationWrite, identity.CapabilityMasterDataAndOperationsRead}}
)

var grades = []error{
	warehousehttp.ErrAccessChannelNotConfigured, warehousehttp.ErrOperatorCredentialRejected,
	warehousehttp.ErrOperatorNotGranted, warehousehttp.ErrOutsideAdmissionScope, warehousehttp.ErrIdentityDependencyUnavailable,
}

// Covers: AT-WF-010（仓的部分）——持登记册配置写授予的操作者认证到绑定的租户，登记责任方是（发行方，sub）这一对；不判准入范围：
// 准入范围是未配置那一只，判了就会答不在准入范围。
func TestRegistryWriterAuthenticatesToTheBoundTenantAndTheOperatorAsRegistrant(t *testing.T) {
	operator, err := registryWriter(t, operatorVerifier{}, writeGrant).AuthenticateRegistryWrite(context.Background(), "presented.operator.token")

	if err != nil || operator.Tenant.String() != operatorTenant || operator.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("operator = %+v, err = %v", operator, err)
	}
}

// Covers: AT-WF-010（仓的部分）——发行方未配置、令牌无效、没有登记册配置写授予（包括只持查阅读的操作者）、身份依赖读不动，逐格译成
// warehousehttp 的格，各格只落在自己那一格。
func TestRegistryGradesTranslateIntoWarehouseGrades(t *testing.T) {
	cases := map[string]struct {
		verifier identity.OperatorCredentialVerifier
		register operatorRegister
		token    string
		want     error
	}{
		"issuer parameters unset": {identity.UnconfiguredOperatorCredentialVerifier{}, writeGrant, "t", warehousehttp.ErrAccessChannelNotConfigured},
		"token missing":           {operatorVerifier{}, writeGrant, "", warehousehttp.ErrOperatorCredentialRejected},
		"only a read grant":       {operatorVerifier{}, readGrant, "t", warehousehttp.ErrOperatorNotGranted},
		"no grant at all":         {operatorVerifier{}, operatorRegister{}, "t", warehousehttp.ErrOperatorNotGranted},
		"issuer key set down":     {operatorVerifier{err: identity.ErrCredentialVerifierUnavailable}, writeGrant, "t", warehousehttp.ErrIdentityDependencyUnavailable},
		"operator register down":  {operatorVerifier{}, operatorRegister{err: errors.New("down")}, "t", warehousehttp.ErrIdentityDependencyUnavailable},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := registryWriter(t, testCase.verifier, testCase.register).AuthenticateRegistryWrite(context.Background(), testCase.token)
			assertOnlyGrade(t, err, testCase.want)
		})
	}
}

// Covers: AT-WF-010（仓的部分）——主数据与运营查阅读授予的操作者认证到绑定的租户，不判准入范围。
func TestOperationsReaderAuthenticatesToTheBoundTenantWithoutAnAdmissionCheck(t *testing.T) {
	tenant, err := operationsReader(t, operatorVerifier{}, readGrant).AuthenticateOperationsRead(context.Background(), "presented.operator.token")

	if err != nil || tenant.String() != operatorTenant {
		t.Fatalf("tenant = %q, err = %v", tenant, err)
	}
}

// Covers: AT-WF-010（仓的部分）最后一行——只持登记册配置写、没有查阅授予的操作者去查阅，答未授予；两格是册上平列的授予，持写不等于
// 持读，反过来持读也登记不了。其余各格译法与登记写面一致。
func TestOperationsReadGradesTranslateAndAWriteGrantIsNotAReadGrant(t *testing.T) {
	cases := map[string]struct {
		verifier identity.OperatorCredentialVerifier
		register operatorRegister
		token    string
		want     error
	}{
		"issuer parameters unset": {identity.UnconfiguredOperatorCredentialVerifier{}, readGrant, "t", warehousehttp.ErrAccessChannelNotConfigured},
		"token missing":           {operatorVerifier{}, readGrant, "", warehousehttp.ErrOperatorCredentialRejected},
		"only a write grant":      {operatorVerifier{}, writeGrant, "t", warehousehttp.ErrOperatorNotGranted},
		"issuer key set down":     {operatorVerifier{err: identity.ErrCredentialVerifierUnavailable}, readGrant, "t", warehousehttp.ErrIdentityDependencyUnavailable},
		"operator register down":  {operatorVerifier{}, operatorRegister{err: errors.New("down")}, "t", warehousehttp.ErrIdentityDependencyUnavailable},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := operationsReader(t, testCase.verifier, testCase.register).AuthenticateOperationsRead(context.Background(), testCase.token)
			assertOnlyGrade(t, err, testCase.want)
		})
	}

	if _, err := registryWriter(t, operatorVerifier{}, bothGrants).AuthenticateRegistryWrite(context.Background(), "t"); err != nil {
		t.Fatalf("两格授予齐备的操作者登记：%v", err)
	}
	if _, err := operationsReader(t, operatorVerifier{}, bothGrants).AuthenticateOperationsRead(context.Background(), "t"); err != nil {
		t.Fatalf("两格授予齐备的操作者查阅：%v", err)
	}
}

type brokenMinter struct{}

func (brokenMinter) MintOperator(context.Context, identity.OperatorCredential, identity.OperatorRequest) (identity.OperatorEnvelope, error) {
	return identity.OperatorEnvelope{}, errors.New("minting blew up")
}

// Covers: AT-WF-010（仓的部分）——接入身份能力交回认不得的错，不被硬塞进任何一格：原样交出，由处理器答 INTAKE_FAILED。
func TestAnUnclassifiedMintingFailureIsNotDressedAsAGrade(t *testing.T) {
	writer, err := whaccess.NewOperatorRegistryAuthenticator(brokenMinter{})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := whaccess.NewOperatorOperationsReadAuthenticator(brokenMinter{})
	if err != nil {
		t.Fatal(err)
	}

	_, writeErr := writer.AuthenticateRegistryWrite(context.Background(), "t")
	_, readErr := reader.AuthenticateOperationsRead(context.Background(), "t")

	for name, err := range map[string]error{"write": writeErr, "read": readErr} {
		if err == nil {
			t.Fatalf("%s：want 错", name)
		}
		for _, grade := range grades {
			if errors.Is(err, grade) {
				t.Fatalf("%s：认不得的错被译成了 %v", name, grade)
			}
		}
	}
}

func TestAuthenticatorsRefuseToBeBuiltWithoutAMinter(t *testing.T) {
	if _, err := whaccess.NewOperatorRegistryAuthenticator(nil); err == nil {
		t.Fatal("registry authenticator built without a minter")
	}
	if _, err := whaccess.NewOperatorOperationsReadAuthenticator(nil); err == nil {
		t.Fatal("operations read authenticator built without a minter")
	}
	if _, err := whaccess.NewOperatorGoodsAuthenticator(nil); err == nil {
		t.Fatal("goods on-behalf authenticator built without a minter")
	}
	if _, err := whaccess.NewOperatorPreAdviceAuthenticator(nil); err == nil {
		t.Fatal("inbound pre-advice on-behalf authenticator built without a minter")
	}
}

// goodsDecisionRegister 只登「商品代录」这一种运营决定。
type goodsDecisionRegister struct {
	kinds []identity.DecisionKind
}

func (register goodsDecisionRegister) FindOperator(_ context.Context, subject identity.OperatorSubject) (identity.OperatorStanding, bool, error) {
	binding, err := identity.NewOperatorBinding(subject, operatorTenant, "SYN-BASIS-BINDING")
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(operatorNow.Add(-time.Hour), time.Time{})
	if err != nil {
		return identity.OperatorStanding{}, false, err
	}
	var grants []identity.RecordedGrant
	for index, kind := range register.kinds {
		grant, err := identity.NewOperationDecisionGrant(operatorTenant, "SYN-DECISION-"+string(rune('A'+index)), subject, kind, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return identity.OperatorStanding{}, false, err
		}
		recorded, err := identity.NewRecordedGrant(grant, nil)
		if err != nil {
			return identity.OperatorStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := identity.NewOperatorStanding(binding, grants)
	return standing, err == nil, err
}

// goodsAdmission 记下代录实际拿去问的那一对准入词。
type goodsAdmission struct {
	asked  identity.AdmissionRequirement
	calls  int
	admit  bool
	askErr error
}

func (scope *goodsAdmission) Admits(_ context.Context, _ string, requirement identity.AdmissionRequirement, _ time.Time) (bool, error) {
	scope.calls++
	scope.asked = requirement
	if scope.askErr != nil {
		return false, scope.askErr
	}
	return scope.admit, nil
}

func goodsAuthenticator(t *testing.T, register goodsDecisionRegister, admission identity.AdmissionScope) *whaccess.OperatorGoodsAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(operatorVerifier{}, register, admission, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := whaccess.NewOperatorGoodsAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: ADR-0384 决定二、决定八——商品代录核的是这一种决定的授予，不是登记册配置写；准入的能力词与事实类型词都是
// GOODS_RECORD_ON_BEHALF。没有这一种授予时不探准入范围。准入范围没登答不在准入范围，读不动与身份依赖读不动同一格。
func TestGoodsOnBehalfAsksForTheDecisionLiteralOnBothAdmissionWords(t *testing.T) {
	kind := identity.DecisionGoodsRecordOnBehalf
	granted := goodsDecisionRegister{kinds: []identity.DecisionKind{kind}}
	scope := &goodsAdmission{admit: true}

	operator, err := goodsAuthenticator(t, granted, scope).AuthenticateGoodsOnBehalf(context.Background(), "presented.operator.token")
	if err != nil || operator.Tenant.String() != operatorTenant || operator.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("operator = %+v, err = %v", operator, err)
	}
	if scope.calls != 1 || scope.asked.Capability != kind.String() || scope.asked.FactKind != kind.String() {
		t.Fatalf("准入词 = %+v，问了 %d 次，want 能力词与事实类型词都是 %s", scope.asked, scope.calls, kind)
	}

	onlyMeasurement := goodsDecisionRegister{kinds: []identity.DecisionKind{identity.DecisionManualMeasurementEntry}}
	silent := &goodsAdmission{admit: true}
	_, err = goodsAuthenticator(t, onlyMeasurement, silent).AuthenticateGoodsOnBehalf(context.Background(), "presented.operator.token")
	assertOnlyGrade(t, err, warehousehttp.ErrOperatorNotGranted)
	if silent.calls != 0 {
		t.Fatal("没有商品代录授予时不该探准入范围")
	}

	_, err = goodsAuthenticator(t, granted, identity.UnconfiguredAdmissionScope{}).AuthenticateGoodsOnBehalf(context.Background(), "presented.operator.token")
	assertOnlyGrade(t, err, warehousehttp.ErrOutsideAdmissionScope)

	_, err = goodsAuthenticator(t, granted, &goodsAdmission{askErr: errors.New("scope down")}).AuthenticateGoodsOnBehalf(context.Background(), "presented.operator.token")
	assertOnlyGrade(t, err, warehousehttp.ErrIdentityDependencyUnavailable)
}

// Covers: ADR-0384 决定二、决定八——入库预报代录核的是这一种决定的授予。准入的能力词与事实类型词都是
// INBOUND_PRE_ADVICE_RECORD_ON_BEHALF。没有这一种授予时不探准入范围。
func TestInboundPreAdviceOnBehalfAsksForTheDecisionLiteralOnBothAdmissionWords(t *testing.T) {
	kind := identity.DecisionInboundPreAdviceRecordOnBehalf
	granted := goodsDecisionRegister{kinds: []identity.DecisionKind{kind}}
	scope := &goodsAdmission{admit: true}

	operator, err := preAdviceAuthenticator(t, granted, scope).AuthenticatePreAdviceOnBehalf(context.Background(), "presented.operator.token")
	if err != nil || operator.Tenant.String() != operatorTenant || operator.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("operator = %+v, err = %v", operator, err)
	}
	if scope.calls != 1 || scope.asked.Capability != kind.String() || scope.asked.FactKind != kind.String() {
		t.Fatalf("准入词 = %+v，问了 %d 次，want 能力词与事实类型词都是 %s", scope.asked, scope.calls, kind)
	}

	onlyGoods := goodsDecisionRegister{kinds: []identity.DecisionKind{identity.DecisionGoodsRecordOnBehalf}}
	silent := &goodsAdmission{admit: true}
	_, err = preAdviceAuthenticator(t, onlyGoods, silent).AuthenticatePreAdviceOnBehalf(context.Background(), "presented.operator.token")
	assertOnlyGrade(t, err, warehousehttp.ErrOperatorNotGranted)
	if silent.calls != 0 {
		t.Fatal("没有入库预报代录授予时不该探准入范围")
	}
}

// Covers: ADR-0384 决定二、决定八——入库预报完结核的是这一种决定的授予。准入的能力词与事实类型词都是
// INBOUND_PRE_ADVICE_CLOSE。没有这一种授予时不探准入范围。
func TestInboundPreAdviceCloseAsksForTheDecisionLiteralOnBothAdmissionWords(t *testing.T) {
	kind := identity.DecisionInboundPreAdviceClose
	granted := goodsDecisionRegister{kinds: []identity.DecisionKind{kind}}
	scope := &goodsAdmission{admit: true}

	operator, err := preAdviceCloseAuthenticator(t, granted, scope).AuthenticatePreAdviceClose(context.Background(), "presented.operator.token")
	if err != nil || operator.Tenant.String() != operatorTenant || operator.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("operator = %+v, err = %v", operator, err)
	}
	if scope.calls != 1 || scope.asked.Capability != kind.String() || scope.asked.FactKind != kind.String() {
		t.Fatalf("准入词 = %+v，问了 %d 次，want 能力词与事实类型词都是 %s", scope.asked, scope.calls, kind)
	}

	onlyOnBehalf := goodsDecisionRegister{kinds: []identity.DecisionKind{identity.DecisionInboundPreAdviceRecordOnBehalf}}
	silent := &goodsAdmission{admit: true}
	_, err = preAdviceCloseAuthenticator(t, onlyOnBehalf, silent).AuthenticatePreAdviceClose(context.Background(), "presented.operator.token")
	assertOnlyGrade(t, err, warehousehttp.ErrOperatorNotGranted)
	if silent.calls != 0 {
		t.Fatal("没有入库预报完结授予时不该探准入范围")
	}
}

func preAdviceCloseAuthenticator(t *testing.T, register goodsDecisionRegister, admission identity.AdmissionScope) *whaccess.OperatorPreAdviceCloseAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(operatorVerifier{}, register, admission, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := whaccess.NewOperatorPreAdviceCloseAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func preAdviceAuthenticator(t *testing.T, register goodsDecisionRegister, admission identity.AdmissionScope) *whaccess.OperatorPreAdviceAuthenticator {
	t.Helper()
	minter, err := identity.NewOperatorMinter(operatorVerifier{}, register, admission, func() time.Time { return operatorNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := whaccess.NewOperatorPreAdviceAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func assertOnlyGrade(t *testing.T, err, want error) {
	t.Helper()
	for _, grade := range grades {
		if errors.Is(err, grade) != (grade == want) {
			t.Fatalf("err = %v: errors.Is(%v) = %v", err, grade, errors.Is(err, grade))
		}
	}
}
