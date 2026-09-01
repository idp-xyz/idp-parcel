package accessidentity

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// —— 测试替身 ——
//
// 三个口都没有生产实现（登记册的表等 PAR-INT-01、凭据形态还锁着、推导口的判重口径等
// S0），所以这里的替身不是「先用假的顶着」，它们是本轮唯一可能的驱动方。

type proofDouble struct{ key ChannelKey }

func (double proofDouble) ClaimedChannel() ChannelKey { return double.key }

type registryDouble struct {
	registration ChannelRegistration
	found        bool
	err          error
	askedKey     ChannelKey
}

func (double *registryDouble) FindChannel(_ context.Context, key ChannelKey) (ChannelRegistration, bool, error) {
	double.askedKey = key
	return double.registration, double.found, double.err
}

type verifierDouble struct {
	accepted     bool
	err          error
	seenProof    ChannelCredentialProof
	seenRefValue string
}

func (double *verifierDouble) VerifyCredential(
	_ context.Context,
	reference CredentialReference,
	proof ChannelCredentialProof,
) (bool, error) {
	double.seenRefValue = reference.String()
	double.seenProof = proof
	return double.accepted, double.err
}

// derivationDouble 提交与撤回各回各的串，用来钉住两个口没有被接成同一条。
type derivationDouble struct {
	submissionKey string
	withdrawalKey string
	err           error
}

func (double derivationDouble) DeriveSubmissionRequestKey(_ context.Context, _ ChannelRequest) (string, error) {
	return double.submissionKey, double.err
}

func (double derivationDouble) DeriveWithdrawalRequestKey(_ context.Context, _ ChannelRequest) (string, error) {
	return double.withdrawalKey, double.err
}

func mustRegistration(t *testing.T, derivation RequestKeyDerivation) ChannelRegistration {
	t.Helper()
	reference, err := NewCredentialReference("vault://channel/anchor-shipper")
	if err != nil {
		t.Fatalf("建受控引用：%v", err)
	}
	registration, err := NewChannelRegistration("TENANT-A", "ACCOUNT-A", "CHANNEL-API", reference, derivation)
	if err != nil {
		t.Fatalf("建登记行：%v", err)
	}
	return registration
}

func mustProof(t *testing.T) ChannelCredentialProof {
	t.Helper()
	key, err := NewChannelKey("chan_anchor")
	if err != nil {
		t.Fatalf("建登记查找键：%v", err)
	}
	return proofDouble{key: key}
}

func mustMinter(t *testing.T, registry ChannelRegistry, verifier CredentialVerifier) *Minter {
	t.Helper()
	minter, err := NewMinter(registry, verifier)
	if err != nil {
		t.Fatalf("建铸造器：%v", err)
	}
	return minter
}

// TestEnvelopeIdentityIgnoresWhateverTheRequestClaims 是 S2 验收点点名要的那一条：
// 报文里写别人的租户，铸出来的信封不变（ADR-0003 的最高隔离边界）。
func TestEnvelopeIdentityIgnoresWhateverTheRequestClaims(t *testing.T) {
	t.Parallel()

	registry := &registryDouble{registration: mustRegistration(t, derivationDouble{submissionKey: "REQ-1"}), found: true}
	minter := mustMinter(t, registry, &verifierDouble{accepted: true})

	// 报文里把三要素全写成别人的。推导口只影响来源请求键，身份三要素在签名上就够不着。
	hostile := NewChannelRequest(map[string]string{
		"tenantId":          "TENANT-VICTIM",
		"customerAccountId": "ACCOUNT-VICTIM",
		"source":            "CHANNEL-FORGED",
	})

	submission, err := minter.MintSubmission(context.Background(), mustProof(t), hostile)
	if err != nil {
		t.Fatalf("铸造提交信封：%v", err)
	}

	envelope := submission.Envelope()
	if envelope.TenantID() != "TENANT-A" {
		t.Errorf("租户取自报文而不是登记行：得 %q", envelope.TenantID())
	}
	if envelope.CustomerAccountID() != "ACCOUNT-A" {
		t.Errorf("客户账户取自报文而不是登记行：得 %q", envelope.CustomerAccountID())
	}
	if envelope.Source() != "CHANNEL-API" {
		t.Errorf("来源取自报文而不是登记行：得 %q", envelope.Source())
	}
	if envelope.RequestKey() != "REQ-1" {
		t.Errorf("来源请求键不是推导口交出的那个：得 %q", envelope.RequestKey())
	}
}

// TestSubmissionAndWithdrawalDoNotShareOneEnvelope 钉住 S2 的「两者不合用」。
//
// 类型那一半由编译器守（SubmissionEnvelope 与 WithdrawalEnvelope 互不可代），这里守的是
// 另一半：两个动作各走各的推导口，不会被接成同一条而在运行期得出同一个键。
func TestSubmissionAndWithdrawalDoNotShareOneEnvelope(t *testing.T) {
	t.Parallel()

	derivation := derivationDouble{submissionKey: "REQ-SUBMIT", withdrawalKey: "REQ-WITHDRAW"}
	registry := &registryDouble{registration: mustRegistration(t, derivation), found: true}
	minter := mustMinter(t, registry, &verifierDouble{accepted: true})
	same := NewChannelRequest(map[string]string{"orderNo": "SO-1"})

	submission, err := minter.MintSubmission(context.Background(), mustProof(t), same)
	if err != nil {
		t.Fatalf("铸造提交信封：%v", err)
	}
	withdrawal, err := minter.MintWithdrawal(context.Background(), mustProof(t), same)
	if err != nil {
		t.Fatalf("铸造撤回信封：%v", err)
	}

	if submission.Envelope().RequestKey() == withdrawal.Envelope().RequestKey() {
		t.Fatalf("同一份请求铸出同一个来源请求键 %q：撤回会被判成原提交的重放",
			submission.Envelope().RequestKey())
	}
	// 身份三要素本就该相同——是同一个渠道的同一个客户，分开的只是请求键那一维。
	if submission.Envelope().TenantID() != withdrawal.Envelope().TenantID() {
		t.Errorf("两个动作的租户不一致，登记行只有一个：%q / %q",
			submission.Envelope().TenantID(), withdrawal.Envelope().TenantID())
	}
}

// TestUnconfiguredChannelAndRejectedCredentialAreTwoAnswers 钉住 S2 的「两答可分辨」。
func TestUnconfiguredChannelAndRejectedCredentialAreTwoAnswers(t *testing.T) {
	t.Parallel()

	t.Run("册里没有这一行", func(t *testing.T) {
		t.Parallel()
		// 空册：found=false 且 err=nil。这一格必须走通而不是报错——空册可读是正常态。
		minter := mustMinter(t, &registryDouble{found: false}, &verifierDouble{accepted: true})

		_, err := minter.MintSubmission(context.Background(), mustProof(t), NewChannelRequest(nil))
		if !errors.Is(err, ErrAccessChannelNotConfigured) {
			t.Fatalf("空册没有答未配置：%v", err)
		}
	})

	t.Run("行在册但核验被拒", func(t *testing.T) {
		t.Parallel()
		registry := &registryDouble{registration: mustRegistration(t, derivationDouble{submissionKey: "REQ-1"}), found: true}
		minter := mustMinter(t, registry, &verifierDouble{accepted: false})

		_, err := minter.MintSubmission(context.Background(), mustProof(t), NewChannelRequest(nil))
		if !errors.Is(err, ErrCredentialRejected) {
			t.Fatalf("核验被拒没有答被拒：%v", err)
		}
		if errors.Is(err, ErrAccessChannelNotConfigured) {
			t.Fatal("核验被拒折进了未配置：两格的恢复动作不同，折在一起会把人指错方向")
		}
	})
}

// TestRegistryFailureIsNotAnUnconfiguredAnswer 守的是第三个方向：读不动登记册是依赖
// 故障，不能顶成未配置，否则运维会去配一个其实已经配好的渠道。
func TestRegistryFailureIsNotAnUnconfiguredAnswer(t *testing.T) {
	t.Parallel()

	broken := errors.New("dial registry: connection refused")
	minter := mustMinter(t, &registryDouble{err: broken}, &verifierDouble{accepted: true})

	_, err := minter.MintSubmission(context.Background(), mustProof(t), NewChannelRequest(nil))
	if !errors.Is(err, broken) {
		t.Fatalf("依赖故障没有原样交回：%v", err)
	}
	if errors.Is(err, ErrAccessChannelNotConfigured) || errors.Is(err, ErrCredentialRejected) {
		t.Fatalf("依赖故障被折进了那两格之一：%v", err)
	}
}

// TestRegistrationWithoutDerivationCannotExist 钉住登记行的完整性。
//
// 推导口缺席若能建成登记行，铸造侧就会多出「行在册但判重口径没配」这第三态，而它与
// 未配置在答复上分不开。要求同笔给全，那一态就不存在。
func TestRegistrationWithoutDerivationCannotExist(t *testing.T) {
	t.Parallel()

	reference, err := NewCredentialReference("vault://channel/anchor-shipper")
	if err != nil {
		t.Fatalf("建受控引用：%v", err)
	}

	cases := map[string]struct {
		tenant     string
		account    string
		source     string
		derivation RequestKeyDerivation
	}{
		"缺推导口":  {"TENANT-A", "ACCOUNT-A", "CHANNEL-API", nil},
		"缺租户":   {"  ", "ACCOUNT-A", "CHANNEL-API", derivationDouble{}},
		"缺客户账户": {"TENANT-A", "", "CHANNEL-API", derivationDouble{}},
		"缺来源":   {"TENANT-A", "ACCOUNT-A", "", derivationDouble{}},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewChannelRegistration(
				testCase.tenant, testCase.account, testCase.source, reference, testCase.derivation,
			); !errors.Is(err, ErrIncompleteRegistration) {
				t.Fatalf("缺件的登记行建成了：%v", err)
			}
		})
	}
}

// TestUnderivedRequestKeyIsItsOwnAnswer 守「推导口交回空串」这一格不被静默放过。
//
// 空串若封进信封，下游拿到的是一个四要素之一为空的身份，而它会在离铸造很远的地方
// 才炸——那时看不出是接入侧没配对字段。
func TestUnderivedRequestKeyIsItsOwnAnswer(t *testing.T) {
	t.Parallel()

	registry := &registryDouble{registration: mustRegistration(t, derivationDouble{submissionKey: "   "}), found: true}
	minter := mustMinter(t, registry, &verifierDouble{accepted: true})

	_, err := minter.MintSubmission(context.Background(), mustProof(t), NewChannelRequest(nil))
	if !errors.Is(err, ErrRequestKeyNotDerived) {
		t.Fatalf("空的来源请求键没有自成一答：%v", err)
	}
}

// TestCredentialProofExposesNothingButTheChannelKey 守住 ADR-0072 Decision 二挡着的那
// 半边：凭据**形态**不在本轮范围内。
//
// 判据落在接口的方法集上而不是留给注释：往 ChannelCredentialProof 上加一个
// Secret()、Certificate() 或 SessionToken()，就是在替某一种渠道拟凭据形态，而那件事
// 要等 PAR-INT-01。加了这条，那一天会变红而不是悄悄过去。
func TestCredentialProofExposesNothingButTheChannelKey(t *testing.T) {
	t.Parallel()

	proofType := reflect.TypeOf((*ChannelCredentialProof)(nil)).Elem()
	if proofType.NumMethod() != 1 {
		var names []string
		for index := 0; index < proofType.NumMethod(); index++ {
			names = append(names, proofType.Method(index).Name)
		}
		t.Fatalf("凭证接口不再只交出查找键，方法集为 %v：多出来的每一个都在替某种渠道拟凭据形态", names)
	}
	if got := proofType.Method(0).Name; got != "ClaimedChannel" {
		t.Fatalf("凭证接口的唯一方法变成了 %q", got)
	}
}

// TestVerifierGetsTheRegisteredReferenceAndTheProof 是上一条的阳性对照：本包不拆凭证，
// 不能顺手把它也从核验方那里挡掉——那样核验永远做不成，而测试只会看见「被拒」。
func TestVerifierGetsTheRegisteredReferenceAndTheProof(t *testing.T) {
	t.Parallel()

	registry := &registryDouble{registration: mustRegistration(t, derivationDouble{submissionKey: "REQ-1"}), found: true}
	verifier := &verifierDouble{accepted: true}
	minter := mustMinter(t, registry, verifier)

	if _, err := minter.MintSubmission(context.Background(), mustProof(t), NewChannelRequest(nil)); err != nil {
		t.Fatalf("铸造提交信封：%v", err)
	}
	if verifier.seenProof == nil {
		t.Fatal("核验方没拿到凭证：本包不拆它，但必须原样递过去")
	}
	if verifier.seenRefValue != "vault://channel/anchor-shipper" {
		t.Errorf("核验方拿到的不是登记行上的受控引用：得 %q", verifier.seenRefValue)
	}
	if registry.askedKey.String() != "chan_anchor" {
		t.Errorf("登记册不是按凭证认领的键查的：得 %q", registry.askedKey.String())
	}
}

// TestMissingProofIsAnAnswerNotAPanic 守 nil 凭证不把装配缺陷报成崩溃。
func TestMissingProofIsAnAnswerNotAPanic(t *testing.T) {
	t.Parallel()

	registry := &registryDouble{registration: mustRegistration(t, derivationDouble{submissionKey: "REQ-1"}), found: true}
	minter := mustMinter(t, registry, &verifierDouble{accepted: true})

	if _, err := minter.MintSubmission(context.Background(), nil, NewChannelRequest(nil)); !errors.Is(err, ErrInvalidChannelKey) {
		t.Fatalf("缺凭证没有如实作答：%v", err)
	}
	if _, err := minter.MintWithdrawal(context.Background(), proofDouble{}, NewChannelRequest(nil)); !errors.Is(err, ErrInvalidChannelKey) {
		t.Fatalf("凭证不说认领哪一行也没有如实作答：%v", err)
	}
}

// TestZeroEnvelopeCarriesNothingUsable 守「零值不冒充已铸造」。
//
// 包外造不出非零 SourceEnvelope（字段不导出、无导出构造函数），但零值它造得出。这一条
// 钉住零值四要素全空，于是忘了铸造的调用方拿到的是空手而不是一个看着像身份的东西。
func TestZeroEnvelopeCarriesNothingUsable(t *testing.T) {
	t.Parallel()

	var envelope SourceEnvelope
	if envelope.TenantID() != "" || envelope.CustomerAccountID() != "" ||
		envelope.Source() != "" || envelope.RequestKey() != "" {
		t.Fatal("零值信封带了内容：零值会冒充成一次已铸造的身份")
	}
}

// TestMinterRefusesToBeBuiltWithoutItsPorts 守铸造器不带口就建不成——带 nil 口建成的
// 铸造器会在第一次真请求时 panic，而那时离装配点已经很远。
func TestMinterRefusesToBeBuiltWithoutItsPorts(t *testing.T) {
	t.Parallel()

	if _, err := NewMinter(nil, &verifierDouble{}); !errors.Is(err, ErrNilDependency) {
		t.Errorf("缺登记册装载口也建成了：%v", err)
	}
	if _, err := NewMinter(&registryDouble{}, nil); !errors.Is(err, ErrNilDependency) {
		t.Errorf("缺凭据核验口也建成了：%v", err)
	}
}
