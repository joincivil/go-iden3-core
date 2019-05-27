package genericserver

import (
	"strings"

	// common3 "github.com/iden3/go-iden3/common"
	"github.com/iden3/go-iden3/core"
	"github.com/iden3/go-iden3/crypto/babyjub"
	"github.com/spf13/viper"
	"github.com/urfave/cli"
)

type ContractInfo struct {
	JsonABI string
	Address string
}

type Config struct {
	Server struct {
		ServiceApi string
		AdminApi   string
	}
	Web3 struct {
		Url string
	}
	KeyStore struct {
		Path     string
		Address  string
		Password string
	}
	KeyStoreBaby struct {
		Path       string
		PubKeyRaw  string                `mapstructure:"pubkey"`
		PubKey     babyjub.PublicKey     `mapstructure:"-"`
		PubKeyComp babyjub.PublicKeyComp `mapstructure:"-"`
		Password   string
	}
	IdRaw     string  `mapstructure:"id"`
	Id        core.ID `mapstructure:"-"`
	Contracts struct {
		RootCommits   ContractInfo
		Iden3Impl     ContractInfo
		Iden3Deployer ContractInfo
		Iden3Proxy    ContractInfo
	}
	Storage struct {
		Path string
	}
	Domain    string
	Namespace string
	Names     struct {
		Path string
	}
	Entitites struct {
		Path string
	}
}

var C Config

func MustRead(c *cli.Context) error {

	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath(".")    // adding home directory as first search path
	viper.SetEnvPrefix("iden3") // so viper.AutomaticEnv will get matching envvars starting with O2M_
	viper.AutomaticEnv()        // read in environment variables that match
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if c.GlobalString("config") != "" {
		viper.SetConfigFile(c.GlobalString("config"))
	}

	if err := viper.ReadInConfig(); err != nil {
		return err
	}
	if err := viper.Unmarshal(&C); err != nil {
		return err
	}
	var err error
	if C.Id, err = core.IDFromString(C.IdRaw); err != nil {
		return err
	}
	if err := C.KeyStoreBaby.PubKey.UnmarshalText([]byte(C.KeyStoreBaby.PubKeyRaw)); err != nil {
		return err
	}
	C.KeyStoreBaby.PubKeyComp = C.KeyStoreBaby.PubKey.Compress()
	return nil
}
