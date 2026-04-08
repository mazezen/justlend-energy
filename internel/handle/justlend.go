package handle

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/mazezen/justlend-energy/justlend"
	"github.com/mazezen/justlend-energy/tronsdk"
)

type FeeRequest struct {
	RentalEnergy  int64                 `json:"rental_energy"`
	DurationHours int                   `json:"duration_hours"`
	ResourceType  justlend.ResourceType `json:"resource_type"`
}

// Fee 计算租赁费用
// 返回值：
// cost          → 要支付的 msg.value (单位: sun)
// trxAmount     → 合约需要的 amount 参数 (单位: sun)
// energyPerTrx  → 完整小数字符串 (如 "9.21058689")
// rentalEnergy  → 用户输入的能量数量
func Fee(w http.ResponseWriter, r *http.Request) {
	grpcClient, err := tronsdk.NewClient()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}

	jl, err := justlend.NewEnergyRental(grpcClient)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var req FeeRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "parameters are incorrect"})
		return
	}

	cost, trxAmount, energyPerTrx, rentalEnergy, durationHours, err := jl.EstimateRentCost(req.RentalEnergy, req.DurationHours, req.ResourceType)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"rentalEnergy":  rentalEnergy,
			"cost":          cost,
			"trxAmount":     trxAmount,
			"energyPerTrx":  energyPerTrx,
			"durationHours": durationHours,
			"note":          "cost = Energy Fee + Security Deposit + Liquidation Penalty (min 20 TRX)",
		},
	})
	return
}

type RentalRequest struct {
	RenterPrivateKeyHex string `json:"renter_private_key"` // 付款者私钥
	Receiver            string `json:"receiver"`           // 接收能量的地址（可与 renter 相同）
	RentalEnergy        int64  `json:"rental_energy"`      // 租赁的能量数量
	DurationHours       int    `json:"duration_hours"`     // 租赁时长（小时）
	ResourceType        int8   `json:"resource_type"`      // 1: 能量  0: 带宽
	ExtraDepositSun     int64  `json:"extra_deposit_sun"`  // 额外保证金（sun），可传 nil 或 0
}

// Rental 租赁
// 返回值：
// txId          → 租赁HASH
func Rental(w http.ResponseWriter, r *http.Request) {
	grpcClient, err := tronsdk.NewClient()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	jl, err := justlend.NewEnergyRental(grpcClient)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var req RentalRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "parameters are incorrect"})
		return
	}

	txId, err := jl.RentResource(
		req.RenterPrivateKeyHex,
		req.Receiver,
		req.RentalEnergy,
		req.DurationHours,
		justlend.ResourceType(req.ResourceType),
		big.NewInt(req.ExtraDepositSun),
	)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"txId": txId,
		},
	})
}

type ReturnByRenterRequest struct {
	RenterPrivateKeyHex string `json:"renter_private_key"` // 付款者私钥
	Receiver            string `json:"receiver"`           // 接收能量的地址（订单中的 receiver）
	ReturnEnergy        int64  `json:"return_energy"`      // 部分退还的 TRX 量，传 nil 或 0 表示全部退还
	ResourceType        int8   `json:"resource_type"`      // 1: 能量  0: 带宽
}

// ReturnByRenter 退租
// 返回值：
// txId          → 退租HASH
func ReturnByRenter(w http.ResponseWriter, r *http.Request) {
	grpcClient, err := tronsdk.NewClient()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	jl, err := justlend.NewEnergyRental(grpcClient)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var (
		req ReturnByRenterRequest
	)
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "parameters are incorrect"})
		return
	}

	txId, err := jl.ReturnResource(
		req.RenterPrivateKeyHex,
		req.Receiver,
		big.NewInt(req.ReturnEnergy),
		justlend.ResourceType(req.ResourceType),
	)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"txId": txId,
		},
	})
}

type ReturnByReceiverRequest struct {
	ReceiverPrivateKeyHex string `json:"receiver_private_key_hex"` // 必须是 receiver（接收者）的私钥
	Renter                string `json:"renter"`                   // 订单的付款者（renter）地址
	ReturnEnergy          int64  `json:"return_energy"`            // 部分退还的 TRX 量，传 nil 或 0 表示全部退还
	ResourceType          int8   `json:"resource_type"`            // 1: 能量  0: 带宽
}

// ReturnByReceiver 退租
// 返回值：
// txId          → 退租HASH
func ReturnByReceiver(w http.ResponseWriter, r *http.Request) {
	grpcClient, err := tronsdk.NewClient()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	jl, err := justlend.NewEnergyRental(grpcClient)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var req ReturnByReceiverRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "parameters are incorrect"})
		return
	}

	var txId string
	txId, err = jl.ReturnResourceByReceiver(
		req.ReceiverPrivateKeyHex,
		req.Renter,
		big.NewInt(req.ReturnEnergy),
		justlend.ResourceType(req.ResourceType),
	)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"txId": txId,
		},
	})
}

// GetOrderInfo 租赁订单信息
// url: /orderInfo/{renter}/{receiver}/{resourceType}
func GetOrderInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	renter := vars["renter"]             // 订单的付款者（renter）地址
	receiver := vars["receiver"]         // 能量接受者（receiver）地址
	resourceType := vars["resourceType"] // 1: 能量  0: 带宽
	at, err := strconv.Atoi(resourceType)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	rt := justlend.ResourceType(at)

	fmt.Printf("[DEBUG] renter: %s\n", renter)
	fmt.Printf("[DEBUG] receiver: %s\n", receiver)
	fmt.Printf("[DEBUG] resourceType: %d\n", at)

	grpcClient, err := tronsdk.NewClient()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	jl, err := justlend.NewEnergyRental(grpcClient)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	rentalInfo, err := jl.GetRentInfo(renter, receiver, rt)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "data": map[string]interface{}{"order": rentalInfo}})
}
