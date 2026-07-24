// internal/operator/storage/migrations_test.go
//
// checkCompatibility is deliberately a pure function (no DB access) so the
// startup safety-net logic can be verified directly, without spinning up a
// real Postgres instance.

package storage

import "testing"

func TestCheckCompatibilityAllowsNonBreakingAheadOfBuild(t *testing.T) {
	// A newer binary applied migration 3 (non-breaking, e.g. a new index)
	// and was then rolled back to this build, which only understands up to
	// version 2. Non-breaking migrations must never block startup.
	applied := []appliedMigration{
		{Version: 1, Description: "baseline", Breaking: false},
		{Version: 2, Description: "indexes", Breaking: false},
		{Version: 3, Description: "add optional nullable column", Breaking: false},
	}
	if err := checkCompatibility(applied, 2); err != nil {
		t.Fatalf("non-breaking migration ahead of this build must not block startup, got: %v", err)
	}
}

func TestCheckCompatibilityBlocksBreakingAheadOfBuild(t *testing.T) {
	// This is the scenario the guard exists for: the operator's own Helm
	// release was rolled back past a migration that already dropped/renamed
	// something this build's queries still depend on.
	applied := []appliedMigration{
		{Version: 1, Description: "baseline", Breaking: false},
		{Version: 2, Description: "indexes", Breaking: false},
		{Version: 3, Description: "drop legacy column", Breaking: true},
	}
	err := checkCompatibility(applied, 2)
	if err == nil {
		t.Fatal("breaking migration ahead of this build must block startup, got nil error")
	}
}

func TestCheckCompatibilityAllowsBreakingMigrationThisBuildApplied(t *testing.T) {
	// A breaking migration at or below the build's own required version is
	// exactly what this binary itself applied (or a same-version peer did):
	// it must not treat its own schema as incompatible with itself.
	applied := []appliedMigration{
		{Version: 1, Description: "baseline", Breaking: false},
		{Version: 2, Description: "drop legacy column", Breaking: true},
	}
	if err := checkCompatibility(applied, 2); err != nil {
		t.Fatalf("a breaking migration this build understands must not block startup, got: %v", err)
	}
}

func TestCheckCompatibilityEmptyDatabase(t *testing.T) {
	if err := checkCompatibility(nil, RequiredSchemaVersion); err != nil {
		t.Fatalf("a fresh/empty database must never block startup, got: %v", err)
	}
}

func TestHighestVersionDerivedFromMigrationsList(t *testing.T) {
	// RequiredSchemaVersion must track the actual migrations table, not a
	// hand-maintained constant that can drift from it.
	got := highestVersion(migrations)
	want := 0
	for _, m := range migrations {
		if m.Version > want {
			want = m.Version
		}
	}
	if got != want {
		t.Fatalf("highestVersion(migrations) = %d, want %d", got, want)
	}
	if RequiredSchemaVersion != got {
		t.Fatalf("RequiredSchemaVersion = %d, want it to equal highestVersion(migrations) = %d", RequiredSchemaVersion, got)
	}
}

func TestMigrationVersionsAreUniqueAndSequential(t *testing.T) {
	seen := make(map[int]bool)
	for i, m := range migrations {
		wantVersion := i + 1
		if m.Version != wantVersion {
			t.Fatalf("migrations[%d] has Version %d, expected sequential %d — migrations must never be reordered or have gaps", i, m.Version, wantVersion)
		}
		if seen[m.Version] {
			t.Fatalf("duplicate migration version %d", m.Version)
		}
		seen[m.Version] = true
		if len(m.Statements) == 0 {
			t.Fatalf("migration %d (%s) has no statements", m.Version, m.Description)
		}
	}
}
