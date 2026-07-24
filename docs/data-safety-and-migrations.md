# Data Safety During Rollback

Two different questions hide inside "what happens to data when we roll back":
one about the three business microservices, one about the operator's own
Postgres store. They have different answers.

---

## 1. The business microservices hold no persistent data

`order-service`, `inventory-service`, and `payment-service` each store their
domain data in an **in-memory, per-process map** guarded by a `sync.RWMutex`
(`services/*/repository/repository.go` — `InMemoryOrderRepository`,
`InMemoryInventoryRepository`, `InMemoryPaymentRepository`). There is no
database connection, no volume, nothing that survives past the process.

**Consequence: a Helm rollback of these services cannot lose or corrupt
application data, because there is no data to lose or corrupt across a
rollback that a plain pod restart wouldn't already discard.** Rolling
`order-service` from revision N back to N-1 replaces the pod; the new pod
starts with an empty order map exactly as it would after a crash, an
eviction, or a routine rolling update. Schema questions ("what if a field was
added to `models.Order`?") don't apply here in the traditional sense either —
there is no on-disk schema to be incompatible with; only the currently-running
binary's in-memory struct definition exists at any moment.

This is worth stating explicitly rather than leaving implicit, because it is
the reason the platform's rollback safety work (dependency batch rollback,
SLO-aware verification, this document) has never needed an "undo the data
migration" step for the services the operator actually heals. If a future
iteration of this platform gives these services a real datastore, the
migration discipline in §3 below is exactly what should be adopted for them
too.

---

## 2. The operator's own Postgres store is the one real, persistent schema

`internal/operator/storage/postgres.go` (`PostgresIncidentStore`) is
different: it is a genuine Postgres-backed store the operator uses to persist
incidents, decision history, and rollback history — the audit trail behind
the `GET /runbooks` endpoint and the MTTR dashboards. This schema *is* real,
*is* persistent, and *does* need a real answer to "what happens on rollback."

### What existed before this change

`migrate()` ran a fixed list of `CREATE TABLE IF NOT EXISTS` statements,
unconditionally, on every boot. That is safe for the one thing it does
(create tables that don't yet exist) but it had no way to represent an
`ALTER TABLE`, no record of what had been applied, and therefore no way to
reason about compatibility between a running binary and the schema it finds
itself talking to.

### The versioned migration system (`internal/operator/storage/migrations.go`)

- Each schema change is a `migration{Version, Description, Breaking,
  Statements}` entry in an append-only, ordered list.
- A `schema_migrations` table (created directly, before anything else) is the
  ledger: every applied migration's version, description, breaking flag, and
  timestamp is recorded there, in the same transaction as the DDL it
  describes — so a failure partway through a migration can't leave the schema
  half-changed and unrecorded.
- `RequiredSchemaVersion` is **derived from the migrations list itself**
  (`highestVersion(migrations)`), not hand-maintained — it can't silently
  drift from what the binary's code actually depends on.

### Expand/contract discipline

1. **Never drop or rename a column/table in the same migration that stops
   using it.** First stop reading/writing it in an additive ("expand")
   migration that old code can still coexist with, deploy, observe, then
   remove it later.
2. **Mark a migration `Breaking: true`** if it drops/renames a column or
   table, changes a column's type incompatibly, or adds a `NOT NULL` column
   with no default — anything an older binary's hardcoded column list could
   not survive.
3. **Additive changes are not breaking.** A new nullable column, a new index,
   a new table: old code that doesn't know about them keeps working exactly
   as before, because every query in this store uses explicit column lists,
   never `SELECT *`.

### The startup compatibility guard — the actual rollback-safety answer

This is the piece that directly answers "how do we preserve data across a
rollback": **the operator now refuses to start** if the database has a
`Breaking` migration applied whose version is higher than what this binary's
build understands.

That situation — DB schema ahead of the running binary by a breaking change —
is exactly the fingerprint of the **operator's own Helm release
(`helm/rollback-operator`) being rolled back** to a build older than a
migration that already ran against the live database. Without the guard, that
old binary would boot normally and either fail with a cryptic SQL error the
first time it touched the changed column/table, or — worse, if the change was
a same-named column repurposed to a new meaning — silently misinterpret data
without ever erroring. With the guard, it fails loudly at startup with a
message naming the exact migration, its version, and what to do:

```
refusing to start: database schema is ahead by breaking migration 3
("drop legacy column"), but this operator build only understands schema
version 2. This looks like the rollback-operator Helm release was rolled
back to a build that predates a breaking data migration. Roll forward to a
build with schema version >= 3 instead, or restore the database from a
backup taken before that migration ran.
```

This converts a silent data-integrity risk into a safe, loud failure — the
same "reported success must be verified" principle the live-failure-injection
suite established for service rollbacks
(`docs/experiments/live-failure-simulation-findings.md`, F1/§6.4), applied to
the operator's own schema instead of a service's SLO.

`checkCompatibility` is a pure function taking the applied-migrations list and
the build's required version — it needs no live database to test, and is unit
tested directly in `internal/operator/storage/migrations_test.go`, including
the exact rolled-back-operator scenario above.

### What this does *not* do

- It does not attempt automatic down-migrations. Down-migrations that
  actually reverse a breaking change (e.g. resurrecting dropped data) are, in
  general, lossy or impossible — the guard's job is to refuse to run
  unsafely, not to pretend a rollback can always be made whole.
- It does not migrate data values, only schema (table/column/index
  definitions). A migration that needs to backfill or transform existing
  rows is still a normal additional statement in that migration's
  `Statements` list — the transaction wrapping already covers it.
