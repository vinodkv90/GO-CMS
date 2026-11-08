package interfaces

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCapabilitySet(t *testing.T) {
	set := NewCapabilitySet(
		CapTransactions,
		CapForeignKeys,
		CapUniqueConstraints,
	)

	assert.True(t, set.Has(CapTransactions))
	assert.True(t, set.Has(CapForeignKeys))
	assert.True(t, set.Has(CapUniqueConstraints))
	assert.False(t, set.Has(CapTransactionalDDL))
	assert.False(t, set.Has(CapFullTextSearch))

	all := set.All()
	assert.Len(t, all, 3)

	var nilSet *CapabilitySet
	assert.False(t, nilSet.Has(CapTransactions))
	assert.Nil(t, nilSet.All())
}
