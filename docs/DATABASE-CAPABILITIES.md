# Database Capabilities Matrix & Abstraction Design

## 1. Design Rationale

The CMS supports three primary database backends:
1. **PostgreSQL** (via `pgx` / `database/sql`)
2. **MySQL** (via `go-sql-driver/mysql`)
3. **MongoDB** (via official Go `mongo-driver`)

Relational engines and document stores possess fundamentally different guarantees, storage formats, and feature sets. Rather than implementing a lowest-common-denominator ORM that hides these distinctions or makes assumptions that fail at runtime, the CMS implements a **Capability-Driven Architecture**.

Each database adapter reports a strongly typed set of capabilities via the `Capabilities()` method on the database interface. Modules query this capability set before executing features that depend on specific storage characteristics.

---

## 2. Capabilities Matrix

| Capability | PostgreSQL | MySQL 8.0+ | MongoDB 6.0+ | Description |
| :--- | :---: | :---: | :---: | :--- |
| **ACID Transactions** | Yes (Multi-table) | Yes (InnoDB, Multi-table) | Yes (Multi-document with Replica Set) | Ability to run atomic multi-record commit/rollback. |
| **Foreign Key Constraints** | Yes | Yes (InnoDB) | No | Enforced referential integrity at the database level. |
| **Unique Constraints** | Yes | Yes | Yes (Unique Index) | Database-enforced uniqueness on one or more columns/keys. |
| **JSON Column Indexing** | Yes (JSONB + GIN) | Yes (Generated cols / Multi-valued) | Native (BSON document fields) | Indexing and efficient querying within nested JSON structures. |
| **Full-Text Search** | Yes (`tsvector`/`tsquery`) | Yes (`FULLTEXT` indexes) | Yes (`$text` index or Atlas Search) | Tokenized, stemmed linguistic search queries. |
| **Schema Migrations** | Yes (DDL in Transactions) | Yes (Non-transactional DDL) | Yes (Index/collection validation DDL) | Automated versioned schema progression. |
| **Savepoints / Nested Tx** | Yes | Yes | No | Partial rollbacks within an ongoing transaction. |
| **Ad-hoc Relations (JOINs)** | Yes | Yes | Yes (`$lookup` aggregation) | Cross-collection or cross-table data joining. |
| **Cursor Pagination** | Yes (Keyset pagination) | Yes (Keyset pagination) | Yes (Range or bucket pagination) | High-performance pagination without `OFFSET` degradation. |
| **Bulk Upsert / On Conflict**| Yes (`ON CONFLICT`) | Yes (`ON DUPLICATE KEY UPDATE`)| Yes (`bulkWrite` with `upsert`) | Atomic insert-or-update batch operations. |
| **Optimistic Locking** | Yes (`version` column) | Yes (`version` column) | Yes (`version` field) | Version check during update (`UPDATE ... WHERE id = ? AND version = ?`). |

---

## 3. Go Interface Definition

```go
package database

import "context"

// Capability represents an atomic feature supported by a database adapter.
type Capability string

const (
    CapTransactions      Capability = "transactions"
    CapForeignKeys       Capability = "foreign_keys"
    CapUniqueConstraints Capability = "unique_constraints"
    CapJSONIndexing      Capability = "json_indexing"
    CapFullTextSearch    Capability = "full_text_search"
    CapTransactionalDDL  Capability = "transactional_ddl"
    CapNestedTx          Capability = "nested_transactions"
    CapCursorPagination  Capability = "cursor_pagination"
    CapBulkUpsert        Capability = "bulk_upsert"
)

// Capabilities describes the operational feature set of the active database adapter.
type Capabilities interface {
    Has(cap Capability) bool
    All() []Capability
}

// Database represents the base database abstraction.
type Database interface {
    DriverName() string
    Ping(ctx context.Context) error
    Close() error
    Capabilities() Capabilities
}

// TransactionManager provides transactional boundaries when supported.
type TransactionManager interface {
    Begin(ctx context.Context) (Transaction, error)
}

// Transaction represents an active unit of work.
type Transaction interface {
    Commit(ctx context.Context) error
    Rollback(ctx context.Context) error
}
```

---

## 4. Graceful Degradation & Feature Gating

When an operation requires a capability:
1. The service checks `db.Capabilities().Has(Cap...)`.
2. If true, the optimal native database execution path is chosen (e.g. native database transaction or unique index).
3. If false, the service evaluates whether an application-level fallback is safe:
   - For **Foreign Keys in MongoDB:** Application-level referential integrity verification runs inside the service layer before deletion or creation.
   - For **Transactions in Standalone MongoDB:** If transactions are requested but the MongoDB instance is a single node without a replica set, the CMS emits an explicit actionable error explaining that replica sets are required for transactions.
4. The CMS never silently ignores a required capability. If an invariant cannot be guaranteed, the operation fails with a clear, descriptive domain error.
