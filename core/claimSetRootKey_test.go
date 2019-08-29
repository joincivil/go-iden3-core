package core

import (
	"testing"

	"github.com/iden3/go-iden3-core/merkletree"
	"github.com/stretchr/testify/assert"
)

func TestClaimSetRootKey(t *testing.T) {
	// ClaimSetRootKey
	id, err := IDFromString("113kyY52PSBr9oUqosmYkCavjjrQFuiuAw47FpZeUf")
	assert.Nil(t, err)

	rootKey := merkletree.Hash(merkletree.ElemBytes{
		0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b,
		0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b,
		0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b,
		0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0b, 0x0c})
	c0, err := NewClaimSetRootKey(&id, &rootKey)
	assert.Nil(t, err)
	c0.Version = 1
	c0.Era = 1
	e := c0.Entry()
	assert.Equal(t,
		"0x18401636bb62be35476e292834b24b85527131ce434d6ffa43b95f0a8b1f4586",
		e.HIndex().Hex())
	assert.Equal(t,
		"0x01705b25f2cf7cda34d836f09e9b0dd1777bdc16752657cd9d1ae5f6286525ba",
		e.HValue().Hex())
	dataTestOutput(&e.Data)
	assert.Equal(t, ""+
		"0000000000000000000000000000000000000000000000000000000000000000"+
		"0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0c"+
		"0000003cc1c968fa000000000000000000000000000000000000000000000328"+
		"0000000000000000000000000000000000000001000000010000000000000002",
		e.Data.String())
	c1 := NewClaimSetRootKeyFromEntry(e)
	c2, err := NewClaimFromEntry(e)
	assert.Nil(t, err)
	assert.Equal(t, c0, c1)
	assert.Equal(t, c0, c2)
}
