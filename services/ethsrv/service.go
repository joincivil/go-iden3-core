package ethsrv

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/iden3/go-iden3-core/core"
	"github.com/iden3/go-iden3-core/eth"
	"github.com/iden3/go-iden3-core/eth/contracts"
	"github.com/iden3/go-iden3-core/merkletree"
)

type Service interface {
	// Smart contract calls
	GetRoot(id *core.ID) (*core.RootData, error)
	GetRootByBlock(id *core.ID, blockN uint64) (merkletree.Hash, error)
	GetRootByTime(id *core.ID, blockTimestamp int64) (merkletree.Hash, error)

	Client() *eth.Client2
}

type ContractAddresses struct {
	RootCommits common.Address
}

type ServiceImpl struct {
	client    *eth.Client2
	addresses ContractAddresses
}

func New(client *eth.Client2, addresses ContractAddresses) *ServiceImpl {
	return &ServiceImpl{
		client:    client,
		addresses: addresses,
	}
}

func (s *ServiceImpl) GetRoot(id *core.ID) (*core.RootData, error) {
	var root [32]byte
	var blockN uint64
	var blockTS uint64
	err := s.client.Call(func(c *ethclient.Client) error {
		rootcommits, err := contracts.NewRootCommits(s.addresses.RootCommits, c)
		if err != nil {
			return err
		}
		blockN, blockTS, root, err = rootcommits.GetRootDataById(nil, *id)
		return err
	})
	return &core.RootData{
		BlockN:         blockN,
		BlockTimestamp: int64(blockTS),
		Root:           (*merkletree.Hash)(&root),
	}, err
}

func (s *ServiceImpl) GetRootByBlock(id *core.ID, blockN uint64) (merkletree.Hash, error) {
	var root [32]byte
	err := s.client.Call(func(c *ethclient.Client) error {
		rootcommits, err := contracts.NewRootCommits(s.addresses.RootCommits, c)
		if err != nil {
			return err
		}
		root, err = rootcommits.GetRootByBlock(nil, *id, blockN)
		return err
	})
	return merkletree.Hash(root), err
}

func (s *ServiceImpl) GetRootByTime(id *core.ID, blockTimestamp int64) (merkletree.Hash, error) {
	var root [32]byte
	err := s.client.Call(func(c *ethclient.Client) error {
		rootcommits, err := contracts.NewRootCommits(s.addresses.RootCommits, c)
		if err != nil {
			return err
		}
		root, err = rootcommits.GetRootByTime(nil, *id, uint64(blockTimestamp))
		return err
	})
	return merkletree.Hash(root), err
}

func (s *ServiceImpl) Client() *eth.Client2 {
	return s.client
}
