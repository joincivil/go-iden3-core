package main

import (
	"bytes"
	"fmt"

	"github.com/iden3/go-iden3-core/core"
	"github.com/iden3/go-iden3-core/db"
	"github.com/iden3/go-iden3-core/merkletree"
)

func main() {
	storage, err := db.NewLevelDbStorage("./path", false)
	if err != nil {
		panic(err)
	}
	// new merkletree of 140 levels of maximum depth using the defined
	// storage
	mt, err := merkletree.NewMerkleTree(storage, 140)
	if err != nil {
		panic(err)
	}
	defer mt.Storage().Close()

	name0 := "alice@iden3.io"
	id0, err := core.IDFromString("14ZjYvqmB4sNQG5o1orKUWinDt1Zk13iVvs4zPCPmvE")
	if err != nil {
		panic(err)
	}
	claim0 := core.NewClaimAssignName(name0, id0)
	claimEntry0 := claim0.Entry()

	name1 := "bob@iden3.io"
	id1, err := core.IDFromString("11iu5zg52EFoKcKcE6ChPiChftxBdidpxzh4vsmzf6F")
	if err != nil {
		panic(err)
	}
	claim1 := core.NewClaimAssignName(name1, id1)
	claimEntry1 := claim1.Entry()

	fmt.Println("adding claim0")
	err = mt.Add(claimEntry0)
	if err != nil {
		panic(err)
	}
	fmt.Println("merkle root: " + mt.RootKey().Hex())
	fmt.Println("adding claim1")
	err = mt.Add(claimEntry1)
	if err != nil {
		panic(err)
	}

	mp, err := mt.GenerateProof(claimEntry0.HIndex(), nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("merkle root: " + mt.RootKey().Hex())

	fmt.Println("merkle proof: ", mp)
	checked := merkletree.VerifyProof(mt.RootKey(), mp,
		claimEntry0.HIndex(), claimEntry0.HValue())
	fmt.Println("merkle proof checked:", checked)

	claimDataInPos, err := mt.GetDataByIndex(claimEntry0.HIndex())
	if err != nil {
		panic(err)
	}
	claimEntryInPos := merkletree.Entry{Data: *claimDataInPos}
	// print true if the claimInPosBytes is the same than claimEntry0.Bytes()
	fmt.Println("claim in position equals to the original:",
		bytes.Equal(claimEntry0.Bytes(), claimEntryInPos.Bytes()))

	name2 := "eve@iden3.io"
	id2, err := core.IDFromString("12cCE371biv2KzYyBiHZDGmc4nHPaJsQBCYS7Zwxkyk")
	if err != nil {
		panic(err)
	}
	claim2 := core.NewClaimAssignName(name2, id2)
	claimEntry2 := claim2.Entry()

	mp, err = mt.GenerateProof(claimEntry2.HIndex(), nil)
	if err != nil {
		panic(err)
	}

	fmt.Println("merkle proof: ", mp)

	checked = merkletree.VerifyProof(mt.RootKey(), mp, claimEntry2.HIndex(), claimEntry2.HValue())

	fmt.Println("merkle proof of non existence checked:", checked)

	// New merkle tree with some claims to get the graphviz visualization.
	storage2, err := db.NewLevelDbStorage("./graph", false)
	if err != nil {
		panic(err)
	}
	mt2, err := merkletree.NewMerkleTree(storage2, 140)
	if err != nil {
		panic(err)
	}
	defer mt2.Storage().Close()

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("%v@iden3.io", i)
		id, err := core.IDFromString("12cCE371biv2KzYyBiHZDGmc4nHPaJsQBCYS7Zwxkyk")
		if err != nil {
			panic(err)
		}
		claim := core.NewClaimAssignName(name, id)
		err = mt2.Add(claim.Entry())
		if err != nil {
			panic(err)
		}
	}
	s := bytes.NewBufferString("")
	mt2.GraphViz(s, nil)
	fmt.Println("GraphViz code:")
	fmt.Println(s)
}
