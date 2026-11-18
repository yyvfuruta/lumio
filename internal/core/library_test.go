package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type memoryRepo struct {
	Repository
	photos    []Photo
	filter    Filter
	duplicate bool
	saved     bool
}

func (r *memoryRepo) List(_ context.Context, f Filter) ([]Photo, error) {
	r.filter = f
	return r.photos, nil
}
func (r *memoryRepo) Save(ctx context.Context, p Photo, write func(context.Context) error) (Photo, bool, error) {
	if r.duplicate {
		return p, true, nil
	}
	if err := write(ctx); err != nil {
		return Photo{}, false, err
	}
	r.saved = true
	return p, false, nil
}

type memoryObjects struct {
	Objects
	keys []string
	fail string
}

func (s *memoryObjects) Put(_ context.Context, key, _ string, src io.Reader) error {
	s.keys = append(s.keys, key)
	if key == s.fail {
		return errors.New("storage offline")
	}
	_, err := io.Copy(io.Discard, src)
	return err
}
func TestListPaginationPreservesBounds(t *testing.T) {
	repo := &memoryRepo{}
	for i := 0; i < 61; i++ {
		repo.photos = append(repo.photos, Photo{ID: strings.Repeat("a", 64), AddedAt: time.Date(2020, 1, 1, 0, 0, 0, i, time.UTC)})
	}
	service := NewLibrary(repo, nil, nil)
	input := ListInput{Query: "trip", Favorite: true, From: "2019-01-01T00:00:00Z", Until: "2021-01-01T00:00:00Z"}
	page, err := service.List(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Photos) != 60 || page.NextCursor == "" {
		t.Fatal("missing page boundary")
	}
	input.Cursor = page.NextCursor
	repo.photos = nil
	page, err = service.List(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if repo.filter.Before.Nanosecond() != 59 || repo.filter.BeforeID != strings.Repeat("a", 64) || !repo.filter.Favorite || repo.filter.Query != "trip" || repo.filter.From == nil || repo.filter.Until == nil {
		t.Fatal("pagination lost boundary or filters", repo.filter)
	}
	if page.Photos == nil || page.NextCursor != "" {
		t.Fatal("empty page contract")
	}
}
func TestImportDeduplicationAndStorageFailure(t *testing.T) {
	id := strings.Repeat("b", 64)
	for _, test := range []struct {
		name       string
		duplicate  bool
		fail       string
		wantSaved  bool
		wantWrites int
	}{{"new", false, "", true, 3}, {"duplicate", true, "", false, 0}, {"failed preview", false, id + "/preview.jpg", false, 3}} {
		t.Run(test.name, func(t *testing.T) {
			repo := &memoryRepo{duplicate: test.duplicate}
			store := &memoryObjects{fail: test.fail}
			service := NewLibrary(repo, store, nil)
			_, duplicate, err := service.Import(context.Background(), Photo{ID: id, PreviewSize: 1}, strings.NewReader("original"), strings.NewReader("thumbnail"), strings.NewReader("preview"))
			if (err != nil) != (test.fail != "") || duplicate != test.duplicate || repo.saved != test.wantSaved || len(store.keys) != test.wantWrites {
				t.Fatal("incorrect transaction/deduplication behavior", err)
			}
		})
	}
}
func TestInvalidInputsDoNotReachPorts(t *testing.T) {
	service := NewLibrary(nil, nil, nil)
	ctx := context.Background()
	for _, input := range []ListInput{{Cursor: "invalid"}, {Query: strings.Repeat("x", 201)}, {From: "today"}, {From: "2020-01-01T00:00:00Z", Until: "2019-01-01T00:00:00Z"}} {
		_, err := service.List(ctx, input)
		var invalid *InputError
		if !errors.As(err, &invalid) {
			t.Fatalf("expected invalid input: %v", err)
		}
	}
	if _, err := service.Get(ctx, "invalid"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := service.Timeline(ctx, "Local", "", false); err == nil {
		t.Fatal("accepted process-local timezone")
	}
	if _, err := service.CreateArchive(ctx, []string{"bad"}); err == nil {
		t.Fatal("accepted bad archive")
	}
}

type memorySessions struct {
	Sessions
	attempts  int
	hash, tag []byte
	expires   time.Time
	revoked   []byte
}

func (s *memorySessions) Attempt(context.Context) (int, error) { s.attempts++; return s.attempts, nil }
func (s *memorySessions) SaveSession(_ context.Context, hash, tag []byte, expires time.Time) error {
	s.hash = append([]byte{}, hash...)
	s.tag = append([]byte{}, tag...)
	s.expires = expires
	return nil
}
func (s *memorySessions) Valid(_ context.Context, hash, tag []byte) (bool, error) {
	return string(hash) == string(s.hash) && string(tag) == string(s.tag), nil
}
func (s *memorySessions) Revoke(_ context.Context, hash []byte) error {
	s.revoked = append([]byte{}, hash...)
	return nil
}
func TestAccounts(t *testing.T) {
	sessions := &memorySessions{}
	accounts := NewAccounts(sessions, "password")
	ctx := context.Background()
	if _, _, err := accounts.Login(ctx, "wrong"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	token, expires, err := accounts.Login(ctx, "password")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(token))
	if len(token) != 64 || string(sessions.hash) != string(digest[:]) || expires.Before(time.Now().Add(6*24*time.Hour)) {
		t.Fatal("invalid credential persistence")
	}
	if !accounts.Authenticated(ctx, token) || NewAccounts(sessions, "changed").Authenticated(ctx, token) {
		t.Fatal("password/session isolation failed")
	}
	if err := accounts.Logout(ctx, token); err != nil || string(sessions.revoked) != string(digest[:]) {
		t.Fatal("wrong revocation")
	}
	sessions.attempts = 20
	if _, _, err := accounts.Login(ctx, "password"); !errors.Is(err, ErrThrottled) {
		t.Fatal(err)
	}
}
