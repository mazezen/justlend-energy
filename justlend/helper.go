package justlend

import (
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
)

// PrivateKeyToPublicAddress private key import base58 address
func PrivateKeyToPublicAddress(pk string) (string, error) {
	sk, err := crypto.HexToECDSA(pk)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	return address.PubkeyToAddress(sk.PublicKey).String(), nil
}
