package channel

import (
	"context"
	"errors"
	"testing"
)

// memRepo is an in-memory Repository used to test service behavior.
type memRepo struct {
	items     map[string]Channel
	conflicts int // Number of Save calls that should report a conflict.
	events    []StatusEvent
}

func newMemRepo() *memRepo { return &memRepo{items: map[string]Channel{}} }

func (m *memRepo) Create(_ context.Context, c *Channel) error {
	c.ID = "ch-" + string(rune('a'+len(m.items)))
	c.Version = 1
	m.items[c.ID] = *c
	return nil
}

func (m *memRepo) Get(_ context.Context, owner, id string) (*Channel, error) {
	c, ok := m.items[id]
	if !ok || c.OwnerID != owner {
		return nil, ErrNotFound
	}
	return &c, nil
}

func (m *memRepo) List(_ context.Context, owner string, _ Page) ([]Channel, string, error) {
	var out []Channel
	for _, c := range m.items {
		if c.OwnerID == owner {
			out = append(out, c)
		}
	}
	return out, "", nil
}

func (m *memRepo) Save(_ context.Context, c *Channel, ev *StatusEvent) error {
	if m.conflicts > 0 {
		m.conflicts--
		return ErrConflict
	}
	stored := m.items[c.ID]
	if stored.Version != c.Version {
		return ErrConflict
	}
	c.Version++
	m.items[c.ID] = *c
	if ev != nil {
		m.events = append(m.events, *ev)
	}
	return nil
}

type fixedConnections int

func (f fixedConnections) CountVerified(context.Context, string, string) (int, error) {
	return int(f), nil
}

func TestServiceOwnershipBoundary(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo, fixedConnections(0))
	ctx := context.Background()
	c, err := svc.CreateDraft(ctx, "alice", "History", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, "bob", c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found for other owner, got %v", err)
	}
	name := "Stolen"
	if _, err := svc.UpdateSettings(ctx, "bob", c.ID, SettingsUpdate{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found updating other owner's channel, got %v", err)
	}
	if _, err := svc.ChangeStatus(ctx, "bob", c.ID, StatusArchived, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found changing status of other owner's channel, got %v", err)
	}
}

func TestServiceRetriesOnConflict(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo, fixedConnections(0))
	ctx := context.Background()
	c, _ := svc.CreateDraft(ctx, "alice", "History", "", "")

	repo.conflicts = 2
	name := "Old Maps"
	got, err := svc.UpdateSettings(ctx, "alice", c.ID, SettingsUpdate{Name: &name})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if got.Name != name {
		t.Fatalf("name not applied: %q", got.Name)
	}

	repo.conflicts = 3
	if _, err := svc.UpdateSettings(ctx, "alice", c.ID, SettingsUpdate{Name: &name}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict after exhausting retries, got %v", err)
	}
}

func TestServiceChangeStatusRecordsEvent(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo, fixedConnections(0))
	ctx := context.Background()
	c, _ := svc.CreateDraft(ctx, "alice", "History", "", "")
	if _, err := svc.ChangeStatus(ctx, "alice", c.ID, StatusArchived, "not needed"); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 || repo.events[0].To != StatusArchived {
		t.Fatalf("expected one archived event, got %+v", repo.events)
	}
}

func TestBulkChangeStatus(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo, fixedConnections(1))
	ctx := context.Background()
	a, _ := svc.CreateDraft(ctx, "alice", "History", "", "")
	stored := repo.items[a.ID]
	stored.Status = StatusActive
	repo.items[a.ID] = stored
	b, _ := svc.CreateDraft(ctx, "alice", "Space", "", "")

	res, err := svc.BulkChangeStatus(ctx, "alice", []string{a.ID, b.ID, "missing"}, StatusPaused, "")
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Err != nil || res[0].Status != StatusPaused {
		t.Errorf("expected first channel paused, got %+v", res[0])
	}
	var te *TransitionError
	if !errors.As(res[1].Err, &te) {
		t.Errorf("expected transition error for draft channel, got %v", res[1].Err)
	}
	if !errors.Is(res[2].Err, ErrNotFound) {
		t.Errorf("expected not found, got %v", res[2].Err)
	}

	if _, err := svc.BulkChangeStatus(ctx, "alice", []string{a.ID}, StatusArchived, ""); err == nil {
		t.Error("expected bulk archive to be rejected")
	}
	if _, err := svc.BulkChangeStatus(ctx, "alice", nil, StatusPaused, ""); err == nil {
		t.Error("expected empty bulk request to be rejected")
	}
}
