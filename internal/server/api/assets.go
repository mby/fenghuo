package api

import (
	"errors"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/mby/fenghuo/internal/server/store"
)

func validateAsset(a *store.Asset) error {
	a.Provider, a.Region, a.Tags = strings.TrimSpace(a.Provider), strings.TrimSpace(a.Region), strings.TrimSpace(a.Tags)
	for _, f := range []struct {
		name, value string
		limit       int
	}{{"provider", a.Provider, 64}, {"region", a.Region, 64}, {"tags", a.Tags, 200}} {
		if err := checkText(f.name, f.value, 0, f.limit); err != nil {
			return err
		}
	}
	if math.IsNaN(a.Price) || math.IsInf(a.Price, 0) || a.Price < 0 || a.Price > 10000000 {
		return errors.New("price must be between 0 and 10000000")
	}
	a.Price = math.Round(a.Price*100) / 100
	a.Currency = strings.ToUpper(strings.TrimSpace(a.Currency))
	if a.Currency == "" {
		a.Currency = "CNY"
	}
	if len(a.Currency) != 3 || strings.IndexFunc(a.Currency, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
		return errors.New("currency must be a three-letter code")
	}
	if a.BillingMonths == 0 {
		a.BillingMonths = 1
	}
	if !slices.Contains([]int{1, 3, 6, 12, 24, 36}, a.BillingMonths) {
		return errors.New("billing_months must be 1, 3, 6, 12, 24 or 36")
	}
	return nil
}

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.Costs(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
