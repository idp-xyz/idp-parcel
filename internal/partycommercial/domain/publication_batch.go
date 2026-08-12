package domain

import "time"

// PublicationBatchItem 是批次中的一项草稿发布意图。批次不是聚合：项与项只有折叠顺序，
// 没有共同命运（ADR-0037 / AT-PC-011）。
type PublicationBatchItem struct {
	Draft        CommercialVersion
	Basis        ApprovalBasis
	RoleStanding ApprovalRoleStanding
	PublishedAt  time.Time
}

// PublicationBatchItemResult 是单项折叠结果。已成功写入登记册的前项，不得因本项失败被撤出。
type PublicationBatchItemResult struct {
	kind         CommercialObjectKind
	objectID     CommercialObjectID
	version      CommercialVersionLabel
	published    CommercialVersion
	hasPublished bool
	registration RegistrationOutcome
	err          error
}

func (result PublicationBatchItemResult) Kind() CommercialObjectKind {
	return result.kind
}

func (result PublicationBatchItemResult) ObjectID() CommercialObjectID {
	return result.objectID
}

func (result PublicationBatchItemResult) Version() CommercialVersionLabel {
	return result.version
}

func (result PublicationBatchItemResult) Published() (CommercialVersion, bool) {
	return result.published, result.hasPublished
}

func (result PublicationBatchItemResult) Registration() RegistrationOutcome {
	return result.registration
}

func (result PublicationBatchItemResult) Err() error {
	return result.err
}

// PublishBatch 按序对每一项执行 Publish 然后 Register，各自留下独立结果。
// standing 为 nil 时，用登记册当前已发布对象回答指名引用存续，使同批先写入的对象
// 可被后项看见——仍然不是事务，更不是 Outbox。
func (registry *CommercialRegistry) PublishBatch(
	items []PublicationBatchItem,
	standing NamedReferenceStandingLookup,
) []PublicationBatchItemResult {
	results := make([]PublicationBatchItemResult, 0, len(items))
	for _, item := range items {
		result := PublicationBatchItemResult{
			kind:     item.Draft.Kind(),
			objectID: item.Draft.ObjectID(),
			version:  item.Draft.Version(),
		}
		lookup := standing
		if lookup == nil {
			lookup = registry.standingOfPublishedObjects()
		}
		published, err := item.Draft.Publish(item.Basis, item.RoleStanding, item.PublishedAt, lookup)
		if err != nil {
			result.err = err
			results = append(results, result)
			continue
		}
		result.published = published
		result.hasPublished = true
		outcome, regErr := registry.Register(published)
		result.registration = outcome
		result.err = regErr
		results = append(results, result)
	}
	return results
}

// standingOfPublishedObjects 把登记册里非草稿对象读成「已发布」存续。批次折叠用它补齐
// 调用方未传入的 lookup，不猜测登记册外的事实。
func (registry *CommercialRegistry) standingOfPublishedObjects() NamedReferenceStandingLookup {
	return func(kind CommercialObjectKind, objectID CommercialObjectID) NamedReferenceStanding {
		if registry == nil {
			return NamedReferenceStandingInvalid
		}
		for _, version := range registry.versions {
			if version.kind != kind || version.objectID != objectID {
				continue
			}
			if version.status == CommercialVersionStatusInvalid || version.status == CommercialVersionDraft {
				continue
			}
			return NamedReferencePublished
		}
		return NamedReferenceUnpublished
	}
}
