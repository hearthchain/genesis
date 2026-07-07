// Package journal loads the canonical weekly price journal (CSV artifact:
// week_end ISO date, price_avg_usd decimal string) and answers the credit
// formula's only price query: the maximum weekly-average price since a date.
// CSV decimal strings are parsed straight to micro-USD integers; floats never
// enter the pipeline.
package journal

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Week is one journal row: the Sunday (UTC) the week ends on and the weekly
// average of daily closing prices, in micro-USD.
type Week struct {
	End      time.Time
	PriceMic uint64
}

// Journal is an immutable, date-ascending weekly price series.
type Journal struct {
	weeks []Week
}

// Load reads a journal CSV with a "week_end,price_avg_usd"-style header.
func Load(path string) (*Journal, error) {
	f, err := os.Open(path) //nolint:gosec // the journal path comes from our own config, not user input
	if err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	const columns = 2 // week_end, price_avg_usd
	if len(rows) < columns {
		return nil, fmt.Errorf("journal: %s has no data rows", path)
	}
	weeks := make([]Week, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) != columns {
			return nil, fmt.Errorf("journal: malformed row %v", row)
		}
		end, parseErr := time.Parse(time.DateOnly, row[0])
		if parseErr != nil {
			return nil, fmt.Errorf("journal: %w", parseErr)
		}
		price, priceErr := parseMicroUSD(row[1])
		if priceErr != nil {
			return nil, fmt.Errorf("journal: row %v: %w", row, priceErr)
		}
		weeks = append(weeks, Week{End: end, PriceMic: price})
	}
	if !sort.SliceIsSorted(weeks, func(i, j int) bool { return weeks[i].End.Before(weeks[j].End) }) {
		return nil, fmt.Errorf("journal: %s is not date-ascending", path)
	}
	return &Journal{weeks: weeks}, nil
}

// MaxSince returns the maximum weekly-average price (micro-USD) over all weeks
// ending on or after the given date, and the ISO date of that week.
func (j *Journal) MaxSince(since time.Time) (uint64, string, error) {
	var (
		best     uint64
		bestWeek time.Time
		found    bool
	)
	for _, w := range j.weeks {
		if w.End.Before(since) {
			continue
		}
		if !found || w.PriceMic > best {
			best, bestWeek, found = w.PriceMic, w.End, true
		}
	}
	if !found {
		return 0, "", fmt.Errorf("journal: no weeks at or after %s", since.Format(time.DateOnly))
	}
	return best, bestWeek.Format(time.DateOnly), nil
}

// parseMicroUSD converts a decimal string to micro-USD, truncating digits
// beyond the sixth decimal place. Truncation is the published rule; no floats.
func parseMicroUSD(s string) (uint64, error) {
	const microDigits = 6
	intPart, fracPart, _ := strings.Cut(strings.TrimSpace(s), ".")
	whole, err := strconv.ParseUint(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad price %q: %w", s, err)
	}
	if len(fracPart) > microDigits {
		fracPart = fracPart[:microDigits]
	}
	fracPart += strings.Repeat("0", microDigits-len(fracPart))
	frac, err := strconv.ParseUint(fracPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad price %q: %w", s, err)
	}
	const micro = 1_000_000
	return whole*micro + frac, nil
}
