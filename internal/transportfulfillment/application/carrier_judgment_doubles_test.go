package application_test

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// judgmentRegistryDouble 是 ports.ActualCarrierJudgmentRegistry 的替身。
//
// 它按版本行存、读时走 RehydrateActualCarrierJudgment 装回，与真库适配器同一道重建门——判据同
// segmentRegistryDouble：替身若直接交回内存里那份聚合，「装不回的判断」这一类在编排里就永远测不出来。
type judgmentRegistryDouble struct {
	rows      map[string]*judgmentRowsDouble
	findErr   error
	openErr   error
	appendErr error
	// forceAppend 让 AppendVersion 无条件答某个结果，用来演并发撞序号那一格。
	forceAppend ports.JudgmentVersionAppendOutcome
	opens       int
	appends     int
}

type judgmentRowsDouble struct {
	key           ports.ActualCarrierJudgmentKey
	establishedAt time.Time
	versions      []domain.RehydrateActualCarrierJudgmentVersionSpec
	recordedAt    time.Time
}

func newJudgmentRegistry() *judgmentRegistryDouble {
	return &judgmentRegistryDouble{rows: map[string]*judgmentRowsDouble{}}
}

func judgmentRegistryKey(key ports.ActualCarrierJudgmentKey) string {
	return key.TenantID.String() + "|" + key.Segment.String()
}

// judgmentVersionSpecOf 把领域版本摊回库面的样子，逐格取自它自己的读面。
func judgmentVersionSpecOf(version domain.ActualCarrierJudgmentVersion) domain.RehydrateActualCarrierJudgmentVersionSpec {
	spec := domain.RehydrateActualCarrierJudgmentVersionSpec{
		Sequence:     version.Sequence(),
		BusinessTime: version.BusinessTime(),
		FormedAt:     version.FormedAt(),
		Bases:        version.Bases(),
	}
	if subject, identified := version.Verdict().Identified(); identified {
		spec.Subject = subject
	}
	if reason, pending := version.Verdict().Pending(); pending {
		spec.Pending = reason
	}
	return spec
}

func (double *judgmentRegistryDouble) FindByKey(
	_ context.Context,
	key ports.ActualCarrierJudgmentKey,
) (ports.ActualCarrierJudgmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.ActualCarrierJudgmentRecord{}, false, double.findErr
	}
	rows, found := double.rows[judgmentRegistryKey(key)]
	if !found {
		return ports.ActualCarrierJudgmentRecord{}, false, nil
	}
	judgment, err := domain.RehydrateActualCarrierJudgment(domain.RehydrateActualCarrierJudgmentSpec{
		TenantID:      key.TenantID,
		Segment:       key.Segment,
		EstablishedAt: rows.establishedAt,
		Versions:      rows.versions,
	})
	if err != nil {
		return ports.ActualCarrierJudgmentRecord{}, false, err
	}
	return ports.ActualCarrierJudgmentRecord{Key: key, Judgment: judgment, RecordedAt: rows.recordedAt}, true, nil
}

func (double *judgmentRegistryDouble) Open(
	_ context.Context,
	record ports.ActualCarrierJudgmentRecord,
) (ports.JudgmentOpenOutcome, error) {
	double.opens++
	if double.openErr != nil {
		return ports.JudgmentOpenOutcomeInvalid, double.openErr
	}
	if _, exists := double.rows[judgmentRegistryKey(record.Key)]; exists {
		return ports.JudgmentAlreadyOpened, nil
	}
	rows := &judgmentRowsDouble{
		key:           record.Key,
		establishedAt: record.Judgment.SegmentEstablishedAt(),
		recordedAt:    record.RecordedAt,
	}
	for _, version := range record.Judgment.Versions() {
		rows.versions = append(rows.versions, judgmentVersionSpecOf(version))
	}
	double.rows[judgmentRegistryKey(record.Key)] = rows
	return ports.JudgmentOpened, nil
}

func (double *judgmentRegistryDouble) AppendVersion(
	_ context.Context,
	key ports.ActualCarrierJudgmentKey,
	version domain.ActualCarrierJudgmentVersion,
	recordedAt time.Time,
) (ports.JudgmentVersionAppendOutcome, error) {
	double.appends++
	if double.appendErr != nil {
		return ports.JudgmentVersionAppendOutcomeInvalid, double.appendErr
	}
	if double.forceAppend != ports.JudgmentVersionAppendOutcomeInvalid {
		return double.forceAppend, nil
	}
	rows, found := double.rows[judgmentRegistryKey(key)]
	if !found {
		return ports.JudgmentVersionAppendOutcomeInvalid, nil
	}
	for _, existing := range rows.versions {
		if existing.Sequence == version.Sequence() {
			return ports.JudgmentVersionAlreadyRecorded, nil
		}
	}
	rows.versions = append(rows.versions, judgmentVersionSpecOf(version))
	rows.recordedAt = recordedAt
	return ports.JudgmentVersionAppended, nil
}

// identityDirectoryDouble 是 ports.CarrierIdentityDirectory 的替身：按（分支，引用）答在册与否。
type identityDirectoryDouble struct {
	registered map[string]bool
	err        error
	asked      []domain.CarrierSubject
}

func newIdentityDirectory(registered ...domain.CarrierSubject) *identityDirectoryDouble {
	double := &identityDirectoryDouble{registered: map[string]bool{}}
	for _, subject := range registered {
		double.registered[subject.Kind().String()+"|"+subject.Reference()] = true
	}
	return double
}

func (double *identityDirectoryDouble) IdentityRegistered(
	_ context.Context,
	_ domain.TenantID,
	subject domain.CarrierSubject,
) (bool, error) {
	double.asked = append(double.asked, subject)
	if double.err != nil {
		return false, double.err
	}
	return double.registered[subject.Kind().String()+"|"+subject.Reference()], nil
}
