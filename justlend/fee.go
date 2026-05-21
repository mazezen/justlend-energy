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
// 返回 map 说明：
// prepayCost     → *big.Int，你实际要支付的费用 (sun)，用于 msg.value
// minRefund      → *big.Int，最少可退还金额 (sun)，= 最低6h租金 + penalty
// minRefundTrx   → string，最少可退还金额 (TRX, 格式化)
// trxAmount      → string，合约 rentResource 的 amount 参数 (TRX)
// trxAmountSun   → string，合约 rentResource 的 amount 参数 (sun)
// energyPerTrx   → string，完整小数字符串，如 "9.21052404"
// rentalEnergy   → string，用户输入的能量数量
// EstimateRentCost 计算租赁费用
// typ: 1=首租, 2=续租
// 首租: rentalEnergy>0 && durationHours>0
// 续租: rentalEnergy>=0 && durationHours>=0, 至少一个 >0
func (e *EnergyRental) EstimateRentCost(rentalEnergy string, durationHours int, resourceType ResourceType, typ int) (map[string]interface{}, error) {
	if typ != 1 && typ != 2 {
		return nil, fmt.Errorf("typ must be 1 (first-time rental) or 2 (renewal)")
	}
	if typ == 1 {
		// 首租：两个参数都必须 > 0
		if utils.LessThanOrEqual(rentalEnergy, "0") {
			return nil, fmt.Errorf("rental_energy must be > 0")
		}
		if durationHours <= 0 {
			return nil, fmt.Errorf("duration_hours must be > 0")
		}
	} else {
		// 续租：两个参数都必须 >= 0，至少一个 > 0
		energy, _ := new(big.Float).SetString(rentalEnergy)
		if energy == nil || energy.Sign() < 0 {
			return nil, fmt.Errorf("rental_energy must be >= 0")
		}
		if durationHours < 0 {
			return nil, fmt.Errorf("duration_hours must be >= 0")
		}
		if (energy.Sign() == 0) && durationHours == 0 {
		return nil, fmt.Errorf("must be > 0 at least one of both which rental_energy and duration_hours")
		}
	}

	// 1. 获取精确汇率
	energyStakePerTrxFloat, err := e.getEnergyStakePerTrxFloat()
	if err != nil {
		return nil, fmt.Errorf("failed to get energyStakePerTrx: %w", err)
	}
	energyPerTrxStr := utils.Float64ToString(energyStakePerTrxFloat, 8)

	// 2. 计算 trxAmount (精确计算)
	trxAmount := utils.Div(rentalEnergy, energyPerTrxStr, 6)
	if utils.LessThan(rentalEnergy, "1") {
		return nil, fmt.Errorf("resource rent: resource amount must be no less than 1TRX")
	}
	trxAmountSun := utils.Mul(trxAmount, "1e6", 0)

	fmt.Printf("[DEGBU] trxAmount: %s\n", trxAmount)
	fmt.Printf("[DEGBU] trxAmountSun: %s\n", trxAmountSun)

	// 3. 合约最低要求：amount >= 1 TRX
	if utils.LessThanOrEqual(trxAmount, "0") {
		return nil, fmt.Errorf(
			"rental_energy too small. Current rate ≈ %.8f Energy/TRX. Minimum rental_energy required ≈ %s",
			energyStakePerTrxFloat, energyPerTrxStr)
	}

	trxAmountBigInt := utils.StringToBigInt(trxAmountSun)

	// 4. 取两类费率的最大值
	rentalRate, err := e.getRentalRate(resourceType)
	if err != nil {
		return nil, fmt.Errorf("get rental rate failed: %w", err)
	}
	stableRate, err := e.getStableRate(resourceType)
	if err != nil {
		return nil, fmt.Errorf("get stable rate failed: %w", err)
	}
	maxRate := rentalRate
	if stableRate.Cmp(rentalRate) > 0 {
		maxRate = stableRate
	}
	dailyRate := new(big.Float).Quo(new(big.Float).SetInt(maxRate), big.NewFloat(1e18))
	dailyRate = dailyRate.Mul(dailyRate, big.NewFloat(86400))

	// 5. 取清算阈值
	liquidateThresholdBI, err := e.getLiquidateThreshold()
	if err != nil {
		return nil, fmt.Errorf("get liquidate threshold failed: %w", err)
	}

	// 6. EnergyBillExtras: 长租(≥23h 或整日)加 2h buffer
	durationSeconds := int64(durationHours) * 3600
	wholeDayRental := durationSeconds >= 86400 && durationSeconds%86400 == 0
	longHourlyRental := durationSeconds >= 82800
	buffer := int64(0)
	if wholeDayRental || longHourlyRental {
		buffer = 7200
	}
	energyBillSeconds := durationSeconds + buffer

	// 7. 计算 energyFee = trxAmount * maxRate * (energyBillSeconds + liquidateThreshold) / 1e18
	// = trxAmount * dailyRate * (energyBillSeconds + liquidateThreshold) / 86400
	timeSeconds := new(big.Float).SetInt64(energyBillSeconds)
	timeSeconds.Add(timeSeconds, new(big.Float).SetInt(liquidateThresholdBI))
	energyFee := new(big.Float).Mul(new(big.Float).SetInt(trxAmountBigInt), dailyRate)
	energyFee = energyFee.Quo(energyFee, big.NewFloat(86400))
	energyFee = energyFee.Mul(energyFee, timeSeconds)

	// 8. Security Deposit = trxAmount * maxRate * 86400 / 1e18 = trxAmount * dailyRate（1天的租金作为押金）
	securityDeposit := new(big.Float).Mul(new(big.Float).SetInt(trxAmountBigInt), dailyRate)

	// 9. Penalty = trxAmount * 8 / 100000 (0.008%), 用 big.Int 整数运算, 带上取整, min 20 TRX = 20_000_000 sun
	penaltyInt := new(big.Int).Mul(trxAmountBigInt, big.NewInt(8))
	penaltyInt.Add(penaltyInt, big.NewInt(99999)) // ceiling: + (denom-1)
	penaltyInt.Quo(penaltyInt, big.NewInt(100000))
	if penaltyInt.Cmp(big.NewInt(20_000_000)) < 0 {
		penaltyInt = big.NewInt(20_000_000)
	}
	penalty := new(big.Float).SetInt(penaltyInt)

	// 10. 最终支付金额
	total := new(big.Float).Add(energyFee, securityDeposit)
	total = new(big.Float).Add(total, penalty)

	prepayCost := new(big.Int)
	total.Int(prepayCost)
	fmt.Printf("[DEBUG] prepayCost = %s energyPerTrxStr=%s trxAmount=%s rentalEnergy=%s\n", prepayCost.String(), energyPerTrxStr, trxAmount, rentalEnergy)

	// 11. 最少退还 = 最低6小时(21600s)租金 + 罚金
	// 公式：trxAmount * maxRate * (21600 + liquidateThreshold) / 1e18 + penalty
	minTimeSeconds := new(big.Float).SetInt64(21600)
	minTimeSeconds.Add(minTimeSeconds, new(big.Float).SetInt(liquidateThresholdBI))
	minRefund := new(big.Float).Mul(new(big.Float).SetInt(trxAmountBigInt), dailyRate)
	minRefund = minRefund.Quo(minRefund, big.NewFloat(86400))
	minRefund = minRefund.Mul(minRefund, minTimeSeconds)
	minRefund = new(big.Float).Add(minRefund, penalty)
	if minRefund.Sign() < 0 {
		minRefund = new(big.Float)
	}
	minRefundSun := new(big.Int)
	minRefund.Int(minRefundSun)
	minRefundTrx := utils.Div(minRefundSun.String(), "1e6", 6)

	return map[string]interface{}{
		"prepayCost":    prepayCost,
		"minRefund":     minRefundSun,  // 最少退还 (sun)
		"minRefundTrx":  minRefundTrx,  // 最少退还 (TRX, 格式化字符串)
		"trxAmount":     trxAmount,
		"trxAmountSun":  trxAmountSun,
		"energyPerTrx":  energyPerTrxStr,
		"rentalEnergy":  rentalEnergy,
	}, nil
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
		return nil, fmt.Errorf("TriggerConstantContract _rentalRate failed: %w", err)
	}

	if len(result.ConstantResult) == 0 || len(result.ConstantResult[0]) == 0 {
		return nil, fmt.Errorf("empty result from _rentalRate")
	}

	rate := new(big.Int).SetBytes(result.ConstantResult[0])
	return rate, nil
}

// getStableRate 取稳定租赁费率 _stableRate(uint256)
func (e *EnergyRental) getStableRate(resourceType ResourceType) (*big.Int, error) {
	jsonParams := fmt.Sprintf(`["%d"]`, uint64(resourceType))
	result, err := e.client.TriggerConstantContract(
		"",
		e.contractAddr,
		"_stableRate(uint256)",
		jsonParams,
	)
	if err != nil {
		return nil, fmt.Errorf("TriggerConstantContract _stableRate failed: %w", err)
	}

	if len(result.ConstantResult) == 0 || len(result.ConstantResult[0]) == 0 {
		return nil, fmt.Errorf("empty result from _stableRate")
	}

	rate := new(big.Int).SetBytes(result.ConstantResult[0])
	return rate, nil
}

// getLiquidateThreshold 取清算阈值时间（秒）
func (e *EnergyRental) getLiquidateThreshold() (*big.Int, error) {
	result, err := e.client.TriggerConstantContract(
		"",
		e.contractAddr,
		"liquidateThreshold()",
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("TriggerConstantContract liquidateThreshold failed: %w", err)
	}

	if len(result.ConstantResult) == 0 || len(result.ConstantResult[0]) == 0 {
		return nil, fmt.Errorf("empty result from liquidateThreshold")
	}

	threshold := new(big.Int).SetBytes(result.ConstantResult[0])
	return threshold, nil
}
