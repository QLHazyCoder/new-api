package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func TestTopUpPricingSnapshotResponseIsolation(t *testing.T) {
	snapshot := model.TopUpPricingSnapshot{
		Version:         model.TopUpPricingSnapshotVersion,
		PaymentProvider: model.PaymentProviderEpay,
		UserGroup:       "vip",
		PayMoney:        94,
	}
	rawSnapshot, err := snapshot.Marshal()
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	topUp := &model.TopUp{
		Id:              1,
		TradeNo:         "snapshot-order",
		PricingSnapshot: rawSnapshot,
	}

	userPayload, err := common.Marshal(topUp)
	if err != nil {
		t.Fatalf("marshal user payload: %v", err)
	}
	if strings.Contains(string(userPayload), "pricing_snapshot") {
		t.Fatalf("user payload leaked pricing snapshot: %s", userPayload)
	}

	adminRecord, err := newAdminTopUpRecord(topUp)
	if err != nil {
		t.Fatalf("build admin record: %v", err)
	}
	adminPayload, err := common.Marshal(adminRecord)
	if err != nil {
		t.Fatalf("marshal admin payload: %v", err)
	}
	if !strings.Contains(string(adminPayload), `"pricing_snapshot"`) {
		t.Fatalf("admin payload omitted pricing snapshot: %s", adminPayload)
	}
	if adminRecord.PricingSnapshot == nil || adminRecord.PricingSnapshot.UserGroup != "vip" {
		t.Fatalf("unexpected parsed snapshot: %#v", adminRecord.PricingSnapshot)
	}
}

func TestNewAdminTopUpRecordLegacyAndInvalidSnapshots(t *testing.T) {
	legacy, err := newAdminTopUpRecord(&model.TopUp{TradeNo: "legacy-order"})
	if err != nil {
		t.Fatalf("legacy snapshot returned error: %v", err)
	}
	if legacy.PricingSnapshot != nil {
		t.Fatalf("legacy order should not have a parsed snapshot: %#v", legacy.PricingSnapshot)
	}

	invalid, err := newAdminTopUpRecord(&model.TopUp{
		TradeNo:         "invalid-order",
		PricingSnapshot: `{"version":0}`,
	})
	if err == nil {
		t.Fatal("invalid snapshot should return an error")
	}
	if invalid.PricingSnapshot != nil {
		t.Fatalf("invalid snapshot should not be exposed: %#v", invalid.PricingSnapshot)
	}
}
