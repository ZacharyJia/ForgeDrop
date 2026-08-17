package db

import (
	"context"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := Migrate(context.Background(), d.SQL); err != nil {
		t.Fatal(err)
	}
	return NewStore(d.SQL)
}

func TestUpsertPreviewEnvRevivesSoftDeletedEnv(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t)

	repo, err := store.CreateRepo(ctx, "acme/demo", "secret")
	if err != nil {
		t.Fatal(err)
	}
	app, err := store.CreateApp(ctx, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}

	pr := 42
	env, err := store.UpsertPreviewEnv(ctx, app.ID, *repo, &pr, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SoftDeleteEnv(ctx, env.ID); err != nil {
		t.Fatal(err)
	}

	// Re-creating the same PR env must not fail on the table-level UNIQUE
	// constraint left behind by the soft-deleted row.
	revived, err := store.UpsertPreviewEnv(ctx, app.ID, *repo, &pr, "")
	if err != nil {
		t.Fatalf("upsert after soft delete: %v", err)
	}
	if revived.ID != env.ID {
		t.Fatalf("expected soft-deleted env %s to be revived, got %s", env.ID, revived.ID)
	}
	if revived.DeletedAt != nil {
		t.Fatalf("expected deleted_at to be cleared, got %v", revived.DeletedAt)
	}

	// Upserting the now-active env again returns it unchanged.
	again, err := store.UpsertPreviewEnv(ctx, app.ID, *repo, &pr, "")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != env.ID {
		t.Fatalf("expected existing env %s, got %s", env.ID, again.ID)
	}
}

func TestUpsertPreviewEnvRevivesSoftDeletedChangeSetEnv(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t)

	repo, err := store.CreateRepo(ctx, "acme/demo", "secret")
	if err != nil {
		t.Fatal(err)
	}
	app, err := store.CreateApp(ctx, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}

	env, err := store.UpsertPreviewEnv(ctx, app.ID, *repo, nil, "feature-x")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SoftDeleteEnv(ctx, env.ID); err != nil {
		t.Fatal(err)
	}

	revived, err := store.UpsertPreviewEnv(ctx, app.ID, *repo, nil, "feature-x")
	if err != nil {
		t.Fatalf("upsert after soft delete: %v", err)
	}
	if revived.ID != env.ID {
		t.Fatalf("expected soft-deleted env %s to be revived, got %s", env.ID, revived.ID)
	}
}
