package justlend

import (
	"fmt"
	"math/big"
)

type RentalInfo struct {
	Amount          *big.Int // 委托的 TRX 数量 (sun)
	SecurityDeposit *big.Int // 保证金
	RentIndex       *big.Int // 租金指数
}

// GetRentInfo 查询订单信息（推荐使用）
// renter:   付款者地址
// receiver: 能量接收者地址
// resourceType
func (e *EnergyRental) GetRentInfo(renter, receiver string, resourceType ResourceType) (*RentalInfo, error) {
	fmt.Printf("[DEBUG] GetFullRentalInfo -> renter: %s | receiver: %s | resourceType: %d\n", renter, receiver, resourceType)

	// 1. 构造参数
	jsonParams := fmt.Sprintf(`["%s", "%s", "%d"]`, renter, receiver, uint64(resourceType))

	// 2. 调用合约
	result, err := e.client.TriggerConstantContract(
		renter,
		e.contractAddr,
		"rentals(address,address,uint256)",
		jsonParams,
	)
	if err != nil {
		return nil, fmt.Errorf("TriggerConstantContract rentals failed: %w", err)
	}

	if len(result.ConstantResult) == 0 || len(result.ConstantResult[0]) == 0 {
		return nil, fmt.Errorf("no rental info found (order may not exist)")
	}

	data := result.ConstantResult[0]
	fmt.Printf("[DEBUG] ConstantResult length = %d bytes\n", len(data))

	// ABI 编码 struct：3 个 uint256，每个 32 字节，共 96 字节
	if len(data) < 96 {
		return nil, fmt.Errorf("invalid struct data, expected at least 96 bytes, got %d", len(data))
	}

	// 解析三个字段
	amount := new(big.Int).SetBytes(data[0:32])
	securityDeposit := new(big.Int).SetBytes(data[32:64])
	rentIndex := new(big.Int).SetBytes(data[64:96])

	fmt.Printf("[DEBUG] Amount          = %s sun\n", amount.String())
	fmt.Printf("[DEBUG] SecurityDeposit = %s sun\n", securityDeposit.String())
	fmt.Printf("[DEBUG] RentIndex       = %s\n", rentIndex.String())

	return &RentalInfo{
		Amount:          amount,
		SecurityDeposit: securityDeposit,
		RentIndex:       rentIndex,
	}, nil
}
