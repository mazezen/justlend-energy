package justlend

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/mazezen/justlend-energy/utils"
)

// EstimateRentCost 计算租赁费用
// 返回值说明：
// prepayCost     → 你实际要支付的费用 (sun)，用于 msg.value
// trxAmount      → 合约 rentResource 的 amount 参数 (sun)
// energyPerTrx   → 完整小数字符串，如 "9.21052404"
// rentalEnergy   → 用户输入的能量数量
func (e *EnergyRental) EstimateRentCost(rentalEnergy int64, durationHours int, resourceType ResourceType) (*big.Int, *big.Int, string, int64, error) {
	if rentalEnergy <= 0 {
		return nil, nil, "", 0, fmt.Errorf("rental_energy must be > 0")
	}
	if durationHours <= 0 {
		return nil, nil, "", 0, fmt.Errorf("duration_hours must be > 0")
	}

	// 1. 获取精确汇率
	energyStakePerTrxFloat, err := e.getEnergyStakePerTrxFloat()
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("failed to get energyStakePerTrx: %w", err)
	}
	energyPerTrxStr := utils.Float64ToString(energyStakePerTrxFloat, 8)

	// 2. 计算 trxAmount (精确计算)
	rentalEnergyStr := utils.Int64ToString(rentalEnergy, 2)
	div := utils.Div(rentalEnergyStr, energyPerTrxStr, 8)
	if utils.LessThan(div, "1") {
		return nil, nil, "", 0, fmt.Errorf("resource rent: resource amount must be no less than 1TRX")
	}
	trxAmountStr := utils.Mul(div, "1e6", 8)
	fmt.Printf("[DEGBU] trxAmount: %s\n", trxAmountStr)

	// 3. 合约最低要求：amount >= 1 TRX
	if utils.LessThanOrEqual(trxAmountStr, "0") {
		minEnergy := new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(int64(energyStakePerTrxFloat)))
		return nil, nil, energyPerTrxStr, rentalEnergy, fmt.Errorf(
			"rental_energy too small. Current rate ≈ %.8f Energy/TRX. Minimum rental_energy required ≈ %d",
			energyStakePerTrxFloat, minEnergy.Int64())
	}
	trxAmount := utils.StringToBigInt(trxAmountStr)

	// 4. 计算基础租金（很小）
	durationDays := float64(durationHours) / 24.0
	rentalRate, _ := e.getRentalRate(resourceType)
	dailyRate := new(big.Float).Quo(new(big.Float).SetInt(rentalRate), big.NewFloat(1e18))
	dailyRate = dailyRate.Mul(dailyRate, big.NewFloat(86400))
	energyFee := new(big.Float).Mul(new(big.Float).SetInt(trxAmount), dailyRate)
	energyFee = energyFee.Mul(energyFee, big.NewFloat(durationDays))

	// 5. Security Deposit（保证金）
	securityDeposit := new(big.Float).Mul(new(big.Float).SetInt(trxAmount), big.NewFloat(2)) // 1-2倍
	minDeposit := big.NewFloat(30)

	if securityDeposit.Cmp(minDeposit) < 0 {
		securityDeposit = minDeposit
	}

	// 6. Liquidation Penalty（官方公式）
	penalty := new(big.Float).Mul(new(big.Float).SetInt(trxAmount), big.NewFloat(0.00008))
	minPenalty := big.NewFloat(20)
	if penalty.Cmp(minPenalty) < 0 {
		penalty = minPenalty
	}

	// 7. 最终支付金额
	total := new(big.Float).Add(energyFee, securityDeposit)
	total = new(big.Float).Add(total, penalty)

	prepayCost := new(big.Int)
	total.Int(prepayCost)
	fmt.Printf("[DEBUG] prepayCost = %s energyPerTrxStr=%s trxAmount=%s rentalEnergy=%s\n", prepayCost.String(), energyPerTrxStr, trxAmountStr, rentalEnergyStr)

	return prepayCost, trxAmount, energyPerTrxStr, rentalEnergy, nil
}

type DashboardResponse struct {
	Data struct {
		EnergyStakePerTrx string `json:"energyStakePerTrx"`
		EnergyRentPerTrx  string `json:"energyRentPerTrx"`
	} `json:"data"`
}

// getEnergyStakePerTrx
// getEnergyStakePerTrxFloat 能量单价查询: 1 TRX ≈ ? 返回带完整小数的 energyStakePerTrx
func (e *EnergyRental) getEnergyStakePerTrxFloat() (float64, error) {
	if e == nil {
		return 0, fmt.Errorf("nil pointer")
	}

	e.cacheMutex.RLock()
	if time.Since(e.cacheTime) < 30*time.Second && e.cacheEnergyStakePerTrx > 0 {
		val := e.cacheEnergyStakePerTrx
		e.cacheMutex.RUnlock()
		return val, nil
	}
	e.cacheMutex.RUnlock()

	resp, err := http.Get(dashboardUrl)
	if err != nil {
		return 0.0, fmt.Errorf("failed to fetch dashboard: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0.0, err
	}

	var dashboard DashboardResponse
	json.Unmarshal(body, &dashboard)

	fmt.Printf("[DEBUG] Raw EnergyStakePerTrx from dashboard = %s\n", dashboard.Data.EnergyStakePerTrx)
	f := new(big.Float)
	f.SetString(dashboard.Data.EnergyStakePerTrx)
	result, _ := f.Float64()

	e.cacheMutex.Lock()
	e.cacheEnergyStakePerTrx = result
	e.cacheTime = time.Now()
	e.cacheMutex.Unlock()

	return result, nil
}

// getRentalRate 从合约调用 _rentalRate(uint256 amount, uint256 resourceType)
func (e *EnergyRental) getRentalRate(resourceType ResourceType) (*big.Int, error) {
	jsonParams := fmt.Sprintf(`["0", "%d"]`, uint64(resourceType))
	result, err := e.client.TriggerConstantContract(
		"",
		e.contractAddr,
		"_rentalRate(uint256,uint256)",
		jsonParams,
	)
	if err != nil {
		return nil, fmt.Errorf("TriggerConstantContract failed: %w", err)
	}

	if len(result.ConstantResult) == 0 || len(result.ConstantResult[0]) == 0 {
		return nil, fmt.Errorf("empty result from _rentalRate")
	}

	rate := new(big.Int).SetBytes(result.ConstantResult[0])
	return rate, nil
}
