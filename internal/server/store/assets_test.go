package store

import (
	"context"
	"testing"
)

func TestAssetCostsAndRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for i, a := range []Asset{
		{Provider: "provider", Region: "Tokyo", Tags: "prod,web", Price: 120, Currency: "USD", BillingMonths: 12, AutoRenew: true},
		{Price: 30, Currency: "USD", BillingMonths: 3},
		{Price: 60, Currency: "CNY", BillingMonths: 1},
		{Price: 0, Currency: "CNY", BillingMonths: 1},
	} {
		n := Node{Name: "asset", TokenHash: string(rune('a' + i)), Asset: a}
		if err := s.CreateNode(ctx, &n); err != nil {
			t.Fatal(err)
		}
	}
	nodes, err := s.ListNodes(ctx)
	if err != nil || len(nodes) != 4 {
		t.Fatalf("nodes: %v %v", nodes, err)
	}
	if nodes[0].Provider != "provider" || nodes[0].Region != "Tokyo" || nodes[0].Tags != "prod,web" || !nodes[0].AutoRenew {
		t.Fatalf("asset round trip: %+v", nodes[0].Asset)
	}
	costs, err := s.Costs(ctx)
	if err != nil || len(costs) != 2 {
		t.Fatalf("costs: %v %v", costs, err)
	}
	for _, tt := range []struct {
		currency        string
		monthly, annual float64
		count           int
	}{{"CNY", 60, 720, 1}, {"USD", 20, 240, 2}} {
		t.Run(tt.currency, func(t *testing.T) {
			for _, c := range costs {
				if c.Currency == tt.currency {
					if c.Monthly != tt.monthly || c.Annual != tt.annual || c.Nodes != tt.count {
						t.Fatalf("costs: %+v", c)
					}
					return
				}
			}
			t.Fatal("missing currency")
		})
	}
	nodes[0].Price = 240
	nodes[0].AutoRenew = false
	if err := s.UpdateNode(ctx, nodes[0]); err != nil {
		t.Fatal(err)
	}
	nodes, err = s.ListNodes(ctx)
	if err != nil || nodes[0].Price != 240 || nodes[0].AutoRenew {
		t.Fatalf("update: %v %v", nodes, err)
	}
}
