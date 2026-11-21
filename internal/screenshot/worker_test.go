package screenshot_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/db/dbtest"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/project"
	"github.com/suwahadi/project-ops-manager/internal/screenshot"
)

type fakeShooter struct {
	err  error
	data []byte
}

func (f *fakeShooter) Capture(context.Context, string) ([]byte, error) { return f.data, f.err }

type memFiles struct {
	files   map[string][]byte
	removed []string
}

func (m *memFiles) WriteFile(rel string, data []byte) error { m.files[rel] = data; return nil }
func (m *memFiles) Remove(rels ...string)                   { m.removed = append(m.removed, rels...) }

func TestWorker(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	q := sqlc.New(pool)
	clock := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	projects := project.NewService(pool, now)
	url := "https://example.com/"
	id, err := projects.Create(ctx, project.Input{Name: "Site", URL: &url, Brief: project.BriefTemplate})
	if err != nil {
		t.Fatal(err)
	}
	shooter := &fakeShooter{data: []byte("RIFF....WEBP")}
	files := &memFiles{files: map[string][]byte{}}
	n := 0
	w := screenshot.NewWorker(pool, shooter, files, slog.New(slog.NewTextHandler(io.Discard, nil)), now, func() (string, error) { n++; return []string{"a", "b", "c"}[n-1], nil })

	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	sc, _ := q.GetProjectScreenshot(ctx, id)
	if sc.Status != "done" || sc.StoredPath == nil || *sc.StoredPath != "screenshots/2026/11/a.webp" || sc.CapturedAt == nil {
		t.Fatalf("after success: %+v", sc)
	}

	clock = clock.Add(time.Minute)
	if err := projects.RequestScreenshot(ctx, id); err != nil {
		t.Fatal(err)
	}
	shooter.err = errors.New("the page did not finish loading within 30 seconds")
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	sc, _ = q.GetProjectScreenshot(ctx, id)
	if sc.Status != "failed" || sc.Error == nil || sc.StoredPath == nil || *sc.StoredPath != "screenshots/2026/11/a.webp" {
		t.Fatalf("failure must keep the previous image: %+v", sc)
	}

	clock = clock.Add(time.Minute)
	_ = projects.RequestScreenshot(ctx, id)
	shooter.err = nil
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	sc, _ = q.GetProjectScreenshot(ctx, id)
	if *sc.StoredPath != "screenshots/2026/11/b.webp" || len(files.removed) != 1 || files.removed[0] != "screenshots/2026/11/a.webp" {
		t.Fatalf("new capture should replace and remove the old file: %+v %v", sc, files.removed)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(files.files) != 2 {
		t.Fatal("no pending work should mean no new capture")
	}
}
