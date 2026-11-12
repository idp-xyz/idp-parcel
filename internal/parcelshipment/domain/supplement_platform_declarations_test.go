package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉受理前受控补充对平台声明的整份重报（ADR-0376 决定二，票 declared-goods-lines/10）：补充带了平台段就替换委托上的平台
// 货件与全部箱标号；形状规则与首次提交同一处——箱标号要有平台货件、只指成员、委托内不重号；接受之后没有补充这条路。

// Covers: 补充补报箱标号——首次提交只报了平台货件，补充补上两箱的箱标号并改了平台参考号：委托此刻的平台声明就是补充这一份。
func TestASupplementReplacesThePlatformDeclarations(t *testing.T) {
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "SYN-REF-1")
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit shipment request: %v", err)
	}

	next := supersessionSpecFor(t, "parcel-1", "parcel-2")
	next.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "SYN-REF-2")
	next.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{
		boxLabel(t, "parcel-1", "FBA15SYNTESTU000001"), boxLabel(t, "parcel-2", "FBA15SYNTESTU000002"),
	}
	superseded, err := request.FormNewSubmissionVersion(next)
	if err != nil {
		t.Fatalf("form new submission version: %v", err)
	}
	shipment, declared := superseded.PlatformShipment()
	if !declared {
		t.Fatal("补充带了平台货件，委托上却没有")
	}
	if reference, present := shipment.ReferenceNumber(); !present || reference.String() != "SYN-REF-2" {
		t.Fatalf("平台参考号 = %v, want 补充改成的 SYN-REF-2", reference)
	}
	if labels := superseded.PlatformBoxLabels(); len(labels) != 2 {
		t.Fatalf("箱标号 %d 个, want 补充补报的 2 个", len(labels))
	}
}

// Covers: 补充的平台段照首次提交的形状规则核——没有平台货件的箱标号、两箱同号、指到成员之外的箱标号都拒，委托原样不动。
func TestASupplementsPlatformDeclarationsFollowTheSubmissionShapeRules(t *testing.T) {
	request, err := domain.SubmitShipmentRequest(submitSpec(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("submit shipment request: %v", err)
	}
	for name, mutate := range map[string]func(*domain.NewSubmissionVersionSpec){
		"箱标号没有平台货件": func(spec *domain.NewSubmissionVersionSpec) {
			spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "parcel-1", "FBA15SYNTESTU000001")}
		},
		"两箱同号": func(spec *domain.NewSubmissionVersionSpec) {
			spec.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "")
			spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{
				boxLabel(t, "parcel-1", "FBA15SYNTESTU000001"), boxLabel(t, "parcel-2", "FBA15SYNTESTU000001"),
			}
		},
		"箱标号指到成员之外": func(spec *domain.NewSubmissionVersionSpec) {
			spec.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "")
			spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "parcel-9", "FBA15SYNTESTU000009")}
		},
	} {
		t.Run(name, func(t *testing.T) {
			next := supersessionSpecFor(t, "parcel-1", "parcel-2")
			mutate(&next)
			if _, err := request.FormNewSubmissionVersion(next); !errors.Is(err, domain.ErrInvalidPlatformIdentifier) {
				t.Fatalf("err = %v, want ErrInvalidPlatformIdentifier", err)
			}
		})
	}
}
