package app_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arturonavax/anx/internal/app"
	"github.com/arturonavax/anx/internal/catalog"
	"github.com/arturonavax/anx/internal/domain"
	"github.com/arturonavax/anx/internal/platform"
)

func TestConfigurationHostSelectorsInPlanAndDiff(t *testing.T) {
	fx := newFixture(t)
	manifests := filepath.Join(fx.repo, "manifests")
	write(t, filepath.Join(manifests, "profile.yaml"), `
configurations:
  - id: match
    group: host-rules
    source: files/example
    target: ~/.match
    os: [linux]
    distros: [Ubuntu]
    desktops: [gnome]
    display_servers: [wayland]
  - id: distro
    group: host-rules
    source: files/example
    target: ~/.distro
    distros: [fedora]
  - id: display
    group: host-rules
    source: files/example
    target: ~/.display
    display_servers: [x11]
  - id: desktop
    group: host-rules
    source: files/example
    target: ~/.desktop
    desktops: [kde]
  - id: portable
    group: host-rules
    source: files/example
    target: ~/.portable
profiles:
  - name: host-rules
    configurations: [match, distro, display, desktop, portable]
`)
	write(t, filepath.Join(fx.repo, "files", "example"), "candidate")
	cat, err := catalog.NewRegistryWithOverlays(manifests, "")
	if err != nil {
		t.Fatal(err)
	}
	plat := platform.NewTestInfo(platform.TestInfoParams{
		OS: "linux", Distribution: "ubuntu", DesktopEnv: "gnome", DisplayServer: "wayland",
		HomeDir: fx.home, StateDir: fx.plat.StateDir(),
	})
	sync := app.NewSyncService(plat, cat, fx.snap, fx.store, fx.repo)
	svc := app.NewInstallService(app.InstallDeps{Platform: plat, Catalog: cat, Store: fx.store, Sync: sync})
	ctx := context.Background()
	plan, err := svc.Plan(ctx, "host-rules", nil)
	if err != nil {
		t.Fatal(err)
	}
	var planned []string
	for _, step := range plan.Steps {
		planned = append(planned, step.ResourceID)
	}
	diffs, err := sync.Diff(ctx, "host-rules")
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, diff := range diffs {
		seen = append(seen, diff.ConfigID)
	}
	slices.Sort(planned)
	slices.Sort(seen)
	want := []string{"match", "portable"}
	if !slices.Equal(planned, want) || !slices.Equal(seen, want) {
		t.Fatalf("expected %v in plan and diff, got plan=%v diff=%v", want, planned, seen)
	}
}

type concurrencyProbe struct {
	running   atomic.Int32
	peak      atomic.Int32
	completed atomic.Int32
}

func (*concurrencyProbe) Name() string                     { return "probe" }
func (*concurrencyProbe) IsAvailable(context.Context) bool { return true }
func (*concurrencyProbe) IsInstalled(context.Context, domain.ManagedResource) (bool, string, error) {
	return false, "", nil
}
func (*concurrencyProbe) Uninstall(context.Context, domain.ManagedResource) error { return nil }
func (p *concurrencyProbe) Install(ctx context.Context, _ domain.ManagedResource) error {
	active := p.running.Add(1)
	defer p.running.Add(-1)
	for peak := p.peak.Load(); active > peak; peak = p.peak.Load() {
		if p.peak.CompareAndSwap(peak, active) {
			break
		}
	}
	timer := time.NewTimer(40 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		p.completed.Add(1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestProfileCapsInstallConcurrency(t *testing.T) {
	fx := newFixture(t)
	manifests := filepath.Join(fx.repo, "manifests")
	write(t, filepath.Join(manifests, "profile.yaml"), "profiles:\n  - name: serial\n    max_parallel: 1\n")
	cat, err := catalog.NewRegistryWithOverlays(manifests, "")
	if err != nil {
		t.Fatal(err)
	}
	probe := &concurrencyProbe{}
	svc := app.NewInstallService(app.InstallDeps{
		Platform: fx.plat, Catalog: cat, Provider: probe, Store: fx.store,
	})
	plan := &domain.ExecutionPlan{ID: "fixture-parallelism", Profile: "serial"}
	for i, id := range []string{"git", "curl", "anx"} {
		plan.Steps = append(plan.Steps, domain.PlanStep{ID: i + 1, Action: domain.ActionInstall, ResourceID: id})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := svc.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if probe.peak.Load() != 1 || probe.completed.Load() != 3 {
		t.Fatalf("max_parallel=1: peak=%d completed=%d", probe.peak.Load(), probe.completed.Load())
	}
}

func TestProfileRejectsUnsafeParallelLimit(t *testing.T) {
	manifests := t.TempDir()
	write(t, filepath.Join(manifests, "profile.yaml"), "profiles:\n  - name: oversized\n    max_parallel: 33\n")
	_, err := catalog.NewRegistryWithOverlays(manifests, "")
	if err == nil || !strings.Contains(err.Error(), "max_parallel") {
		t.Fatalf("expected a validation error for max_parallel: %v", err)
	}
}
