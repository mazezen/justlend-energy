package justlend

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/mazezen/justlend-energy/utils"
)

// RentResource 租赁资源（输入 rentalEnergy + durationHours）
// renterPrivateKeyHex string, // 付款者（renter）的私钥，必须提供
// receiver string,            // 接收能量的地址（可与 renter 相同）
// rentalEnergy string,        // 租赁的能量数量（例如 10000 能量）
// durationHours int,          // 租赁时长（小时）
// resourceType ResourceType,
// extraDepositSun *big.Int, // 额外保证金（sun），可传 nil 或 0
func (e *EnergyRental) RentResource(
	renterPrivateKeyHex,
	receiver string,
	rentalEnergy string,
	durationHours int,
	resourceType ResourceType,
	extraDepositSun *big.Int,
) (string, error) {
	if extraDepositSun == nil {
		extraDepositSun = big.NewInt(0)
	}
	if strings.TrimSpace(receiver) == "" {
		return "", fmt.Errorf("receiver address cannot be empty")
	}

	fromAddr, err := PrivateKeyToPublicAddress(renterPrivateKeyHex) // from/ownerAddress
	if err != nil {
		return "", err
	}

	// 1. 费用预估
	prepayCost, trxAmount, _, _, err := e.EstimateRentCost(rentalEnergy, durationHours, resourceType)
	if err != nil {
		return "", fmt.Errorf("estimate rent cost failed: %w", err)
	}

	// 2. 如果额外提供了保证金，则累加（大多数情况 extraDepositSun = 0）
	callValue := new(big.Int).Add(prepayCost, extraDepositSun)

	fmt.Printf("[DEBUG] fromAddr (renter) = %s\n", fromAddr)
	fmt.Printf("[DEBUG] receiver = %s\n", receiver)
	fmt.Printf("[DEBUG] trxAmount = %s sun\n", trxAmount)
	fmt.Printf("[DEBUG] callValue = %s sun\n", callValue.String())

	trxAmountSun := utils.Mul(trxAmount, "1e6", 0)
	jsonParams := fmt.Sprintf(`["%s", "%s", "%d"]`, receiver, trxAmountSun, uint64(resourceType))
	fmt.Printf("[DEBUG] jsonParams = %s\n", jsonParams)

	// 3. 调用合约（payable）
	tx, err := e.client.TriggerContract(
		fromAddr,                                // 从私钥推导 ownerAddress
		e.contractAddr,                          // Base58 地址
		"rentResource(address,uint256,uint256)", // method rentResource(address,uint256,uint256)
		jsonParams,                              // 使用数组格式 ["addr", "amount", "type"]
		150_000_000,                             // feeLimit (sun)，建议设大一点，如 150_000_000
		callValue.Int64(),                       // msg.value = 要支付的总费用（sun）
		"",                                      // tTokenID（TRC10 token id，留空）
		0,                                       // tTokenAmount
	)
	if err != nil {
		return "", fmt.Errorf("trigger rentResource failed: %w", err)
	}

	// 4. 使用私钥签名
	tx, err = Signature(renterPrivateKeyHex, tx)
	if err != nil {
		return "", fmt.Errorf("signature failed: %w", err)
	}

	result, err := e.client.Broadcast(tx.Transaction)
	if err != nil {
		return "", fmt.Errorf("broadcast failed: %w", err)
	}

	if result.Code != api.Return_SUCCESS {
		return "", fmt.Errorf("broadcast error: %s", result.Message)
	}

	txID := hex.EncodeToString(tx.Txid)
	return txID, nil
}
