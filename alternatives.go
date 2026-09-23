package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

type Difference struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// These are candidates for revising a query, never strict matches.
type NearbyAlternative struct {
	Profile
	Differences  []Difference `json:"differences"`
	Dates        []string     `json:"dates"`
	budgetGap    int64
	dateDistance int
}

func (a *App) nearbyAlternatives(q Query) []NearbyAlternative {
	result := []NearbyAlternative{}
	for _, p := range a.Profiles {
		if !slices.Contains(p.Categories, q.Category) {
			continue
		}
		n := NearbyAlternative{Profile: p, Differences: []Difference{}, Dates: []string{}}
		add := func(field, message string) { n.Differences = append(n.Differences, Difference{field, message}) }
		if p.City != q.City {
			add("city", fmt.Sprintf("Другой город: %s вместо %s. Выезд в ваш город не подтверждён.", p.City, q.City))
		}
		failed := profileFailures(p, q)
		if failed["budget"] {
			n.budgetGap = p.Price - *q.Budget
			add("budget_kzt", fmt.Sprintf("Цена от %s ₸ — на %s ₸ выше вашего бюджета. Итоговая стоимость может отличаться.", money(p.Price), money(n.budgetGap)))
		}
		if failed["format"] {
			add("event_type", fmt.Sprintf("Формат «%s» не указан. В каталоге: %s. Возможность работы на вашем мероприятии нужно уточнить.", q.Format, strings.Join(p.Formats, ", ")))
		}
		if failed["language"] {
			add("language", fmt.Sprintf("Язык «%s» не указан. В каталоге: %s.", q.Language, strings.Join(p.Languages, ", ")))
		}
		if failed["duration"] {
			add("duration_hours", fmt.Sprintf("Работа до %g ч, вы запросили %g ч.", *p.Hours, *q.Hours))
		}
		if failed["booked"] {
			add("event_date", fmt.Sprintf("Занят на выбранную дату %s.", q.Date))
			n.Dates = nearestDates(p, q.Date)
			n.dateDistance = 101 // Beyond the entire known calendar if no free day exists.
			if len(n.Dates) > 0 {
				selected, _ := time.Parse("2006-01-02", q.Date)
				nearest, _ := time.Parse("2006-01-02", n.Dates[0])
				n.dateDistance = int(nearest.Sub(selected).Abs().Hours() / 24)
			}
		}
		if len(n.Differences) > 0 {
			result = append(result, n)
		}
	}
	// Fewer mismatches, then same city, budget increase, date distance, price and ID.
	// No semantic score can hide a failed strict condition.
	sort.Slice(result, func(i, j int) bool {
		x, y := result[i], result[j]
		if len(x.Differences) != len(y.Differences) {
			return len(x.Differences) < len(y.Differences)
		}
		if (x.City == q.City) != (y.City == q.City) {
			return x.City == q.City
		}
		if x.budgetGap != y.budgetGap {
			return x.budgetGap < y.budgetGap
		}
		if x.dateDistance != y.dateDistance {
			return x.dateDistance < y.dateDistance
		}
		if x.Price != y.Price {
			return x.Price < y.Price
		}
		return x.ID < y.ID
	})
	return result[:min(3, len(result))]
}
