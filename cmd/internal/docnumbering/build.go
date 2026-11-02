package docnumbering

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
)

// Components 是编号组件在一个进程里的那一套：同一份类型汇总、同一本规则册、同一组加扰密钥。
type Components struct {
	// Registrations 是包进事务的规则册登记：规则册的写口无事务即拒，登记口与受控登记口都经它写。
	Registrations Registrations
	// Reader 答规则册的读与预览，不读也不占序列。
	Reader *docnumber.Reader
	// Assigner 是取号编排。开立单据的上下文经自己的消费侧端口接它，在开立事务的 ctx 里调；收到撞号或错误必须回滚
	// 开立事务（见 docnumber.NumberingRuleCollision）。
	Assigner *docnumber.Assigner
}

// Build 在 db 上装编号组件。规则册、序列与开立事务接在同一个 *bentopg.DB 上：框架按 DB 认事务归属，换一个 DB
// 构造的序列存储在开立事务里会被拒。各取号进程启动时调它，装不起来（声明不合规、序列存储构造失败）就带原因退出，
// 而不是等第一张单据开立。
func Build(db *bentopg.DB, secrets docnumber.ScrambleSecrets) (Components, error) {
	if db == nil || secrets == nil {
		return Components{}, errors.New("document numbering: build needs a db and scramble secrets")
	}
	catalogue, err := Catalogue()
	if err != nil {
		return Components{}, fmt.Errorf("document numbering: aggregate type declarations: %w", err)
	}
	store, err := docnumberpg.NewStore(db)
	if err != nil {
		return Components{}, err
	}
	sequences, err := docnumberpg.NewSequences(db)
	if err != nil {
		return Components{}, err
	}
	clock := serverClock{}
	registrar, err := docnumber.NewRegistrar(docnumber.RegistrarDeps{Catalogue: catalogue, Book: store, Secrets: secrets, Clock: clock})
	if err != nil {
		return Components{}, err
	}
	reader, err := docnumber.NewReader(docnumber.ReaderDeps{Catalogue: catalogue, Book: store, Secrets: secrets, Clock: clock})
	if err != nil {
		return Components{}, err
	}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: store, Sequences: sequences, Transactions: store, Secrets: secrets, Clock: clock,
	})
	if err != nil {
		return Components{}, err
	}
	return Components{
		Registrations: Registrations{transactor: db.Transactor(), inner: registrar},
		Reader:        reader,
		Assigner:      assigner,
	}, nil
}

// serverClock 是服务端时钟：登记时刻与取号时刻都取它，不取载荷声明的时刻（ADR-0355 决定七）。
type serverClock struct{}

func (serverClock) Now() time.Time { return time.Now().UTC() }

// Registrations 把一次规则册登记包进一笔事务；编排返回 error 时整笔回滚。
type Registrations struct {
	transactor bentoapp.Transactor
	inner      *docnumber.Registrar
}

func (registrations Registrations) RegisterRule(ctx context.Context, command docnumber.RegisterRuleCommand) (docnumber.RuleRegistration, error) {
	if registrations.inner == nil {
		return docnumber.RuleRegistration{}, errors.New("document numbering: registrations are not built")
	}
	var result docnumber.RuleRegistration
	err := registrations.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := registrations.inner.RegisterRule(txCtx, command)
		result = handled
		return err
	})
	if err != nil {
		return docnumber.RuleRegistration{}, err
	}
	return result, nil
}

func (registrations Registrations) RegisterTimeZone(ctx context.Context, command docnumber.RegisterTimeZoneCommand) (docnumber.TimeZoneRegistration, error) {
	if registrations.inner == nil {
		return docnumber.TimeZoneRegistration{}, errors.New("document numbering: registrations are not built")
	}
	var result docnumber.TimeZoneRegistration
	err := registrations.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := registrations.inner.RegisterTimeZone(txCtx, command)
		result = handled
		return err
	})
	if err != nil {
		return docnumber.TimeZoneRegistration{}, err
	}
	return result, nil
}

// LoadScrambleSecrets 读加扰密钥那一项实例配置（docnumber.ScrambleSecretsFileEnv），各取号与登记进程读的是同一项。
// 配了而读不动或不合规交回错误，进程在开池之前带原因退出。logger 非空时配没配都出声：只说配了几个租户，不说是谁、
// 更不说密钥——没配时用加扰的模板登记答 SCRAMBLE_SECRET_NOT_CONFIGURED，启动日志是分辨「没配」与「配了但不含这个
// 租户」的第一处。
func LoadScrambleSecrets(getenv func(string) string, readFile func(string) ([]byte, error), logger *slog.Logger) (docnumber.SecretTable, error) {
	table, configured, err := docnumber.LoadScrambleSecrets(getenv, readFile)
	if err != nil {
		return nil, err
	}
	if logger != nil {
		if configured {
			logger.Info("Document number scramble secrets configured (ADR-0355): scrambled templates may be registered and issued for the listed tenants",
				"env", docnumber.ScrambleSecretsFileEnv, "tenants", table.Tenants())
		} else {
			logger.Info("Document number scramble secrets not configured (ADR-0355): registrations of scrambled templates answer SCRAMBLE_SECRET_NOT_CONFIGURED",
				"env", docnumber.ScrambleSecretsFileEnv)
		}
	}
	return table, nil
}
