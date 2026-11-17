package app

import (
	"errors"
	"net/http"
	"time"
	_ "time/tzdata"
)

type timelineMonth struct {
	Month string `json:"month"`
	Count int64  `json:"count"`
}

func photoRange(fromRaw, untilRaw string) (from, until *time.Time, err error) {
	parse := func(raw string) (*time.Time, error) {
		if raw == "" {
			return nil, nil
		}
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, errors.New("from and until must be RFC 3339 timestamps")
		}
		return &t, nil
	}
	if from, err = parse(fromRaw); err != nil {
		return
	}
	if until, err = parse(untilRaw); err != nil {
		return
	}
	if from != nil && until != nil && !until.After(*from) {
		err = errors.New("until must be later than from")
	}
	return
}

func (a *App) timeline(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	zone := q.Get("tz")
	if zone == "" {
		zone = "UTC"
	}
	if len(zone) > 100 || zone == "Local" {
		problem(w, 400, "invalid timezone")
		return
	}
	if _, err := time.LoadLocation(zone); err != nil {
		problem(w, 400, "invalid timezone")
		return
	}
	search := q.Get("q")
	if len(search) > 200 {
		problem(w, 400, "search query is too long")
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT to_char(added_at AT TIME ZONE $1,'YYYY-MM'),count(*)
	 FROM lumio_photos WHERE deleted_at IS NULL AND (NOT $2::boolean OR favorite)
	 AND strpos(lower(name),lower($3))>0 GROUP BY 1 ORDER BY 1 DESC LIMIT 1200`, zone, q.Get("favorite") == "true", search)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	months := make([]timelineMonth, 0)
	for rows.Next() {
		var m timelineMonth
		if err := rows.Scan(&m.Month, &m.Count); err != nil {
			internal(w, err)
			return
		}
		months = append(months, m)
	}
	if err := rows.Err(); err != nil {
		internal(w, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"months": months})
}
