package store

import (
	"context"
	"fmt"
	"math"
)

// Asset is administrator-only inventory. Prices describe a billing cycle;
// monthly and annual totals are estimates without currency conversion.
type Asset struct {
	Provider      string  `json:"provider"`
	Region        string  `json:"region"`
	Tags          string  `json:"tags"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"`
	BillingMonths int     `json:"billing_months"`
	AutoRenew     bool    `json:"auto_renew"`
}

type CostSummary struct {
	Currency string  `json:"currency"`
	Nodes    int     `json:"nodes"`
	Monthly  float64 `json:"monthly"`
	Annual   float64 `json:"annual"`
}

func (s *Store) Costs(ctx context.Context) ([]CostSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT currency, COUNT(*), SUM(price / billing_months)
		FROM nodes WHERE price > 0 AND billing_months > 0 GROUP BY currency ORDER BY currency`)
	if err != nil {
		return nil, fmt.Errorf("query costs: %w", err)
	}
	defer rows.Close()
	out := []CostSummary{}
	for rows.Next() {
		var c CostSummary
		if err := rows.Scan(&c.Currency, &c.Nodes, &c.Monthly); err != nil {
			return nil, fmt.Errorf("scan costs: %w", err)
		}
		c.Annual = math.Round(c.Monthly*1200) / 100
		c.Monthly = math.Round(c.Monthly*100) / 100
		out = append(out, c)
	}
	return out, rows.Err()
}
