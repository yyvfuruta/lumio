// Package core owns Lumio's entities, use cases and outbound ports.
// It depends only on the standard library, never on HTTP or database drivers.
package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"
	_ "time/tzdata"
)

type InputError struct{ Message string }

func (e *InputError) Error() string { return e.Message }
func invalid(message string) error  { return &InputError{Message: message} }

var ErrNotFound = errors.New("photo not found")
var ErrArchiveNotFound = errors.New("archive not found or expired")

type Photo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Size        int64     `json:"size"`
	ThumbSize   int64     `json:"-"`
	PreviewSize int64     `json:"-"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	AddedAt     time.Time `json:"addedAt"`
	Favorite    bool      `json:"favorite"`
	Duration    float64   `json:"duration,omitempty"`
}
type Month struct {
	Month string `json:"month"`
	Count int64  `json:"count"`
}
type Stats struct {
	Photos           int64 `json:"photos"`
	Favorites        int64 `json:"favorites"`
	OriginalBytes    int64 `json:"originalBytes"`
	ThumbnailBytes   int64 `json:"thumbnailBytes"`
	PreviewBytes     int64 `json:"previewBytes"`
	PendingDeletions int64 `json:"pendingDeletions"`
}
type Filter struct {
	Query       string
	Favorite    bool
	Before      time.Time
	BeforeID    string
	From, Until *time.Time
}
type ListInput struct {
	Query, Cursor, From, Until string
	Favorite                   bool
}
type Page struct {
	Photos     []Photo `json:"photos"`
	NextCursor string  `json:"nextCursor"`
}
type Archive struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expiresAt"`
	Photos    int       `json:"photos"`
}

// Repository coordinates metadata transactions. Save/Collect keep the per-photo
// lock across the object callback, so replicas cannot race deduplication or GC.
// Callbacks run once and must not be retried inside a database transaction.
type Repository interface {
	List(context.Context, Filter) ([]Photo, error)
	Get(context.Context, string) (Photo, error)
	SetFavorite(context.Context, string, bool) (Photo, error)
	Delete(context.Context, string) error
	Save(context.Context, Photo, func(context.Context) error) (Photo, bool, error)
	Collect(context.Context, func(context.Context, string) error) (bool, error)
	Timeline(context.Context, string, string, bool) ([]Month, error)
	Stats(context.Context) (Stats, error)
	Select(context.Context, []string) ([]Photo, error)
	CreateArchive(context.Context, string, []string) (time.Time, error)
	ArchiveIDs(context.Context, string) ([]string, error)
}

// Objects treats local volumes and cloud buckets as replaceable backing services.
type Objects interface {
	Open(context.Context, string, int64) (io.ReadCloser, error)
	Put(context.Context, string, string, io.Reader) error
	Delete(context.Context, string) error
	Accessible(context.Context) (bool, error)
}
type Library struct {
	repo      Repository
	objects   Objects
	processor Processor
}

func NewLibrary(repo Repository, objects Objects, processor Processor) *Library {
	return &Library{repo: repo, objects: objects, processor: processor}
}
func ValidID(id string) bool {
	if len(id) != 64 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func PhotoRange(fromRaw, untilRaw string) (from, until *time.Time, err error) {
	parse := func(raw string) (*time.Time, error) {
		if raw == "" {
			return nil, nil
		}
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, invalid("from and until must be RFC 3339 timestamps")
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
		err = invalid("until must be later than from")
	}
	return
}
func (l *Library) List(ctx context.Context, in ListInput) (Page, error) {
	if len(in.Query) > 200 {
		return Page{}, invalid("search query is too long")
	}
	from, until, err := PhotoRange(in.From, in.Until)
	if err != nil {
		return Page{}, err
	}
	f := Filter{Query: in.Query, Favorite: in.Favorite, From: from, Until: until, Before: time.Now().Add(24 * time.Hour), BeforeID: strings.Repeat("f", 64)}
	if in.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(in.Cursor)
		parts := strings.Split(string(raw), "|")
		if err != nil || len(parts) != 2 || !ValidID(parts[1]) {
			return Page{}, invalid("invalid cursor")
		}
		f.Before, err = time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			return Page{}, invalid("invalid cursor")
		}
		f.BeforeID = parts[1]
	}
	photos, err := l.repo.List(ctx, f)
	if err != nil {
		return Page{}, err
	}
	page := Page{Photos: photos}
	if page.Photos == nil {
		page.Photos = []Photo{}
	}
	if len(photos) > 60 {
		page.Photos = photos[:60]
		p := photos[59]
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(p.AddedAt.Format(time.RFC3339Nano) + "|" + p.ID))
	}
	return page, nil
}
func (l *Library) Get(ctx context.Context, id string) (Photo, error) {
	if !ValidID(id) {
		return Photo{}, ErrNotFound
	}
	return l.repo.Get(ctx, id)
}
func (l *Library) Favorite(ctx context.Context, id string, favorite bool) (Photo, error) {
	if !ValidID(id) {
		return Photo{}, ErrNotFound
	}
	return l.repo.SetFavorite(ctx, id, favorite)
}
func (l *Library) Delete(ctx context.Context, id string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	return l.repo.Delete(ctx, id)
}
func (l *Library) Import(ctx context.Context, p Photo, original, thumb, preview io.Reader) (Photo, bool, error) {
	return l.repo.Save(ctx, p, func(ctx context.Context) error {
		if err := l.objects.Put(ctx, p.ID+"/original", p.Type, original); err != nil {
			return err
		}
		if err := l.objects.Put(ctx, p.ID+"/thumb.jpg", "image/jpeg", thumb); err != nil {
			return err
		}
		if p.PreviewSize > 0 {
			return l.objects.Put(ctx, p.ID+"/preview.jpg", "image/jpeg", preview)
		}
		return nil
	})
}
func (l *Library) Collect(ctx context.Context) (bool, error) {
	return l.repo.Collect(ctx, func(ctx context.Context, id string) error {
		for _, key := range []string{"original", "thumb.jpg", "preview.jpg"} {
			if err := l.objects.Delete(ctx, id+"/"+key); err != nil {
				return err
			}
		}
		return nil
	})
}
func (l *Library) Timeline(ctx context.Context, zone, query string, favorite bool) ([]Month, error) {
	if zone == "" {
		zone = "UTC"
	}
	if len(zone) > 100 || zone == "Local" {
		return nil, invalid("invalid timezone")
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return nil, invalid("invalid timezone")
	}
	if len(query) > 200 {
		return nil, invalid("search query is too long")
	}
	return l.repo.Timeline(ctx, zone, query, favorite)
}
func (l *Library) Stats(ctx context.Context) (Stats, error)     { return l.repo.Stats(ctx) }
func (l *Library) Accessible(ctx context.Context) (bool, error) { return l.objects.Accessible(ctx) }
func (l *Library) Select(ctx context.Context, ids []string) ([]Photo, error) {
	return l.repo.Select(ctx, ids)
}
func (l *Library) CreateArchive(ctx context.Context, input []string) (Archive, error) {
	if len(input) == 0 || len(input) > 1000 {
		return Archive{}, invalid("select between 1 and 1000 photos")
	}
	ids := make([]string, 0, len(input))
	seen := map[string]bool{}
	for _, id := range input {
		if !ValidID(id) {
			return Archive{}, invalid("invalid photo ID")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if _, err := l.repo.Select(ctx, ids); err != nil {
		return Archive{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Archive{}, err
	}
	id := hex.EncodeToString(random[:])
	expires, err := l.repo.CreateArchive(ctx, id, ids)
	if err != nil {
		return Archive{}, err
	}
	return Archive{ID: id, ExpiresAt: expires.UTC(), Photos: len(ids)}, nil
}
func (l *Library) ArchivePhotos(ctx context.Context, id string) ([]Photo, error) {
	if len(id) != 32 {
		return nil, ErrArchiveNotFound
	}
	if _, err := hex.DecodeString(id); err != nil {
		return nil, ErrArchiveNotFound
	}
	ids, err := l.repo.ArchiveIDs(ctx, id)
	if err != nil {
		return nil, err
	}
	return l.repo.Select(ctx, ids)
}
func (l *Library) Open(ctx context.Context, key string, offset int64) (io.ReadCloser, error) {
	return l.objects.Open(ctx, key, offset)
}
