package claimsrv

import (
	"crypto/ecdsa"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/iden3/go-iden3/core"
	"github.com/iden3/go-iden3/merkletree"
	"github.com/iden3/go-iden3/services/rootsrv"
	"github.com/iden3/go-iden3/services/signsrv"
	"github.com/iden3/go-iden3/utils"
	log "github.com/sirupsen/logrus"
)

var (
	ErrNotFound     = errors.New("value not found")
	ErrRevokedClaim = errors.New("the claim is revoked: the next version exists")
)

type Service interface {
	CommitNewIdRoot(idaddr common.Address, kSignPk *ecdsa.PublicKey, root merkletree.Hash, timestamp uint64, signature *utils.SignatureEthMsg) (*core.ClaimSetRootKey, error)
	AddClaimAssignName(claimAssignName core.ClaimAssignName) error
	AddClaimAuthorizeKSign(idAddr common.Address, claimAuthorizeKSignMsg ClaimAuthorizeKSignMsg) error
	AddClaimAuthorizeKSignFirst(idAddr common.Address, claimAuthorizeKSign core.ClaimAuthorizeKSign) error
	// TODO
	//AddClaimAuthorizeKSignSecp256k1(idAddr common.Address, claimAuthorizeKSignMsg ClaimAuthorizeKSignMsg) error
	AddClaimAuthorizeKSignSecp256k1First(idAddr common.Address,
		claimAuthorizeKSignSecp256k1 core.ClaimAuthorizeKSignSecp256k1) error
	AddUserIdClaim(idAddr common.Address, claimValueMsg ClaimValueMsg) error
	AddDirectClaim(claim core.ClaimBasic) error
	GetIdRoot(idAddr common.Address) (merkletree.Hash, []byte, error)
	GetClaimProofUserByHi(idAddr common.Address, hi *merkletree.Hash) (*core.ProofOfClaim, error)
	GetClaimProofUserByHiOld(idAddr common.Address, hi merkletree.Hash) (*ProofOfClaimUser, error)
	GetClaimProofByHi(hi *merkletree.Hash) (*core.ProofOfClaim, error)
	MT() *merkletree.MerkleTree
}

type ServiceImpl struct {
	mt      *merkletree.MerkleTree
	rootsrv rootsrv.Service
	signer  signsrv.Service
}

func New(mt *merkletree.MerkleTree, rootsrv rootsrv.Service, signer signsrv.Service) *ServiceImpl {
	return &ServiceImpl{mt, rootsrv, signer}
}

// SetNewIdRoot checks that the data is valid and performs a claim in the Relay merkletree setting the new Root of the emiting Id
func (cs *ServiceImpl) CommitNewIdRoot(idaddr common.Address, kSignPk *ecdsa.PublicKey, root merkletree.Hash, timestamp uint64, signature *utils.SignatureEthMsg) (*core.ClaimSetRootKey, error) {
	// get the user's id storage, using the user id prefix (the idaddress itself)
	stoUserId := cs.mt.Storage().WithPrefix(idaddr.Bytes())

	// open the MerkleTree of the user
	userMT, err := merkletree.NewMerkleTree(stoUserId, 140)
	if err != nil {
		return &core.ClaimSetRootKey{}, err
	}

	// verify that the KSign is authorized
	if !CheckKSignInIddb(userMT, kSignPk) {
		return &core.ClaimSetRootKey{}, errors.New("can not verify the KSign")
	}
	// in the future the user merkletree will be in the client side, and this step will be a check of the ProofOfKSign

	// check data timestamp
	verified := utils.VerifyTimestamp(timestamp, 30000) //needs to be from last 30 seconds
	if !verified {
		return &core.ClaimSetRootKey{}, errors.New("timestamp too old")
	}
	// check signature with idaddr
	// whee data signed is idaddr+root+timestamp
	timestampBytes := utils.Uint64ToEthBytes(timestamp)
	// signature of idaddr+root+timestamp, only valid if is from last X seconds
	var msg []byte
	msg = append(msg, idaddr.Bytes()...)
	msg = append(msg, root.Bytes()...)
	msg = append(msg, timestampBytes...)
	if !utils.VerifySigEthMsg(crypto.PubkeyToAddress(*kSignPk), signature, msg) {
		return &core.ClaimSetRootKey{}, errors.New("signature can not be verified")
	}

	// claimSetRootKey of the user in the Relay Merkle Tree
	// create new ClaimSetRootKey
	claimSetRootKey := core.NewClaimSetRootKey(idaddr, root)
	// entry := claimSetRootKey.Entry()
	// version, err := GetNextVersion(cs.mt, entry.HIndex())
	version, err := GetNextVersion(cs.mt, claimSetRootKey.Entry().HIndex())
	if err != nil {
		return &core.ClaimSetRootKey{}, err
	}
	claimSetRootKey.Version = version

	// add User's Id Merkle Root into the Relay's Merkle Tree
	e := claimSetRootKey.Entry()
	err = cs.mt.Add(e)
	if err != nil {
		return &core.ClaimSetRootKey{}, err
	}

	// update Relay Root in Smart Contract
	cs.rootsrv.SetRoot(*cs.mt.RootKey())

	return claimSetRootKey, nil
}

// AddClaimAssignName adds ClaimAssignName into the merkletree, updates the root in the smart contract, and returns the merkle proof of the claim in the merkletree
func (cs *ServiceImpl) AddClaimAssignName(claimAssignName core.ClaimAssignName) error {
	// get next version of the claim
	entry := claimAssignName.Entry()
	version, err := GetNextVersion(cs.mt, entry.HIndex())
	if err != nil {
		return err
	}
	claimAssignName.Version = version

	// add ClaimAssignName to the Relay's merkletree
	e := claimAssignName.Entry()
	err = cs.mt.Add(e)
	if err != nil {
		log.Fatal(err)
		return err
	}

	// update relay's root in smart contract
	cs.rootsrv.SetRoot(*cs.mt.RootKey())

	return nil
}

// AddClaimAuthorizeKSign adds ClaimAuthorizeKSign into the Id's merkletree, and adds the Id's merkle root into the Relay's merkletree inside a ClaimSetRootKey. Returns the merkle proof of both Claims
func (cs *ServiceImpl) AddClaimAuthorizeKSign(idAddr common.Address, claimAuthorizeKSignMsg ClaimAuthorizeKSignMsg) error {

	// get the user's id storage, using the user id prefix (the idaddress itself)
	stoUserId := cs.mt.Storage().WithPrefix(idAddr.Bytes())

	// open the MerkleTree of the user
	userMT, err := merkletree.NewMerkleTree(stoUserId, 140)
	if err != nil {
		return err
	}

	// verify that the KSign is authorized
	if !CheckKSignInIddb(userMT, &claimAuthorizeKSignMsg.KSignPk.PublicKey) {
		return errors.New("can not verify the KSign")
	}

	// verify signature of the ClaimAuthorizeKSign
	if !utils.VerifySigEthMsg(crypto.PubkeyToAddress(claimAuthorizeKSignMsg.KSignPk.PublicKey),
		claimAuthorizeKSignMsg.Signature, claimAuthorizeKSignMsg.ClaimAuthorizeKSign.Entry().Bytes()) {
		return errors.New("signature can not be verified")
	}

	// add ClaimAuthorizeKSign into the User's Id Merkle Tree
	err = userMT.Add(claimAuthorizeKSignMsg.ClaimAuthorizeKSign.Entry())
	if err != nil {
		return err
	}

	// create new ClaimSetRootKey
	claimSetRootKey := core.NewClaimSetRootKey(idAddr, *userMT.RootKey())

	// get next version of the claim
	version, err := GetNextVersion(cs.mt, claimSetRootKey.Entry().HIndex())
	if err != nil {
		return err
	}
	claimSetRootKey.Version = version

	// add User's Id Merkle Root into the Relay's Merkle Tree
	err = cs.mt.Add(claimSetRootKey.Entry())
	if err != nil {
		return err
	}

	// update Relay's Root in the Smart Contract
	cs.rootsrv.SetRoot(*cs.mt.RootKey())

	return nil
}

// AddClaimAuthorizeKSign adds ClaimAuthorizeKSign into the Id's merkletree, and adds the Id's merkle root into the Relay's merkletree inside a ClaimSetRootKey. Returns the merkle proof of both Claims
func (cs *ServiceImpl) AddClaimAuthorizeKSignFirst(idAddr common.Address, claimAuthorizeKSign core.ClaimAuthorizeKSign) error {

	// get the user's id storage, using the user id prefix (the idaddress itself)
	stoUserId := cs.mt.Storage().WithPrefix(idAddr.Bytes())

	// open the MerkleTree of the user
	userMT, err := merkletree.NewMerkleTree(stoUserId, 140)
	if err != nil {
		return err
	}

	// add ClaimAuthorizeKSign into the User's Id Merkle Tree
	err = userMT.Add(claimAuthorizeKSign.Entry())
	if err != nil {
		return err
	}

	// create new ClaimSetRootKey
	claimSetRootKey := core.NewClaimSetRootKey(idAddr, *userMT.RootKey())

	// get next version of the claim
	version, err := GetNextVersion(cs.mt, claimSetRootKey.Entry().HIndex())
	if err != nil {
		return err
	}
	claimSetRootKey.Version = version

	// add User's Id Merkle Root into the Relay's Merkle Tree
	err = cs.mt.Add(claimSetRootKey.Entry())
	if err != nil {
		return err
	}

	// update Relay's Root in the Smart Contract
	cs.rootsrv.SetRoot(*cs.mt.RootKey())

	return nil
}

// TODO
// AddClaimAuthorizeKSignSecp256k1 adds ClaimAuthorizeKSignSecp256k1 into the Id's merkletree, and adds the Id's merkle root into the Relay's merkletree inside a ClaimSetRootKey. Returns the merkle proof of both Claims
//func (cs *ServiceImpl) AddClaimAuthorizeKSignSecp256k1(idAddr common.Address, claimAuthorizeKSignMsg ClaimAuthorizeKSignMsg) error {
//
//	// get the user's id storage, using the user id prefix (the idaddress itself)
//	stoUserId := cs.mt.Storage().WithPrefix(idAddr.Bytes())
//
//	// open the MerkleTree of the user
//	userMT, err := merkletree.NewMerkleTree(stoUserId, 140)
//	if err != nil {
//		return err
//	}
//
//	// verify that the KSign is authorized
//	if !CheckKSignInIddb(userMT, claimAuthorizeKSignMsg.KSign) {
//		return errors.New("can not verify the KSign")
//	}
//
//	// verify signature of the ClaimAuthorizeKSign
//	signature, err := common3.HexDecode(claimAuthorizeKSignMsg.Signature)
//	if err != nil {
//		return err
//	}
//	if !utils.VerifySigEthMsg(claimAuthorizeKSignMsg.KSign, signature,
//		claimAuthorizeKSignMsg.ClaimAuthorizeKSign.Entry().Bytes()) {
//		return errors.New("signature can not be verified")
//	}
//
//	// add ClaimAuthorizeKSign into the User's Id Merkle Tree
//	err = userMT.Add(claimAuthorizeKSignMsg.ClaimAuthorizeKSign.Entry())
//	if err != nil {
//		return err
//	}
//
//	// create new ClaimSetRootKey
//	claimSetRootKey := core.NewClaimSetRootKey(idAddr, *userMT.RootKey())
//
//	// get next version of the claim
//	version, err := GetNextVersion(cs.mt, claimSetRootKey.Entry().HIndex())
//	if err != nil {
//		return err
//	}
//	claimSetRootKey.Version = version
//
//	// add User's Id Merkle Root into the Relay's Merkle Tree
//	err = cs.mt.Add(claimSetRootKey.Entry())
//	if err != nil {
//		return err
//	}
//
//	// update Relay's Root in the Smart Contract
//	cs.rootsrv.SetRoot(*cs.mt.RootKey())
//
//	return nil
//}

// AddClaimAuthorizeKSignSecp256k1First adds ClaimAuthorizeKSignSecp256k1 into
// the Id's merkletree, and adds the Id's merkle root into the Relay's
// merkletree inside a ClaimSetRootKey. Returns the merkle proof of both Claims
func (cs *ServiceImpl) AddClaimAuthorizeKSignSecp256k1First(idAddr common.Address,
	claimAuthorizeKSignSecp256k1 core.ClaimAuthorizeKSignSecp256k1) error {

	// get the user's id storage, using the user id prefix (the idaddress itself)
	stoUserId := cs.mt.Storage().WithPrefix(idAddr.Bytes())

	// open the MerkleTree of the user
	userMT, err := merkletree.NewMerkleTree(stoUserId, 140)
	if err != nil {
		return err
	}

	// add ClaimAuthorizeKSign into the User's Id Merkle Tree
	err = userMT.Add(claimAuthorizeKSignSecp256k1.Entry())
	if err != nil {
		return err
	}

	// create new ClaimSetRootKey
	claimSetRootKey := core.NewClaimSetRootKey(idAddr, *userMT.RootKey())

	// get next version of the claim
	version, err := GetNextVersion(cs.mt, claimSetRootKey.Entry().HIndex())
	if err != nil {
		return err
	}
	claimSetRootKey.Version = version

	// add User's Id Merkle Root into the Relay's Merkle Tree
	err = cs.mt.Add(claimSetRootKey.Entry())
	if err != nil {
		return err
	}

	// update Relay's Root in the Smart Contract
	cs.rootsrv.SetRoot(*cs.mt.RootKey())

	return nil
}

// AddUserIdClaim adds a claim into the Id's merkle tree, and with the Id's root, creates a new ClaimSetRootKey and adds it to the Relay's merkletree
func (cs *ServiceImpl) AddUserIdClaim(idAddr common.Address, claimValueMsg ClaimValueMsg) error {
	// get the user's id storage, using the user id prefix (the idaddress itself)
	stoUserId := cs.mt.Storage().WithPrefix(idAddr.Bytes())

	// open the MerkleTree of the user
	userMT, err := merkletree.NewMerkleTree(stoUserId, 140)
	if err != nil {
		return err
	}

	// verify that the KSign is authorized
	if !CheckKSignInIddb(userMT, &claimValueMsg.KSignPk.PublicKey) {
		return errors.New("can not verify the KSign")
	}

	// verify signature with KSign
	if !utils.VerifySigEthMsg(crypto.PubkeyToAddress(claimValueMsg.KSignPk.PublicKey),
		claimValueMsg.Signature, claimValueMsg.ClaimValue.Bytes()) {
		return errors.New("signature can not be verified")
	}

	// add claim in User Id Merkle Tree
	err = userMT.Add(&claimValueMsg.ClaimValue)
	if err != nil {
		return err
	}

	// claimSetRootKey of the user in the Relay Merkle Tree
	// create new ClaimSetRootKey
	claimSetRootKey := core.NewClaimSetRootKey(idAddr, *userMT.RootKey())
	version, err := GetNextVersion(cs.mt, claimSetRootKey.Entry().HIndex())
	if err != nil {
		return err
	}
	claimSetRootKey.Version = version

	// add User's Id Merkle Root into the Relay's Merkle Tree
	err = cs.mt.Add(claimSetRootKey.Entry())
	if err != nil {
		return err
	}

	// update Relay Root in Smart Contract
	cs.rootsrv.SetRoot(*cs.mt.RootKey())

	return nil
}

// AddDirectClaim adds a claim directly to the Relay merkletree
func (cs *ServiceImpl) AddDirectClaim(claim core.ClaimBasic) error {
	err := cs.mt.Add(claim.Entry())
	if err != nil {
		return err
	}
	cs.rootsrv.SetRoot(*cs.mt.RootKey())
	return nil
}

// GetIdRoot returns the root of an Id tree, and the proof of that Root Id tree in the Relay Merkle Tree
func (cs *ServiceImpl) GetIdRoot(idAddr common.Address) (merkletree.Hash, []byte, error) {
	// get the user's id storage, using the user id prefix (the idaddress itself)
	stoUserId := cs.mt.Storage().WithPrefix(idAddr.Bytes())

	// open the MerkleTree of the user
	userMT, err := merkletree.NewMerkleTree(stoUserId, 140)
	if err != nil {
		return merkletree.Hash{}, []byte{}, err
	}

	// build ClaimSetRootKey of the user id
	claimSetRootKey := core.NewClaimSetRootKey(idAddr, *userMT.RootKey())
	version, err := GetNextVersion(cs.mt, claimSetRootKey.Entry().HIndex())
	if err != nil {
		return merkletree.Hash{}, []byte{}, err
	}
	claimSetRootKey.Version = version - 1

	// get proof of SetRootProof in the Relay tree
	idRootProof, err := cs.mt.GenerateProof(claimSetRootKey.Entry().HIndex())
	if err != nil {
		return merkletree.Hash{}, []byte{}, err
	}
	return *userMT.RootKey(), idRootProof.Bytes(), nil
}

// TODO: Remove this
// GetClaimProofUserByHiOld given a Hash(index) (Hi) and an Id, returns the Claim in that Hi position inside the Id's merkletree, and the ClaimSetRootKey with the Id's root in the Relay's merkletree
func (cs *ServiceImpl) GetClaimProofUserByHiOld(idAddr common.Address, hi merkletree.Hash) (*ProofOfClaimUser, error) {
	// get the user's id storage, using the user id prefix (the idaddress itself)
	stoUserId := cs.mt.Storage().WithPrefix(idAddr.Bytes())

	// open the MerkleTree of the user
	userMT, err := merkletree.NewMerkleTree(stoUserId, 140)
	if err != nil {
		return nil, err
	}

	// get the value in the hi position
	// valueBytes, err := userMT.GetValueInPos(hi)
	leafData, err := userMT.GetDataByIndex(&hi)
	if err != nil {
		return nil, err
	}
	// if bytes.Equal(valueBytes, merkletree.EmptyNodeValue[:]) {
	//         return nil, ErrNotFound
	// }

	// value, err := core.ParseValueFromBytes(valueBytes)
	// if err != nil {
	//         return nil, err
	// }

	// get the proof of the value in the User Id Tree
	// idProof, err := userMT.GenerateProof(merkletree.HashBytes(value.Bytes()[:value.IndexLength()]))
	idProof, err := userMT.GenerateProof(&hi)
	if err != nil {
		return nil, err
	}

	leafBytes := leafData.Bytes()
	claimProof := ProofOfTreeLeaf{
		Leaf:  leafBytes[:],
		Proof: idProof.Bytes(),
		Root:  *userMT.RootKey(),
	}

	// build ClaimSetRootKey
	claimSetRootKey := core.NewClaimSetRootKey(idAddr, *userMT.RootKey())
	version, err := GetNextVersion(cs.mt, claimSetRootKey.Entry().HIndex())
	if err != nil {
		return nil, err
	}
	claimSetRootKey.Version = version - 1

	// get the proof of the ClaimSetRootKey in the Relay Tree
	relayProof, err := cs.mt.GenerateProof(claimSetRootKey.Entry().HIndex())
	if err != nil {
		return nil, err
	}
	claimSetRootKeyProof := ProofOfTreeLeaf{
		Leaf:  claimSetRootKey.Entry().Bytes(),
		Proof: relayProof.Bytes(),
		Root:  *cs.mt.RootKey(),
	}

	// get non revocation proofs of the claim
	claimNonRevocationProof, err := getNonRevocationProof(userMT, hi)
	if err != nil {
		return nil, err
	}
	claimSetRootKeyNonRevocationProof, err := getNonRevocationProof(cs.mt, *claimSetRootKey.Entry().HIndex())
	if err != nil {
		return nil, err
	}

	// sign root + date
	sig, date, err := signsrv.SignBytesDate(cs.signer, claimSetRootKeyProof.Root[:])
	if err != nil {
		return nil, err
	}

	proofOfClaim := ProofOfClaimUser{
		claimProof,
		claimNonRevocationProof,
		claimSetRootKeyProof,
		claimSetRootKeyNonRevocationProof,
		date,
		sig[:],
	}
	return &proofOfClaim, nil
}

// GetClaimProofByHi given a Hash(index) (Hi) and an idAddr, returns the Claim
// in that Hi position inside the User merkletree, it's proof of existence and
// of non-revocation, and the proof of existence and of non-revocation for the
// set root claim in the relay tree, all in the form of a ProofOfClaim
func (cs *ServiceImpl) GetClaimProofUserByHi(idAddr common.Address,
	hi *merkletree.Hash) (*core.ProofOfClaim, error) {
	// open the MerkleTree of the user
	userMT, err := utils.NewMerkleTreeUser(idAddr, cs.mt.Storage(), 140)
	if err != nil {
		return nil, err
	}

	// get the value in the hi position
	leafData, err := userMT.GetDataByIndex(hi)
	if err != nil {
		return nil, err
	}

	// get the MT proof of existence of the claim and the non-existence of
	// the claim's next version in the User Tree
	mtpExistUser, err := userMT.GenerateProof(hi)
	if err != nil {
		return nil, err
	}
	mtpNonExistUser, err := getNonRevocationMTProof(userMT, leafData, hi)
	if err != nil {
		return nil, err
	}

	// build ClaimSetRootKey
	claimSetRootKey := core.NewClaimSetRootKey(idAddr, *userMT.RootKey())
	// TODO in a future iteration: make an efficient implementation of GetNextVersion
	version, err := GetNextVersion(cs.mt, claimSetRootKey.Entry().HIndex())
	if err != nil {
		return nil, err
	}
	claimSetRootKey.Version = version - 1

	// Call GetClaimProofByHi to generate a Proof for the top level tree
	proofClaim, err := cs.GetClaimProofByHi(claimSetRootKey.Entry().HIndex())
	if err != nil {
		return nil, err
	}

	// Generate the partial claim proof for the user claim and add it to the ProofOfClaim
	proofClaimUserPartial := core.ProofOfClaimPartial{
		Mtp0: mtpExistUser,
		Mtp1: mtpNonExistUser,
		Root: userMT.RootKey(),
		Aux: &core.SetRootAux{
			Version: claimSetRootKey.Version,
			Era:     0, // NOTE: For the login milestone we don't support Era
			IdAddr:  idAddr,
		},
	}
	proofClaim.Proofs = []core.ProofOfClaimPartial{proofClaimUserPartial, proofClaim.Proofs[0]}
	proofClaim.Leaf = leafData

	return proofClaim, nil
}

// GetClaimProofByHi given a Hash(index) (Hi), returns the Claim in that Hi
// position inside the Relay merkletree, and it's proof of existence and of
// non-revocated, all in the form of a ProofOfClaim
func (cs *ServiceImpl) GetClaimProofByHi(hi *merkletree.Hash) (*core.ProofOfClaim, error) {
	mt, err := cs.mt.Snapshot(cs.mt.RootKey())
	if err != nil {
		return nil, err
	}
	// get the value in the hi position
	leafData, err := mt.GetDataByIndex(hi)
	if err != nil {
		return nil, err
	}

	// get the MT proof of existence of the claim and the non-existence of
	// the claim's next version in the Relay Tree
	mtpExist, err := mt.GenerateProof(hi)
	if err != nil {
		return nil, err
	}
	mtpNonExist, err := getNonRevocationMTProof(mt, leafData, hi)
	if err != nil {
		return nil, err
	}

	rootKey := mt.RootKey()
	sig, date, err := signsrv.SignBytesDate(cs.signer, rootKey[:])
	if err != nil {
		return nil, err
	}

	proofClaimPartial := core.ProofOfClaimPartial{
		Mtp0: mtpExist,
		Mtp1: mtpNonExist,
		Root: rootKey,
		Aux:  nil,
	}
	proofClaim := core.ProofOfClaim{
		Proofs:    []core.ProofOfClaimPartial{proofClaimPartial},
		Leaf:      leafData,
		Date:      date,
		Signature: sig,
	}

	return &proofClaim, nil
}

// MT returns the merkle tree.
func (cs *ServiceImpl) MT() *merkletree.MerkleTree {
	return cs.mt
}

// getNonRevocationMTProof is a helper function to return a proof of non
// existence of the following version of a given claim (leafData).  If the
// following version exists, an error is returned.
func getNonRevocationMTProof(mt *merkletree.MerkleTree, leafData *merkletree.Data, hi *merkletree.Hash) (*merkletree.Proof, error) {
	claimType, claimVersion := core.GetClaimTypeVersionFromData(leafData)

	leafDataCpy := &merkletree.Data{}
	copy(leafDataCpy[:], leafData[:])
	core.SetClaimTypeVersionInData(leafDataCpy, claimType, claimVersion+1)
	entry := merkletree.Entry{
		Data: *leafDataCpy,
	}
	proof, err := mt.GenerateProof(entry.HIndex())
	if err != nil {
		return nil, err
	}
	if proof.Existence {
		return nil, ErrRevokedClaim
	}
	return proof, nil
}

// getNonRevocationProof returns the next version Hi (that don't exist in the tree, it's value is Empty) with merkleproof and root
func getNonRevocationProof(mt *merkletree.MerkleTree, hi merkletree.Hash) (ProofOfTreeLeaf, error) {
	// var value merkletree.Value

	// get claim value in bytes
	leafData, err := mt.GetDataByIndex(&hi)
	if err != nil {
		return ProofOfTreeLeaf{}, err
	}

	claimType, _ := core.GetClaimTypeVersionFromData(leafData)
	nextVersion, err := GetNextVersion(mt, &hi)
	if err != nil {
		return ProofOfTreeLeaf{}, err
	}

	core.SetClaimTypeVersionInData(leafData, claimType, nextVersion)

	entry := merkletree.Entry{
		Data: *leafData,
	}
	mp, err := mt.GenerateProof(entry.HIndex())
	if err != nil {
		return ProofOfTreeLeaf{}, err
	}
	leafBytes := entry.Bytes()
	nonRevocationProof := ProofOfTreeLeaf{
		Leaf:  leafBytes[:],
		Proof: mp.Bytes(),
		Root:  *mt.RootKey(),
	}
	return nonRevocationProof, nil
}

// GetNextVersion returns the next version of a claim, given a Hash(index)
func GetNextVersion(mt *merkletree.MerkleTree, hi *merkletree.Hash) (uint32, error) {
	var claimType core.ClaimType
	var version uint32

	// loop until we find a nextversion that don't exist
	for {
		leafData, err := mt.GetDataByIndex(hi)
		if err == merkletree.ErrEntryIndexNotFound {
			return version, nil
		} else if err != nil {
			return 0, err
		}
		claimType, version = core.GetClaimTypeVersionFromData(leafData)
		version++

		core.SetClaimTypeVersionInData(leafData, claimType, version)

		entry := merkletree.Entry{
			Data: *leafData,
		}
		hi = entry.HIndex()
	}
}
