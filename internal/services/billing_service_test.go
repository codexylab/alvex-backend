package services

import (
	"testing"

	"github.com/codexylab/alvex-backend/internal/models"
)

func TestClientMonthlyCost(t *testing.T) {
	tests := []struct {
		plan     models.BillingPlan
		expected float64
	}{
		{models.BillingBasic, 29.00},
		{models.BillingPro, 99.00},
		{models.BillingEnterprise, 499.00},
	}

	for _, tt := range tests {
		client := &models.Client{BillingPlan: tt.plan}
		got := client.MonthlyCost()
		if got != tt.expected {
			t.Errorf("MonthlyCost for %s = %f; want %f", tt.plan, got, tt.expected)
		}
	}

	// Custom plan
	customRate := 150.00
	customClient := &models.Client{
		BillingPlan: models.BillingCustom,
		CustomRate:  &customRate,
	}
	if customClient.MonthlyCost() != 150.00 {
		t.Errorf("Custom rate expected 150.00, got %f", customClient.MonthlyCost())
	}
}

func TestMaskedAPIKey(t *testing.T) {
	c := &models.Client{APIKey: "ALVX-NEXD-8921xab3c4f2d"}
	masked := c.MaskedAPIKey()
	if masked != "ALVX-NEXD••••" {
		t.Errorf("MaskedAPIKey() = %q; want %q", masked, "ALVX-NEXD••••")
	}

	short := &models.Client{APIKey: "short"}
	if short.MaskedAPIKey() != "••••••••" {
		t.Errorf("MaskedAPIKey() for short key = %q; want %q", short.MaskedAPIKey(), "••••••••")
	}
}
