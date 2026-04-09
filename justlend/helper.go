package justlend

import (
	"crypto/sha256"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/golang/protobuf/proto"
)

// PrivateKeyToPublicAddress private key import base58 address
func PrivateKeyToPublicAddress(pk string) (string, error) {
	sk, err := crypto.HexToECDSA(pk)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	return address.PubkeyToAddress(sk.PublicKey).String(), nil
}

// Signature 签名
func Signature(privateKey string, tx *api.TransactionExtention) (*api.TransactionExtention, error) {
	rowData, err := proto.Marshal(tx.GetTransaction().GetRawData())
	if err != nil {
		return nil, fmt.Errorf("marshal transaction failed: %w", err)
	}

	sk, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	h256h := sha256.New()
	h256h.Write(rowData)
	hash := h256h.Sum(nil)
	sig, err := crypto.Sign(hash, sk)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}
	tx.Transaction.Signature = append(tx.Transaction.Signature, sig)
	return tx, nil
}
