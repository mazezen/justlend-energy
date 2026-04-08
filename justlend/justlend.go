package justlend

import (
	"fmt"
	"sync"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/client"
)

const (
	contractAddress = "TU2MJ5Veik1LRAgjeSzEdvmDYx7mefJZvd" // Mainnet EnergyRental 合约地址
	dashboardUrl    = "https://labc.ablesdxd.link/strx/dashboard"
)

type EnergyRental struct {
	client       *client.GrpcClient
	contractAddr string
	contractByte []byte

	cacheEnergyStakePerTrx float64
	cacheTime              time.Time
	cacheMutex             *sync.RWMutex
}

type ResourceType uint8

func NewEnergyRental(client *client.GrpcClient) (*EnergyRental, error) {
	if client == nil {
		return nil, fmt.Errorf("grpc client is nil")
	}
	contractByte, err := address.Base58ToAddress(contractAddress)
	if err != nil {
		return nil, err
	}
	return &EnergyRental{
		client:       client,
		contractAddr: contractAddress,
		contractByte: contractByte.Bytes(),

		cacheMutex: &sync.RWMutex{},
		cacheTime:  time.Time{},
	}, nil
}
