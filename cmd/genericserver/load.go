package genericserver

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	ethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/iden3/go-iden3/core"
	"github.com/iden3/go-iden3/crypto/babyjub"
	"github.com/iden3/go-iden3/db"
	"github.com/iden3/go-iden3/eth"
	babykeystore "github.com/iden3/go-iden3/keystore"
	"github.com/iden3/go-iden3/merkletree"
	"github.com/iden3/go-iden3/services/adminsrv"
	"github.com/iden3/go-iden3/services/claimsrv"
	"github.com/iden3/go-iden3/services/counterfactualsrv"
	"github.com/iden3/go-iden3/services/identitysrv"
	"github.com/iden3/go-iden3/services/rootsrv"
	"github.com/iden3/go-iden3/services/signedpacketsrv"
	"github.com/iden3/go-iden3/services/signsrv"
	log "github.com/sirupsen/logrus"
)

var (
	dbMerkletreePrefix     = []byte{0}
	dbCounterfactualPrefix = []byte{1}
)

const (
	passwdPrefix = "passwd:"
	filePrefix   = "file:"
)

func Assert(msg string, err error) {
	if err != nil {
		log.Error(msg, " ", err.Error())
		os.Exit(1)
	}
}

var Claimservice claimsrv.Service
var Rootservice rootsrv.Service
var Idservice identitysrv.Service
var Counterfactualservice counterfactualsrv.Service
var Adminservice adminsrv.Service

var SignedPacketService signedpacketsrv.SignedPacketSigner

func LoadKeyStore() (*ethkeystore.KeyStore, accounts.Account) {

	var err error
	var passwd string

	// Load keystore
	ks := ethkeystore.NewKeyStore(C.KeyStore.Path, ethkeystore.StandardScryptN, ethkeystore.StandardScryptP)

	// Password can be prefixed by two options
	//   file: <path to file containing the password>
	//   passwd: raw password
	// if is not prefixed by any of those, file: is used
	if strings.HasPrefix(C.KeyStore.Password, passwdPrefix) {
		passwd = C.KeyStore.Password[len(passwdPrefix):]
	} else {
		filename := C.KeyStore.Password
		if strings.HasPrefix(filename, filePrefix) {
			filename = C.KeyStore.Password[len(filePrefix):]
		}
		passwdbytes, err := ioutil.ReadFile(filename)
		Assert("Cannot read password ", err)
		passwd = string(passwdbytes)
	}

	acc, err := ks.Find(accounts.Account{
		Address: common.HexToAddress(C.KeyStore.Address),
	})
	Assert("Cannot find keystore account", err)

	Assert("Cannot unlock account", ks.Unlock(acc, string(passwd)))
	log.WithField("acc", acc.Address.Hex()).Info("Keystore and account unlocked successfully")

	return ks, acc
}

func LoadKeyStoreBabyJub() (*babykeystore.KeyStore, *babyjub.PublicKeyComp) {
	storage := babykeystore.NewFileStorage(C.KeyStoreBaby.Path)
	ks, err := babykeystore.NewKeyStore(storage, babykeystore.StandardKeyStoreParams)
	if err != nil {
		panic(err)
	}
	pk := &C.KeyStoreBaby.PubKeyComp
	if err := ks.UnlockKey(pk, []byte(C.KeyStoreBaby.Password)); err != nil {
		panic(err)
	}
	return ks, pk
}

func LoadWeb3(ks *ethkeystore.KeyStore, acc *accounts.Account) *eth.Web3Client {
	// Create geth client
	url := C.Web3.Url
	hidden := strings.HasPrefix(url, "hidden:")
	if hidden {
		url = url[len("hidden:"):]
	}
	web3cli, err := eth.NewWeb3Client(url, ks, acc)
	Assert("Cannot open connection to web3 ", err)
	if hidden {
		log.WithField("url", "(hidden)").Info("Connection to web3 server opened")
	} else {
		log.WithField("url", C.Web3.Url).Info("Connection to web3 server opened")
	}
	return web3cli
}

func LoadStorage() db.Storage {
	// Open database
	storage, err := db.NewLevelDbStorage(C.Storage.Path, false)
	Assert("Cannot open storage", err)
	log.WithField("path", C.Storage.Path).Info("Storage opened")
	return storage
}

func LoadMerkele(storage db.Storage) *merkletree.MerkleTree {
	mtstorage := storage.WithPrefix(dbMerkletreePrefix)
	mt, err := merkletree.NewMerkleTree(mtstorage, 140)
	Assert("Cannot open merkle tree", err)
	log.WithField("hash", mt.RootKey().Hex()).Info("Current root")

	return mt
}

func LoadContract(client eth.Client, jsonabifile string, address *string) *eth.Contract {
	abiFile, err := os.Open(jsonabifile)
	Assert("Cannot read contract "+jsonabifile, err)

	abi, code, err := eth.UnmarshallSolcAbiJson(abiFile)
	Assert("Cannot parse contract "+jsonabifile, err)

	var addrPtr *common.Address
	if address != nil && len(strings.TrimSpace(*address)) > 0 {
		addr := common.HexToAddress(strings.TrimSpace(*address))
		addrPtr = &addr
	}
	return eth.NewContract(client, abi, code, addrPtr)
}

func LoadRootsService(client *eth.Web3Client) rootsrv.Service {
	return rootsrv.New(LoadContract(
		client,
		C.Contracts.RootCommits.JsonABI,
		&C.Contracts.RootCommits.Address,
	))
}

func LoadIdentityService(claimservice claimsrv.Service) identitysrv.Service {
	return identitysrv.New(claimservice)
}

func LoadCounterfactualService(client *eth.Web3Client, claimservice claimsrv.Service, storage db.Storage) counterfactualsrv.Service {

	counterfactualstorage := storage.WithPrefix(dbCounterfactualPrefix)

	deployerContract := LoadContract(
		client,
		C.Contracts.Iden3Deployer.JsonABI,
		&C.Contracts.Iden3Deployer.Address)

	implContract := LoadContract(
		client,
		C.Contracts.Iden3Impl.JsonABI,
		&C.Contracts.Iden3Impl.Address)

	proxyContract := LoadContract(
		client,
		C.Contracts.Iden3Proxy.JsonABI,
		nil)

	return counterfactualsrv.New(deployerContract, implContract, proxyContract, claimservice, counterfactualstorage)
}

func LoadClaimService(mt *merkletree.MerkleTree, rootservice rootsrv.Service, ks *babykeystore.KeyStore, pk *babyjub.PublicKey) claimsrv.Service {
	log.WithField("id", C.IdRaw).Info("Running claim service")
	signer := signsrv.New(ks, *pk)
	return claimsrv.New(C.Id, mt, rootservice, *signer)
}

func LoadAdminService(mt *merkletree.MerkleTree, rootservice rootsrv.Service, claimservice claimsrv.Service) adminsrv.Service {
	return adminsrv.New(mt, rootservice, claimservice)
}

// LoadSignedPacketSigner Adds new claim authorizing a secp256ks key. Returns SignedPacketSigner to sign with key sign.
func LoadSignedPacketSigner(ks *babykeystore.KeyStore, pk *babyjub.PublicKey, claimservice claimsrv.Service) *signedpacketsrv.SignedPacketSigner {
	// Create signer object
	signer := signsrv.New(ks, *pk)
	// Claim authorizing public key baby jub and get its proofKsign
	claim := core.NewClaimAuthorizeKSignBabyJub(pk)
	// Add claim
	err := claimservice.AddClaim(claim)
	if (err != nil) && (err != merkletree.ErrEntryIndexAlreadyExists) {
		panic(err)
	}
	// return claim with proofs
	proofKSign, err := claimservice.GetClaimProofByHi(claim.Entry().HIndex())
	if err != nil {
		panic(err)
	}
	return signedpacketsrv.NewSignedPacketSigner(*signer, *proofKSign, C.Id)
}

func LoadGenesis() (*core.GenesisProofClaims, error) {
	kOp := C.Keys.BabyJub.KOp
	kDis := C.Keys.Ethereum.KDis
	kReen := C.Keys.Ethereum.KReen
	kUpdateRoot := C.Keys.Ethereum.KUpdateRoot
	id, proofClaims, err := core.CalculateIdGenesis(kOp, kDis, kReen, kUpdateRoot)

	if *id != C.Id {
		return nil, fmt.Errorf("Configuration id doesn't match calculated genesis id")
	}

	storage, err := db.NewLevelDbStorage(C.Storage.Path, false)
	if err != nil {
		return nil, fmt.Errorf("Error creating level DB storage: %v", err)
	}
	mtstorage := storage.WithPrefix(dbMerkletreePrefix)
	mt, err := merkletree.NewMerkleTree(mtstorage, 140)
	if err != nil {
		return nil, fmt.Errorf("Error creating merkle tree: %v", err)
	}

	kvs, err := mtstorage.List(1)
	if err != nil {
		return nil, fmt.Errorf("Error listing level DB storage: %v", err)
	}
	proofClaimsList := []core.ProofClaim{proofClaims.KOp, proofClaims.KDis,
		proofClaims.KReen, proofClaims.KUpdateRoot}
	if len(kvs) == 0 {
		// Merklee tree DB is empty
		// Add genesis claims to merkle tree
		for _, proofClaim := range proofClaimsList {
			if err := mt.Add(&merkletree.Entry{Data: *proofClaim.Leaf}); err != nil {
				return nil, fmt.Errorf("Error adding claim to merkle tree: %v", err)
			}
		}
	} else {
		// MerkleTree DB has already been initialized
		// Check that the geneiss claims are in the merkle tree
		for _, proofClaim := range proofClaimsList {
			entry := merkletree.Entry{Data: *proofClaim.Leaf}
			data, err := mt.GetDataByIndex(entry.HIndex())
			if err != nil {
				return nil, fmt.Errorf("Error getting claim from the merkle tree: %v", err)
			}
			if entry.Data.Equal(data) {
				return nil, fmt.Errorf("Claim from the merkle tree doesn't match the expected claim")
			}
		}
	}

	return proofClaims, nil
}
