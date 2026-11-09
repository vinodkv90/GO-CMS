package interfaces

// Capability represents an atomic feature supported by a database adapter.
type Capability string

const (
	// CapTransactions indicates support for atomic multi-statement / multi-record transactions.
	CapTransactions Capability = "transactions"
	// CapForeignKeys indicates database-enforced referential integrity.
	CapForeignKeys Capability = "foreign_keys"
	// CapUniqueConstraints indicates database-enforced unique constraints or unique indexes.
	CapUniqueConstraints Capability = "unique_constraints"
	// CapJSONIndexing indicates native indexing and querying inside JSON / BSON structures.
	CapJSONIndexing Capability = "json_indexing"
	// CapFullTextSearch indicates native linguistic full-text indexing and querying.
	CapFullTextSearch Capability = "full_text_search"
	// CapTransactionalDDL indicates support for DDL operations within a transaction.
	CapTransactionalDDL Capability = "transactional_ddl"
	// CapNestedTx indicates support for savepoints / nested transactions.
	CapNestedTx Capability = "nested_transactions"
	// CapCursorPagination indicates efficient keyset/cursor pagination.
	CapCursorPagination Capability = "cursor_pagination"
	// CapBulkUpsert indicates atomic batch insert or update on conflict.
	CapBulkUpsert Capability = "bulk_upsert"
)

// Capabilities provides read access to the supported features of a database backend.
type Capabilities interface {
	Has(cap Capability) bool
	All() []Capability
}

// CapabilitySet is a concrete, immutable set of capabilities.
type CapabilitySet struct {
	caps map[Capability]bool
}

// NewCapabilitySet creates a new CapabilitySet with the specified capabilities enabled.
func NewCapabilitySet(caps ...Capability) *CapabilitySet {
	m := make(map[Capability]bool, len(caps))
	for _, c := range caps {
		m[c] = true
	}
	return &CapabilitySet{caps: m}
}

// Has returns true if the capability is supported.
func (cs *CapabilitySet) Has(cap Capability) bool {
	if cs == nil || cs.caps == nil {
		return false
	}
	return cs.caps[cap]
}

// All returns a slice of all supported capabilities.
func (cs *CapabilitySet) All() []Capability {
	if cs == nil || cs.caps == nil {
		return nil
	}
	res := make([]Capability, 0, len(cs.caps))
	for c := range cs.caps {
		res = append(res, c)
	}
	return res
}
