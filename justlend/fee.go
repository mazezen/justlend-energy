package justlend

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/mazezen/justlend-energy/utils"
)// calcPrepayValues 内部函数：根据能量、时长、费率、清算阈值、trxAmount 计算预付明细
// 返回 (prepayCost, energyFee, securityDeposit, penalty, minRefund, energyBillSeconds)
func calcPrepayValues(trxAmountBigInt *big.Int, durationSeconds int64, maxRate *big.Int, liquidateThreshold *big.Int) (*big.Int, *big.Int, *big.Int, *big.Int, *big.Int, int64) {
	dailyRate := new(big.Float).Quo(new(big.Float).SetInt(maxRate), big.NewFloat(1e18))
	dailyRate = dailyRate.Mul(dailyRate, big.NewFloat(86400))

	// EnergyBillExtras: 长租(≥23h 或整日)加 2h buffer
	wholeDayRental := durationSeconds >= 86400 && durationSeconds%86400 == 0
	longHourlyRental := durationSeconds >= 82800
	buffer := int64(0)
	if wholeDayRental || longHourlyRental {
		buffer = 7200
	}
	energyBillSeconds := durationSeconds + buffer

	// 计算 energyFee = trxAmount * maxRate * (energyBillSeconds + liquidateThreshold) / 1e18
	timeSeconds := new(big.Float).SetInt64(energyBillSeconds)
	timeSeconds.Add(timeSeconds, new(big.Float).SetInt(liquidateThreshold))
	energyFee := new(big.Float).Mul(new(big.Float).SetInt(trxAmountBigInt), dailyRate)
	energyFee = energyFee.Quo(energyFee, big.NewFloat(86400))
	energyFee = energyFee.Mul(energyFee, timeSeconds)

	// Security Deposit = trxAmount * dailyRate（1天的租金作为押金）
	securityDeposit := new(big.Float).Mul(new(big.Float).SetInt(trxAmountBigInt), dailyRate)

	// Penalty = trxAmount * 8 / 100000 (0.008%), min 20 TRX = 20_000_000 sun
	penaltyInt := new(big.Int).Mul(trxAmountBigInt, big.NewInt(8))
	penaltyInt.Add(penaltyInt, big.NewInt(99999)) // ceiling
	penaltyInt.Quo(penaltyInt, big.NewInt(100000))
	if penaltyInt.Cmp(big.NewInt(20_000_000)) < 0 {
		penaltyInt = big.NewInt(20_000_000)
	}
	penalty := new(big.Float).SetInt(penaltyInt)

	// 最终支付金额
	total := new(big.Float).Add(energyFee, securityDeposit)
	total = new(big.Float).Add(total, penalty)

	prepayCost := new(big.Int)
	total.Int(prepayCost)

	// 最少退还 = 最低6小时(21600s)租金 + 罚金
	minTimeSeconds := new(big.Float).SetInt64(21600)
	minTimeSeconds.Add(minTimeSeconds, new(big.Float).SetInt(liquidateThreshold))
	minRefund := new(big.Float).Mul(new(big.Float).SetInt(trxAmountBigInt), dailyRate)
	minRefund = minRefund.Quo(minRefund, big.NewFloat(86400))
	minRefund = minRefund.Mul(minRefund, minTimeSeconds)
	minRefund = new(big.Float).Add(minRefund, penalty)
	if minRefund.Sign() < 0 {
		minRefund = new(big.Float)
	}
	minRefundSun := new(big.Int)
	minRefund.Int(minRefundSun)

	return prepayCost, energyFeeInt(energyFee), energyFeeInt(securityDeposit), penaltyInt, minRefundSun, energyBillSeconds
}

// energyFeeInt 辅助：将 *big.Float 转为 *big.Int（截断取整）
func energyFeeInt(f *big.Float) *big.Int {
	i := new(big.Int)
	f.Int(i)
	return i
}

// EstimateRentCost 计算租赁费用（自动识别首租/续租）
// 返回 map 说明：
// prepayCost     → *big.Int，你实际要支付的费用 (sun)，用于 msg.value
// minRefund      → *big.Int，最少可退还金额 (sun)，= 最低6h租金 + penalty
// minRefundTrx   → string，最少可退还金额 (TRX, 格式化)
// trxAmount      → string，合约 rentResource 的 amount 参数 (TRX)
// trxAmountSun   → string，合约 rentResource 的 amount 参数 (sun)
// energyPerTrx   → string，完整小数字符串，如 "9.21052404"
// rentalEnergy   → string，用户输入的能量数量
//
// 自动识别规则：
//   - currentEnergy != "" && remainingHours > 0 → 续租模式（delta 计算）
//     续租三个模式：
//       🕐 只加时间: rentalEnergy="0", durationHours>0
//       ⚡ 只加能量: rentalEnergy>"0", durationHours=0
//       🕐+⚡ 同时加: 都 >0
//   - 否则 → 首租模式（全量计算）
func (e *EnergyRental) EstimateRentCost(rentalEnergy string, durationHours int, resourceType ResourceType, currentEnergy string, remainingHours int) (map[string]interface{}, error) {
	// 判断是否是续租
	isRenewal := currentEnergy != "" && remainingHours > 0

	if isRenewal {
		// 续租：新增能量和新增时长至少一个 > 0
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
		if utils.LessThanOrEqual(currentEnergy, "0") {
			return nil, fmt.Errorf("current_energy must be > 0 for renewal")
		}
	} else {
		// 首租：两个参数都必须 > 0
		if utils.LessThanOrEqual(rentalEnergy, "0") {
			return nil, fmt.Errorf("rental_energy must be > 0")
		}
		if durationHours <= 0 {
			return nil, fmt.Errorf("duration_hours must be > 0")
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
	if !isRenewal {
		if utils.LessThan(rentalEnergy, "1") {
			return nil, fmt.Errorf("resource rent: resource amount must be no less than 1TRX")
		}
		// 合约最低要求：amount >= 1 TRX
		if utils.LessThanOrEqual(trxAmount, "0") {
			return nil, fmt.Errorf(
				"rental_energy too small. Current rate ≈ %.8f Energy/TRX. Minimum rental_energy required ≈ %s",
				energyStakePerTrxFloat, energyPerTrxStr)
		}
	}
	trxAmountSun := utils.Mul(trxAmount, "1e6", 0)

	fmt.Printf("[DEBUG] isRenewal=%v trxAmount=%s trxAmountSun=%s rentalEnergy=%s durationHours=%d\n", isRenewal, trxAmount, trxAmountSun, rentalEnergy, durationHours)
	if isRenewal {
		fmt.Printf("[DEBUG] renewal currentEnergy=%s remainingHours=%d\n", currentEnergy, remainingHours)
	}

	// 3. 取两类费率的最大值
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

	// 4. 取清算阈值
	liquidateThresholdBI, err := e.getLiquidateThreshold()
	if err != nil {
		return nil, fmt.Errorf("get liquidate threshold failed: %w", err)
	}

	if isRenewal {
		return e.estimateRenewalCost(rentalEnergy, durationHours, currentEnergy, remainingHours, energyPerTrxStr, trxAmount, trxAmountSun, maxRate, liquidateThresholdBI)
	}

	return e.estimateFirstRentCost(rentalEnergy, durationHours, energyPerTrxStr, trxAmount, trxAmountSun, maxRate, liquidateThresholdBI)
}

// estimateFirstRentCost 首租全量计算
func (e *EnergyRental) estimateFirstRentCost(rentalEnergy string, durationHours int, energyPerTrxStr, trxAmount, trxAmountSun string, maxRate, liquidateThresholdBI *big.Int) (map[string]interface{}, error) {
	trxAmountBigInt := utils.StringToBigInt(trxAmountSun)

	prepayCost, _, _, _, minRefundSun, _ := calcPrepayValues(
		trxAmountBigInt, int64(durationHours)*3600, maxRate, liquidateThresholdBI,
	)

	minRefundTrx := utils.Div(minRefundSun.String(), "1e6", 6)

	fmt.Printf("[DEBUG] 首租 prepayCost=%s energyPerTrxStr=%s trxAmount=%s rentalEnergy=%s\n", prepayCost.String(), energyPerTrxStr, trxAmount, rentalEnergy)

	return map[string]interface{}{
		"prepayCost":    prepayCost,
		"minRefund":     minRefundSun,
		"minRefundTrx":  minRefundTrx,
		"trxAmount":     trxAmount,
		"trxAmountSun":  trxAmountSun,
		"energyPerTrx":  energyPerTrxStr,
		"rentalEnergy":  rentalEnergy,
	}, nil
}

// estimateRenewalCost 续租 delta 计算
// 计算方式：NewDetail(当前能量+新增能量, 剩余时长+新增时长) - OldDetail(当前能量, 剩余时长)
func (e *EnergyRental) estimateRenewalCost(rentalEnergy string, durationHours int, currentEnergy string, remainingHours int, energyPerTrxStr, trxAmount, trxAmountSun string, maxRate, liquidateThresholdBI *big.Int) (map[string]interface{}, error) {
	currentEnergyInt := utils.StringToInt64(currentEnergy)
	addEnergyInt := utils.StringToInt64(rentalEnergy)
	remainingSeconds := int64(remainingHours) * 3600
	addSeconds := int64(durationHours) * 3600

	// 计算旧值：当前能量 × 剩余时长
	oldTrxAmountSun := utils.Mul(utils.Div(currentEnergy, energyPerTrxStr, 6), "1e6", 0)
	oldTrxAmountBigInt := utils.StringToBigInt(oldTrxAmountSun)
	oldPrepay, _, _, _, _, _ := calcPrepayValues(oldTrxAmountBigInt, remainingSeconds, maxRate, liquidateThresholdBI)

	// 计算新值：(当前能量+新增能量) × (剩余时长+新增时长)
	newEnergy := currentEnergyInt + addEnergyInt
	newSeconds := remainingSeconds + addSeconds
	newEnergyStr := utils.Int64ToString(newEnergy, 0)
	newTrxAmountSun := utils.Mul(utils.Div(newEnergyStr, energyPerTrxStr, 6), "1e6", 0)
	newTrxAmountBigInt := utils.StringToBigInt(newTrxAmountSun)
	newPrepay, _, _, _, newMinRefundSun, _ := calcPrepayValues(newTrxAmountBigInt, newSeconds, maxRate, liquidateThresholdBI)

	// delta = 新值 - 旧值
	deltaPrepay := new(big.Int).Sub(newPrepay, oldPrepay)
	if deltaPrepay.Sign() < 0 {
		deltaPrepay = big.NewInt(0)
	}

	minRefundTrx := utils.Div(newMinRefundSun.String(), "1e6", 6)

	fmt.Printf("[DEBUG] 续租 deltaPrepay=%s oldPrepay=%s newPrepay=%s currentEnergy=%s remainingHours=%d addEnergy=%s addHours=%d\n",
		deltaPrepay.String(), oldPrepay.String(), newPrepay.String(), currentEnergy, remainingHours, rentalEnergy, durationHours)

	return map[string]interface{}{
		"prepayCost":    deltaPrepay,
		"minRefund":     newMinRefundSun,
		"minRefundTrx":  minRefundTrx,
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
