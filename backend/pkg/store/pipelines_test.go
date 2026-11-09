package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"crm/pkg/migrate"
	"crm/pkg/model"
	"crm/pkg/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pgFKViolation = "23503"

func wantPgCode(t *testing.T, name string, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Errorf("%s = %v, want SQLSTATE %s", name, err, code)
	}
}

func defaultPipeline(t *testing.T, s *Store, workspaceID string) model.Pipeline {
	t.Helper()
	ctx := context.Background()
	page, err := s.ListPipelines(ctx, workspaceID, model.ListParams{})
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("ListPipelines = %+v, %v", page, err)
	}
	p, err := s.GetPipeline(ctx, workspaceID, page.Items[0].ID)
	if err != nil {
		t.Fatalf("GetPipeline: %v", err)
	}
	return p
}

func newPipeline(t *testing.T, s *Store, workspaceID, actorID, name string) model.Pipeline {
	t.Helper()
	p, err := s.CreatePipeline(context.Background(), workspaceID, actorID, name)
	if err != nil {
		t.Fatalf("CreatePipeline(%q): %v", name, err)
	}
	return p
}

func newStage(t *testing.T, s *Store, workspaceID, actorID, pipelineID, name string, kind model.StageKind) model.PipelineStage {
	t.Helper()
	stage, err := s.CreateStage(context.Background(), workspaceID, actorID, pipelineID, model.StageInput{Name: name, Color: "#112233", Kind: kind})
	if err != nil {
		t.Fatalf("CreateStage(%q): %v", name, err)
	}
	return stage
}

// stageOfKind returns the first stage of a kind, by position.
func stageOfKind(t *testing.T, p model.Pipeline, kind model.StageKind) model.PipelineStage {
	t.Helper()
	for _, stage := range p.Stages {
		if stage.Kind == kind {
			return stage
		}
	}
	t.Fatalf("pipeline has no %s stage: %+v", kind, p.Stages)
	return model.PipelineStage{}
}

func stageNames(stages []model.PipelineStage) []string {
	names := make([]string, len(stages))
	for i, stage := range stages {
		names[i] = stage.Name
	}
	return names
}

// wantDensePositions fails unless positions run 0, 1, 2, ... in list order.
func wantDensePositions(t *testing.T, stages []model.PipelineStage) {
	t.Helper()
	for i, stage := range stages {
		if stage.Position != i {
			t.Errorf("stage %q at index %d has position %d", stage.Name, i, stage.Position)
		}
	}
}

func TestNewWorkspaceGetsADefaultPipeline(t *testing.T) {
	s := New(testdb.AppPool(t))
	w := newWorkspace(t, s, newUser(t, s))

	p := defaultPipeline(t, s, w.ID)
	if p.WorkspaceID != w.ID || p.Name == "" {
		t.Errorf("pipeline = %+v", p)
	}
	wantDensePositions(t, p.Stages)

	kinds := map[model.StageKind]int{}
	for _, stage := range p.Stages {
		kinds[stage.Kind]++
		if stage.PipelineID != p.ID || stage.WorkspaceID != w.ID || stage.Name == "" ||
			stage.Probability < 0 || stage.Probability > 100 || len(stage.Color) != 7 {
			t.Errorf("stage = %+v", stage)
		}
	}
	if kinds[model.StageOpen] < 1 || kinds[model.StageWon] != 1 || kinds[model.StageLost] != 1 {
		t.Errorf("stage kinds = %v", kinds)
	}
	if got := stageOfKind(t, p, model.StageWon).Probability; got != 100 {
		t.Errorf("won probability = %d", got)
	}
	if got := stageOfKind(t, p, model.StageLost).Probability; got != 0 {
		t.Errorf("lost probability = %d", got)
	}
}

// scratchStore migrates a throwaway schema and connects to it as both roles, so a
// test can sabotage the schema without touching what other packages are using.
func scratchStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	appURL, ownerURL := testdb.URLs(t)
	ctx := context.Background()
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + hex.EncodeToString(suffix)
	connect := func(url string) *pgxpool.Pool {
		cfg, err := pgxpool.ParseConfig(url)
		if err != nil {
			t.Fatal(err)
		}
		cfg.ConnConfig.RuntimeParams["search_path"] = schema
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	owner := connect(ownerURL)
	if _, err := owner.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	// Registered after the pool, so it runs before the pool closes.
	t.Cleanup(func() {
		if _, err := owner.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
	})
	if err := migrate.Apply(ctx, owner); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return New(connect(appURL)), owner
}

// The workspace and its pipeline commit together: if the pipeline cannot be
// created, the workspace does not exist either.
func TestCreateWorkspaceRollsBackWithItsDefaultPipeline(t *testing.T) {
	s, owner := scratchStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	if _, err := owner.Exec(ctx, `ALTER FUNCTION create_pipeline_with_default_stages RENAME TO sabotaged`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWorkspace(ctx, user.ID, "Half made"); err == nil {
		t.Fatal("CreateWorkspace succeeded without its default pipeline")
	}
	var workspaces, members int
	if err := owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM workspaces), (SELECT count(*) FROM workspace_members)`).Scan(&workspaces, &members); err != nil {
		t.Fatal(err)
	}
	if workspaces != 0 || members != 0 {
		t.Errorf("left behind %d workspaces and %d members", workspaces, members)
	}
}

func TestPipelineLifecycle(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)

	created := newPipeline(t, s, w.ID, owner.ID, "Renewals")
	if created.ID == "" || created.WorkspaceID != w.ID || created.Name != "Renewals" {
		t.Errorf("created = %+v", created)
	}
	// A new pipeline starts usable, with the same stages as the default one.
	if got, want := stageNames(created.Stages), stageNames(defaultPipeline(t, s, w.ID).Stages); !slices.Equal(got, want) {
		t.Errorf("stages = %q, want %q", got, want)
	}

	got, err := s.GetPipeline(ctx, w.ID, created.ID)
	if err != nil || got.Name != "Renewals" || len(got.Stages) != len(created.Stages) {
		t.Errorf("GetPipeline = %+v, %v", got, err)
	}

	renamed, err := s.RenamePipeline(ctx, w.ID, owner.ID, created.ID, "Upsells")
	if err != nil {
		t.Fatalf("RenamePipeline: %v", err)
	}
	if renamed.Name != "Upsells" || !renamed.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("renamed = %+v, created %+v", renamed, created)
	}

	page, err := s.ListPipelines(ctx, w.ID, model.ListParams{})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("list = %+v, %v", page, err)
	}
	if page.Items[1].Name != "Upsells" || page.Items[1].Stages != nil {
		t.Errorf("list items are ordered by age and carry no stages: %+v", page.Items)
	}

	if err := s.DeletePipeline(ctx, w.ID, owner.ID, created.ID); err != nil {
		t.Fatalf("DeletePipeline: %v", err)
	}
	if _, err := s.GetPipeline(ctx, w.ID, created.ID); !errors.Is(err, ErrPipelineNotFound) {
		t.Errorf("get after delete = %v", err)
	}
	if _, err := s.RenamePipeline(ctx, w.ID, owner.ID, created.ID, "Zombie"); !errors.Is(err, ErrPipelineNotFound) {
		t.Errorf("rename after delete = %v", err)
	}
	if err := s.DeletePipeline(ctx, w.ID, owner.ID, created.ID); !errors.Is(err, ErrPipelineNotFound) {
		t.Errorf("second delete = %v", err)
	}
	if page, err := s.ListPipelines(ctx, w.ID, model.ListParams{Search: "Upsells"}); err != nil || len(page.Items) != 0 {
		t.Errorf("search after delete = %+v, %v", page, err)
	}
	// The row is kept, as a soft delete.
	var kept int
	if err := ownerPool(t).QueryRow(ctx, `SELECT count(*) FROM pipelines WHERE id = $1 AND deleted_at IS NOT NULL`, created.ID).Scan(&kept); err != nil || kept != 1 {
		t.Errorf("soft-deleted rows = %d, %v", kept, err)
	}
}

// A workspace keeps at least one pipeline, so a deal always has somewhere to start.
func TestLastPipelineCannotBeDeleted(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	first := defaultPipeline(t, s, w.ID)

	if err := s.DeletePipeline(ctx, w.ID, owner.ID, first.ID); !errors.Is(err, ErrLastPipeline) {
		t.Errorf("delete the only pipeline = %v, want ErrLastPipeline", err)
	}
	second := newPipeline(t, s, w.ID, owner.ID, "Second")
	if err := s.DeletePipeline(ctx, w.ID, owner.ID, first.ID); err != nil {
		t.Errorf("delete one of two: %v", err)
	}
	if err := s.DeletePipeline(ctx, w.ID, owner.ID, second.ID); !errors.Is(err, ErrLastPipeline) {
		t.Errorf("delete the one left = %v, want ErrLastPipeline", err)
	}
	if _, err := s.GetPipeline(ctx, w.ID, second.ID); err != nil {
		t.Errorf("the last pipeline is gone: %v", err)
	}
}

// holdWorkspace is holdPipeline for the workspace row, which every pipeline change
// takes first.
func holdWorkspace(t *testing.T, owner *pgxpool.Pool, workspaceID, app string, racers int) (release func()) {
	t.Helper()
	ctx := context.Background()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err := tx.Exec(ctx, `SELECT 1 FROM workspaces WHERE id = $1 FOR NO KEY UPDATE`, workspaceID); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		waitQueued(t, owner, app, racers)
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// Two deletes of the only two pipelines both queue before either runs, so each would
// see the other still live if they did not run one at a time. Exactly one may win.
func TestConcurrentDeletesKeepOnePipeline(t *testing.T) {
	s, _, app := raceStore(t, 4)
	ctx := context.Background()
	owner := ownerPool(t)
	user := newUser(t, s)
	w := newWorkspace(t, s, user)
	ids := []string{defaultPipeline(t, s, w.ID).ID, newPipeline(t, s, w.ID, user.ID, "Second").ID}

	release := holdWorkspace(t, owner, w.ID, app, len(ids))
	var wg sync.WaitGroup
	errs := make([]error, len(ids))
	for i, id := range ids {
		wg.Go(func() { errs[i] = s.DeletePipeline(ctx, w.ID, user.ID, id) })
	}
	release()
	wg.Wait()

	won, refused := 0, 0
	for i, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.Is(err, ErrLastPipeline):
			refused++
		default:
			t.Errorf("delete %d: %v", i, err)
		}
	}
	if won != 1 || refused != 1 {
		t.Errorf("%d deletes won and %d were refused, want 1 and 1", won, refused)
	}
	if n := countRows(t, owner, `SELECT count(*) FROM pipelines WHERE workspace_id = $1 AND deleted_at IS NULL`, w.ID); n != 1 {
		t.Errorf("%d live pipelines, want 1", n)
	}
}

func TestPipelineMalformedAndUnknownIDsAreNotFound(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	p := defaultPipeline(t, s, w.ID)
	for _, id := range []string{unknownID, "not-a-uuid", ""} {
		if _, err := s.GetPipeline(ctx, w.ID, id); !errors.Is(err, ErrPipelineNotFound) {
			t.Errorf("GetPipeline(%q) = %v", id, err)
		}
		if _, err := s.RenamePipeline(ctx, w.ID, owner.ID, id, "x"); !errors.Is(err, ErrPipelineNotFound) {
			t.Errorf("RenamePipeline(%q) = %v", id, err)
		}
		if err := s.DeletePipeline(ctx, w.ID, owner.ID, id); !errors.Is(err, ErrPipelineNotFound) {
			t.Errorf("DeletePipeline(%q) = %v", id, err)
		}
		if _, err := s.CreateStage(ctx, w.ID, owner.ID, id, model.StageInput{Name: "x", Color: "#000000", Kind: model.StageOpen}); !errors.Is(err, ErrPipelineNotFound) {
			t.Errorf("CreateStage(%q) = %v", id, err)
		}
		if _, err := s.UpdateStage(ctx, w.ID, owner.ID, p.ID, id, model.StagePatch{Name: strPtr("x")}); !errors.Is(err, ErrStageNotFound) {
			t.Errorf("UpdateStage(%q) = %v", id, err)
		}
		if err := s.DeleteStage(ctx, w.ID, owner.ID, p.ID, id); !errors.Is(err, ErrStageNotFound) {
			t.Errorf("DeleteStage(%q) = %v", id, err)
		}
	}
}

func TestPipelineListSearchAndStableOrder(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	for _, name := range []string{"Alpha", "Beta 100%", "Gamma"} {
		newPipeline(t, s, w.ID, owner.ID, name)
	}
	first, err := s.ListPipelines(ctx, w.ID, model.ListParams{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ListPipelines(ctx, w.ID, model.ListParams{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || second.HasMore || len(first.Items) != 3 || len(second.Items) != 1 {
		t.Errorf("pages = %+v then %+v", first, second)
	}
	if second.Items[0].Name != "Gamma" {
		t.Errorf("last = %+v", second.Items[0])
	}
	found, err := s.ListPipelines(ctx, w.ID, model.ListParams{Search: "%"})
	if err != nil || len(found.Items) != 1 || found.Items[0].Name != "Beta 100%" {
		t.Errorf("search for a literal percent = %+v, %v", found, err)
	}
}

func TestPipelineRoleLimits(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	admin := addMember(t, s, w, owner, model.RoleAdmin)
	member := addMember(t, s, w, owner, model.RoleMember)
	outsider := newUser(t, s)
	p := defaultPipeline(t, s, w.ID)
	open := stageOfKind(t, p, model.StageOpen)

	attempts := func(actor string) map[string]func() error {
		return map[string]func() error{
			"create pipeline": func() error { _, err := s.CreatePipeline(ctx, w.ID, actor, "x"); return err },
			"rename pipeline": func() error { _, err := s.RenamePipeline(ctx, w.ID, actor, p.ID, "x"); return err },
			"delete pipeline": func() error { return s.DeletePipeline(ctx, w.ID, actor, p.ID) },
			"create stage": func() error {
				_, err := s.CreateStage(ctx, w.ID, actor, p.ID, model.StageInput{Name: "x", Color: "#000000", Kind: model.StageOpen})
				return err
			},
			"update stage": func() error {
				_, err := s.UpdateStage(ctx, w.ID, actor, p.ID, open.ID, model.StagePatch{Name: strPtr("x")})
				return err
			},
			"reorder stages": func() error {
				_, err := s.ReorderStages(ctx, w.ID, actor, p.ID, []string{open.ID})
				return err
			},
			"delete stage": func() error { return s.DeleteStage(ctx, w.ID, actor, p.ID, open.ID) },
		}
	}
	for name, attempt := range attempts(member.ID) {
		if err := attempt(); !errors.Is(err, ErrWorkspaceForbidden) {
			t.Errorf("member: %s = %v, want forbidden", name, err)
		}
	}
	for name, attempt := range attempts(outsider.ID) {
		if err := attempt(); !errors.Is(err, ErrWorkspaceNotFound) {
			t.Errorf("outsider: %s = %v, want not found", name, err)
		}
	}
	// Members still read.
	if _, err := s.GetPipeline(ctx, w.ID, p.ID); err != nil {
		t.Errorf("read: %v", err)
	}
	// An admin manages pipelines as an owner does.
	extra := newPipeline(t, s, w.ID, admin.ID, "By the admin")
	if _, err := s.RenamePipeline(ctx, w.ID, admin.ID, extra.ID, "Renamed"); err != nil {
		t.Errorf("admin rename: %v", err)
	}
	if _, err := s.CreateStage(ctx, w.ID, admin.ID, extra.ID, model.StageInput{Name: "Extra", Color: "#000000", Kind: model.StageOpen}); err != nil {
		t.Errorf("admin create stage: %v", err)
	}
	if err := s.DeletePipeline(ctx, w.ID, admin.ID, extra.ID); err != nil {
		t.Errorf("admin delete: %v", err)
	}
}

func TestStageLifecycleKeepsPositionsDense(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	p := newPipeline(t, s, w.ID, owner.ID, "Ordered")
	base := stageNames(p.Stages)

	read := func() []model.PipelineStage {
		t.Helper()
		got, err := s.GetPipeline(ctx, w.ID, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		wantDensePositions(t, got.Stages)
		return got.Stages
	}

	// No position means the end.
	last, err := s.CreateStage(ctx, w.ID, owner.ID, p.ID, model.StageInput{Name: "Last", Color: "#aabbcc", Probability: 40, Kind: model.StageOpen})
	if err != nil {
		t.Fatalf("CreateStage: %v", err)
	}
	if last.Position != len(base) || last.Color != "#aabbcc" || last.Probability != 40 || last.Kind != model.StageOpen {
		t.Errorf("last = %+v", last)
	}

	// A position pushes the rest along.
	zero := 0
	first, err := s.CreateStage(ctx, w.ID, owner.ID, p.ID, model.StageInput{Name: "First", Color: "#000000", Kind: model.StageOpen, Position: &zero})
	if err != nil || first.Position != 0 {
		t.Fatalf("CreateStage at 0 = %+v, %v", first, err)
	}
	if got := stageNames(read()); got[0] != "First" || got[len(got)-1] != "Last" {
		t.Errorf("after insert at 0: %q", got)
	}

	// A position beyond the end means the end: the board may have shrunk meanwhile.
	beyond := 1000
	far, err := s.CreateStage(ctx, w.ID, owner.ID, p.ID, model.StageInput{Name: "Far", Color: "#000000", Kind: model.StageWon, Position: &beyond})
	if err != nil || far.Position != len(base)+2 {
		t.Fatalf("CreateStage beyond the end = %+v, %v", far, err)
	}

	// An update changes only the fields it names, and not the order.
	before := read()
	kindWon := model.StageWon
	updated, err := s.UpdateStage(ctx, w.ID, owner.ID, p.ID, last.ID, model.StagePatch{Name: strPtr("Renamed"), Kind: &kindWon})
	if err != nil {
		t.Fatalf("UpdateStage: %v", err)
	}
	if updated.Name != "Renamed" || updated.Kind != model.StageWon || updated.Color != "#aabbcc" || updated.Probability != 40 ||
		updated.Position != last.Position+1 || !updated.UpdatedAt.After(last.UpdatedAt) {
		t.Errorf("updated = %+v", updated)
	}
	if got := read(); len(got) != len(before) {
		t.Errorf("update changed the stage count: %d then %d", len(before), len(got))
	}

	// Deleting closes the gap, and frees the position for a new stage.
	if err := s.DeleteStage(ctx, w.ID, owner.ID, p.ID, first.ID); err != nil {
		t.Fatalf("DeleteStage: %v", err)
	}
	if got := stageNames(read()); slices.Contains(got, "First") || got[0] != base[0] {
		t.Errorf("after delete: %q", got)
	}
	if _, err := s.UpdateStage(ctx, w.ID, owner.ID, p.ID, first.ID, model.StagePatch{Name: strPtr("Zombie")}); !errors.Is(err, ErrStageNotFound) {
		t.Errorf("update after delete = %v", err)
	}
	if err := s.DeleteStage(ctx, w.ID, owner.ID, p.ID, first.ID); !errors.Is(err, ErrStageNotFound) {
		t.Errorf("second delete = %v", err)
	}
	// The row is kept, as a soft delete, and leaves the ordering.
	var kept int
	if err := ownerPool(t).QueryRow(ctx, `SELECT count(*) FROM pipeline_stages WHERE id = $1 AND deleted_at IS NOT NULL AND position IS NULL`, first.ID).Scan(&kept); err != nil || kept != 1 {
		t.Errorf("soft-deleted rows = %d, %v", kept, err)
	}
}

func TestPipelineKeepsAtLeastOneOpenStage(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	p := newPipeline(t, s, w.ID, owner.ID, "One open left")

	opens := []model.PipelineStage{}
	for _, stage := range p.Stages {
		if stage.Kind == model.StageOpen {
			opens = append(opens, stage)
		}
	}
	for _, stage := range opens[1:] {
		if err := s.DeleteStage(ctx, w.ID, owner.ID, p.ID, stage.ID); err != nil {
			t.Fatalf("delete a spare open stage: %v", err)
		}
	}
	last := opens[0]
	if err := s.DeleteStage(ctx, w.ID, owner.ID, p.ID, last.ID); !errors.Is(err, ErrLastOpenStage) {
		t.Errorf("delete the last open stage = %v", err)
	}
	won := model.StageWon
	if _, err := s.UpdateStage(ctx, w.ID, owner.ID, p.ID, last.ID, model.StagePatch{Kind: &won}); !errors.Is(err, ErrLastOpenStage) {
		t.Errorf("close the last open stage = %v", err)
	}
	// Renaming it, or keeping its kind, is fine, and other kinds can always go.
	open := model.StageOpen
	if _, err := s.UpdateStage(ctx, w.ID, owner.ID, p.ID, last.ID, model.StagePatch{Name: strPtr("Still open"), Kind: &open}); err != nil {
		t.Errorf("rename the last open stage: %v", err)
	}
	if err := s.DeleteStage(ctx, w.ID, owner.ID, p.ID, stageOfKind(t, p, model.StageLost).ID); err != nil {
		t.Errorf("delete a lost stage: %v", err)
	}
}

// Two concurrent deletions of the two remaining open stages must not both pass the
// check. The pipeline lock makes the second see the first.
func TestLastOpenStageHoldsUnderConcurrency(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	for round := range 10 {
		p := newPipeline(t, s, w.ID, owner.ID, "Race")
		var opens []string
		for _, stage := range p.Stages {
			if stage.Kind == model.StageOpen {
				opens = append(opens, stage.ID)
			}
		}
		for _, id := range opens[2:] {
			if err := s.DeleteStage(ctx, w.ID, owner.ID, p.ID, id); err != nil {
				t.Fatal(err)
			}
		}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, id := range opens[:2] {
			wg.Go(func() { errs[i] = s.DeleteStage(ctx, w.ID, owner.ID, p.ID, id) })
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil && !errors.Is(err, ErrLastOpenStage) {
				t.Fatalf("round %d: %v", round, err)
			}
		}
		got, err := s.GetPipeline(ctx, w.ID, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		wantDensePositions(t, got.Stages)
		if stageOfKind(t, got, model.StageOpen).ID == "" {
			t.Fatalf("round %d: no open stage left", round)
		}
	}
}

func TestReorderStages(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	p := newPipeline(t, s, w.ID, owner.ID, "Reorder")
	other := newPipeline(t, s, w.ID, owner.ID, "Other")

	ids := make([]string, len(p.Stages))
	for i, stage := range p.Stages {
		ids[i] = stage.ID
	}
	reversed := slices.Clone(ids)
	slices.Reverse(reversed)

	got, err := s.ReorderStages(ctx, w.ID, owner.ID, p.ID, reversed)
	if err != nil {
		t.Fatalf("ReorderStages: %v", err)
	}
	wantDensePositions(t, got)
	for i, stage := range got {
		if stage.ID != reversed[i] {
			t.Errorf("index %d = %s, want %s", i, stage.ID, reversed[i])
		}
	}
	if !got[0].UpdatedAt.After(p.Stages[len(p.Stages)-1].UpdatedAt) {
		t.Error("reordering did not advance updated_at")
	}

	// The list must name every live stage exactly once, so that a client working
	// from a stale board is told, rather than silently scrambling the order.
	for _, tt := range []struct {
		name string
		ids  []string
	}{
		{"empty", nil},
		{"missing one", reversed[1:]},
		{"duplicate", append(slices.Clone(reversed[1:]), reversed[1])},
		{"unknown", append(slices.Clone(reversed[1:]), unknownID)},
		{"malformed", append(slices.Clone(reversed[1:]), "not-a-uuid")},
		{"from another pipeline", append(slices.Clone(reversed[1:]), other.Stages[0].ID)},
		{"one too many", append(slices.Clone(reversed), other.Stages[0].ID)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.ReorderStages(ctx, w.ID, owner.ID, p.ID, tt.ids); !errors.Is(err, ErrStageOrderMismatch) {
				t.Errorf("err = %v", err)
			}
		})
	}
	after, err := s.GetPipeline(ctx, w.ID, p.ID)
	if err != nil || after.Stages[0].ID != reversed[0] {
		t.Errorf("a refused reorder changed the order: %+v, %v", after.Stages, err)
	}
}

// Creates, reorders, and deletes on one pipeline all read the board before any of them
// can act, and queue in a known order behind the workspace lock. Each must then work
// from what the one before it left: positions stay dense, every stage lands where it
// was asked to, and a reorder made from a board that has since changed is refused.
func TestConcurrentStageWritesKeepPositionsDense(t *testing.T) {
	const racers = 8
	s, _, app := raceStore(t, racers+2)
	ctx := context.Background()
	owner := ownerPool(t)
	user := newUser(t, s)
	w := newWorkspace(t, s, user)
	p := newPipeline(t, s, w.ID, user.ID, "Busy")
	spares := make([]model.PipelineStage, 3)
	for i := range spares {
		spares[i] = newStage(t, s, w.ID, user.ID, p.ID, fmt.Sprintf("Spare %d", i), model.StageOpen)
	}
	before, err := s.GetPipeline(ctx, w.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	reversed := make([]string, len(before.Stages))
	board := make([]string, len(before.Stages)) // The names the board should end with.
	for i, stage := range before.Stages {
		reversed[len(reversed)-1-i] = stage.ID
		board[len(board)-1-i] = stage.Name
	}

	create := func(name string, position *int) func() error {
		return func() error {
			_, err := s.CreateStage(ctx, w.ID, user.ID, p.ID, model.StageInput{Name: name, Color: "#000000", Kind: model.StageOpen, Position: position})
			return err
		}
	}
	reorder := func() error {
		_, err := s.ReorderStages(ctx, w.ID, user.ID, p.ID, reversed)
		return err
	}
	remove := func(stage model.PipelineStage) func() error {
		return func() error { return s.DeleteStage(ctx, w.ID, user.ID, p.ID, stage.ID) }
	}
	without := func(names []string, name string) []string {
		return slices.DeleteFunc(names, func(n string) bool { return n == name })
	}
	steps := []struct {
		run     func() error
		want    error
		applied func([]string) []string
	}{
		{reorder, nil, func(b []string) []string { return b }},
		{create("First", intPtr(0)), nil, func(b []string) []string { return slices.Insert(b, 0, "First") }},
		{remove(spares[0]), nil, func(b []string) []string { return without(b, spares[0].Name) }},
		{create("Last", nil), nil, func(b []string) []string { return append(b, "Last") }},
		// Made from the board as it was before the race: it no longer names every stage.
		{reorder, ErrStageOrderMismatch, func(b []string) []string { return b }},
		{remove(spares[1]), nil, func(b []string) []string { return without(b, spares[1].Name) }},
		{create("Third", intPtr(2)), nil, func(b []string) []string { return slices.Insert(b, 2, "Third") }},
		{remove(spares[2]), nil, func(b []string) []string { return without(b, spares[2].Name) }},
	}

	release := holdWorkspace(t, owner, w.ID, app, len(steps))
	var wg sync.WaitGroup
	errs := make([]error, len(steps))
	for i, step := range steps {
		wg.Go(func() { errs[i] = step.run() })
		// Queue each behind the last, so the order they run in is known.
		waitQueued(t, owner, app, i+1)
	}
	release()
	wg.Wait()
	for i, step := range steps {
		if !errors.Is(errs[i], step.want) {
			t.Errorf("step %d = %v, want %v", i, errs[i], step.want)
		}
		board = step.applied(board)
	}

	got, err := s.GetPipeline(ctx, w.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantDensePositions(t, got.Stages)
	if names := stageNames(got.Stages); !slices.Equal(names, board) {
		t.Errorf("stages = %q, want %q", names, board)
	}
}

func TestPipelineStoreFiltersWithoutRLS(t *testing.T) {
	tn := newTenants(t)
	ctx := context.Background()
	rls := New(tn.pool)
	ofB := defaultPipeline(t, rls, tn.b.ID)
	stageOfB := ofB.Stages[0]
	ofA := defaultPipeline(t, rls, tn.a.ID)
	s := New(ownerPool(t)) // A superuser: only the store's own filters apply.

	if _, err := s.GetPipeline(ctx, tn.a.ID, ofB.ID); !errors.Is(err, ErrPipelineNotFound) {
		t.Errorf("get = %v", err)
	}
	if _, err := s.RenamePipeline(ctx, tn.a.ID, tn.ownerA.ID, ofB.ID, "pwned"); !errors.Is(err, ErrPipelineNotFound) {
		t.Errorf("rename = %v", err)
	}
	if err := s.DeletePipeline(ctx, tn.a.ID, tn.ownerA.ID, ofB.ID); !errors.Is(err, ErrPipelineNotFound) {
		t.Errorf("delete = %v", err)
	}
	if _, err := s.UpdateStage(ctx, tn.a.ID, tn.ownerA.ID, ofB.ID, stageOfB.ID, model.StagePatch{Name: strPtr("pwned")}); !errors.Is(err, ErrPipelineNotFound) {
		t.Errorf("update stage through B's pipeline = %v", err)
	}
	if _, err := s.UpdateStage(ctx, tn.a.ID, tn.ownerA.ID, ofA.ID, stageOfB.ID, model.StagePatch{Name: strPtr("pwned")}); !errors.Is(err, ErrStageNotFound) {
		t.Errorf("update B's stage through A's pipeline = %v", err)
	}
	if err := s.DeleteStage(ctx, tn.a.ID, tn.ownerA.ID, ofA.ID, stageOfB.ID); !errors.Is(err, ErrStageNotFound) {
		t.Errorf("delete B's stage through A's pipeline = %v", err)
	}
	page, err := s.ListPipelines(ctx, tn.a.ID, model.ListParams{})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != ofA.ID {
		t.Errorf("list = %+v, %v", page, err)
	}
	survivor, err := rls.GetPipeline(ctx, tn.b.ID, ofB.ID)
	if err != nil || survivor.Name != ofB.Name || survivor.Stages[0].Name != stageOfB.Name {
		t.Errorf("B's pipeline after the attempts = %+v, %v", survivor, err)
	}
}

func TestStageFromAnotherPipelineIsNotFound(t *testing.T) {
	s := New(testdb.AppPool(t))
	ctx := context.Background()
	owner := newUser(t, s)
	w := newWorkspace(t, s, owner)
	a := newPipeline(t, s, w.ID, owner.ID, "A")
	b := newPipeline(t, s, w.ID, owner.ID, "B")
	if _, err := s.UpdateStage(ctx, w.ID, owner.ID, a.ID, b.Stages[0].ID, model.StagePatch{Name: strPtr("x")}); !errors.Is(err, ErrStageNotFound) {
		t.Errorf("update = %v", err)
	}
	if err := s.DeleteStage(ctx, w.ID, owner.ID, a.ID, b.Stages[0].ID); !errors.Is(err, ErrStageNotFound) {
		t.Errorf("delete = %v", err)
	}
}

func TestPipelineTablesRLSIsolation(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ofA, ofB := defaultPipeline(t, s, tn.a.ID), defaultPipeline(t, s, tn.b.ID)

	for _, table := range []string{"pipelines", "pipeline_stages"} {
		t.Run(table, func(t *testing.T) {
			rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, func(ctx context.Context, tx pgx.Tx) {
				if got := strs(t, ctx, tx, `SELECT DISTINCT workspace_id::text FROM `+table); !slices.Equal(got, []string{tn.a.ID}) {
					t.Errorf("workspaces visible %v, want only A", got)
				}
			})
			for _, c := range []struct{ name, setting, value string }{
				{"no setting", "", ""},
				{"empty setting", "app.workspace_id", ""},
				{"user setting grants nothing", "app.user_id", tn.ownerA.ID},
			} {
				t.Run(c.name, func(t *testing.T) {
					rawTx(t, tn.pool, c.setting, c.value, func(ctx context.Context, tx pgx.Tx) {
						var count int
						if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
							t.Fatal(err)
						}
						if count != 0 {
							t.Errorf("%d rows visible", count)
						}
					})
				})
			}
		})
	}
	writes := func(name string, fn func(ctx context.Context, tx pgx.Tx)) {
		t.Run(name, func(t *testing.T) { rawTx(t, tn.pool, "app.workspace_id", tn.a.ID, fn) })
	}
	writes("insert a pipeline into B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `INSERT INTO pipelines (workspace_id, name) VALUES ($1, 'rogue')`, tn.b.ID)
		wantDenied(t, "insert", err)
	})
	writes("insert a stage into B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `INSERT INTO pipeline_stages (workspace_id, pipeline_id, name, position, color, probability, kind)
			VALUES ($1, $2, 'rogue', 99, '#000000', 0, 'open')`, tn.b.ID, ofB.ID)
		wantDenied(t, "insert", err)
	})
	writes("the default pipeline function cannot reach B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `SELECT create_pipeline_with_default_stages($1)`, tn.b.ID)
		wantDenied(t, "function", err)
	})
	writes("move a pipeline from A to B", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `UPDATE pipelines SET workspace_id = $1 WHERE id = $2`, tn.b.ID, ofA.ID)
		wantDenied(t, "move", err)
	})
	writes("update a pipeline of B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE pipelines SET name = 'pwned' WHERE id = $1`, ofB.ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	writes("update a stage of B", func(ctx context.Context, tx pgx.Tx) {
		tag, err := tx.Exec(ctx, `UPDATE pipeline_stages SET name = 'pwned' WHERE id = $1`, ofB.Stages[0].ID)
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("update = %d rows, %v; want 0 rows", tag.RowsAffected(), err)
		}
	})
	writes("hard delete of a pipeline is not granted", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `DELETE FROM pipelines WHERE id = $1`, ofA.ID)
		wantDenied(t, "delete", err)
	})
	writes("hard delete of a stage is not granted", func(ctx context.Context, tx pgx.Tx) {
		_, err := tx.Exec(ctx, `DELETE FROM pipeline_stages WHERE id = $1`, ofA.Stages[0].ID)
		wantDenied(t, "delete", err)
	})
}

// A foreign key check bypasses Row-Level Security, so these run as the superuser,
// with no policy in the way at all: only the schema can refuse them.
func TestStageSchemaRefusesBadReferencesAndDuplicatePositions(t *testing.T) {
	tn := newTenants(t)
	s := New(tn.pool)
	ofA, ofB := defaultPipeline(t, s, tn.a.ID), defaultPipeline(t, s, tn.b.ID)
	ctx := context.Background()
	owner := ownerPool(t)

	inTx := func(name string, fn func(tx pgx.Tx)) {
		t.Run(name, func(t *testing.T) {
			tx, err := owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			fn(tx)
		})
	}
	const insert = `INSERT INTO pipeline_stages (workspace_id, pipeline_id, name, position, color, probability, kind)
		VALUES ($1, $2, 'x', $3, $4, $5, $6)`
	inTx("a stage in A pointing at B's pipeline", func(tx pgx.Tx) {
		_, err := tx.Exec(ctx, insert, tn.a.ID, ofB.ID, 99, "#000000", 0, "open")
		wantPgCode(t, "insert", err, pgFKViolation)
	})
	inTx("two stages at one position", func(tx pgx.Tx) {
		_, err := tx.Exec(ctx, insert, tn.a.ID, ofA.ID, 0, "#000000", 0, "open")
		wantPgCode(t, "insert", err, "23505")
	})
	for _, c := range []struct {
		name        string
		color       string
		probability int
		kind        string
	}{
		{"probability above 100", "#000000", 101, "open"},
		{"negative probability", "#000000", -1, "open"},
		{"unknown kind", "#000000", 0, "closed"},
		{"a color that is not one", "red", 0, "open"},
	} {
		inTx(c.name, func(tx pgx.Tx) {
			_, err := tx.Exec(ctx, insert, tn.a.ID, ofA.ID, 99, c.color, c.probability, c.kind)
			wantPgCode(t, "insert", err, "23514")
		})
	}
	inTx("a live stage needs a position", func(tx pgx.Tx) {
		_, err := tx.Exec(ctx, `UPDATE pipeline_stages SET position = NULL WHERE id = $1`, ofA.Stages[0].ID)
		wantPgCode(t, "update", err, "23514")
	})
}
