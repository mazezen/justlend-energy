package justlend

import (
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/mazezen/justlend-energy/utils"
)

// ReturnResource 退租（由 renter 调用）
// renterPrivateKeyHex string  必须是 renter（付款者）的私钥
// receiver string             接收能量的地址（订单中的 receiver）
// returnEnergy *big.Int       部分退还的 TRX 量，传 nil 或 0 表示全部退还
// resourceType ResourceType
func (e *EnergyRental) ReturnResource(renterPrivateKeyHex, receiver string, returnEnergy string, resourceType ResourceType) (string, error) {
	renter, err := PrivateKeyToPublicAddress(renterPrivateKeyHex)
	if err != nil {
		return "", err
	}

	// 1. 如果要全部退还,先查询当前订单的实际amount
	finalReturnAmount := returnEnergy
	if utils.LessThanOrEqual(returnEnergy, "0") {
		info, err := e.GetRentInfo(renter, receiver, resourceType)
		if err != nil {
			return "", err
		}
		if utils.Equal(info.Amount, "0") {
			return "", fmt.Errorf("there are currently no refundable orders (orders do not exist or have been fully refunded)")
		}
		finalReturnAmount = info.Amount
		fmt.Printf("[DEBUG] 全部退还 → 查询到当前 amount = %s sun\n", finalReturnAmount)
	}
	finalReturnAmount = utils.Mul(finalReturnAmount, "1e6", 0)
	jsonParams := fmt.Sprintf(`["%s","%s","%d"]`, receiver, finalReturnAmount, uint64(resourceType))

	// 2. 调用合约（非 payable，callValue = 0）
	tx, err := e.client.TriggerContract(
		renter,         // 从私钥推导 ownerAddress
		e.contractAddr, // Base58 地址字符串
		"returnResource(address,uint256,uint256)",
		jsonParams,  // 使用数组格式 ["addr", "amount", "type"]
		150_000_000, // feeLimit (sun)，建议设大一点，防止能量不足
		0,           // callValue = msg.value = 0（退租不需要支付 TRX）
		"",          // tTokenID（TRC10）
		0,           // tTokenAmount
	)

	if err != nil {
		return "", fmt.Errorf("TriggerContract returnResource failed: %w", err)
	}

	// 3. 使用私钥签名
	tx, err = Signature(renterPrivateKeyHex, tx)
	if err != nil {
		return "", fmt.Errorf("signature failed: %w", err)
	}

	// 4. 广播交易
	result, err := e.client.Broadcast(tx.Transaction)
	if err != nil {
		return "", fmt.Errorf("broadcast failed: %w", err)
	}

	if result.Code != api.Return_SUCCESS {
		return "", fmt.Errorf("broadcast error: %s", string(result.Message))
	}

	txID := hex.EncodeToString(tx.Txid)
	return txID, nil
}

// ReturnResourceByReceiver 由 receiver 调用退租
// receiverPrivateKeyHex    必须是 receiver（接收者）的私钥
// renter                   订单的付款者（renter）地址
// returnEnergy             部分退还的 TRX 量，传 nil 或 0 表示全部退还
// resourceType
func (e *EnergyRental) ReturnResourceByReceiver(
	receiverPrivateKeyHex string,
	renter string,
	returnEnergy string,
	resourceType ResourceType,
) (string, error) {
	receiver, err := PrivateKeyToPublicAddress(receiverPrivateKeyHex)
	if err != nil {
		return "", err
	}

	// 1. 如果要全部退还,先查询当前订单的实际amount
	finalReturnAmount := returnEnergy
	if utils.LessThanOrEqual(returnEnergy, "0") {
		info, err := e.GetRentInfo(renter, receiver, resourceType)
		if err != nil {
			return "", err
		}
		if utils.Equal(info.Amount, "0") {
			return "", fmt.Errorf("there are currently no refundable orders (orders do not exist or have been fully refunded)")
		}
		finalReturnAmount = info.Amount
		fmt.Printf("[DEBUG] 全部退还 → 查询到当前 amount = %s sun\n", finalReturnAmount)
	}
	finalReturnAmount = utils.Mul(finalReturnAmount, "1e6", 0)
	jsonParams := fmt.Sprintf(`["%s","%s","%d"]`, renter, finalReturnAmount, uint64(resourceType))

	// 2. 调用合约（非 payable）
	txExt, err := e.client.TriggerContract(
		receiver,
		e.contractAddr, // 使用 Base58 字符串地址（不是 contractBytes）
		"returnResourceByReceiver(address,uint256,uint256)",
		jsonParams,
		150_000_000, // feeLimit (sun)，建议设大一点，避免能量不足
		0,           // callValue = msg.value = 0（退租不需要支付 TRX）
		"",
		0,
	)
	if err != nil {
		return "", fmt.Errorf("TriggerContract returnResourceByReceiver failed: %w", err)
	}

	// 4. 使用私钥签名
	txExt, err = Signature(receiverPrivateKeyHex, txExt)
	if err != nil {
		return "", fmt.Errorf("signature failed: %w", err)
	}

	// 4. 广播交易
	result, err := e.client.Broadcast(txExt.Transaction)
	if err != nil {
		return "", fmt.Errorf("broadcast failed: %w", err)
	}

	if result.Code != api.Return_SUCCESS {
		return "", fmt.Errorf("broadcast error: %s", string(result.Message))
	}

	txID := hex.EncodeToString(txExt.Txid)
	return txID, nil
}
