package service

import (
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRequiredAttributesReturnsOnlyMatchingConditionalRules(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.BlockRule{}); err != nil {
		t.Fatal(err)
	}
	svc := NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	for _, rule := range []model.BlockRule{
		{PackageName: "lodash", Version: "4.*", MatchType: model.BlockMatchWildcard, PackageType: "npm", Enabled: true, ConditionType: model.ConditionTypeLicense, ConditionOp: model.ConditionOpEquals, ConditionValue: "GPL-3.0"},
		{PackageName: "*", Version: "*", MatchType: model.BlockMatchWildcard, PackageType: model.PackageTypeAll, Enabled: true, ConditionType: model.ConditionTypePublishTime, ConditionOp: model.ConditionOpWithinLast, ConditionValue: "7"},
	} {
		if err := svc.Create(&rule); err != nil {
			t.Fatal(err)
		}
	}

	requirements := svc.RequiredAttributes("npm", "lodash", "4.17.21")
	if len(requirements) != 2 {
		t.Fatalf("requirements = %#v, want two matching conditional attributes", requirements)
	}
	got := map[string]bool{}
	for _, requirement := range requirements {
		got[requirement.Attribute] = true
	}
	if !got["license"] || !got["published_at"] {
		t.Fatalf("attributes = %#v, want license and published_at", got)
	}
}
