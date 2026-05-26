## JustLend Energy

> 本项目使用 Golang 在 JustLend 平台上实现了 TRON 链上能量的费率预估、租赁、续租、退租、订单查询等功能。

### JustLend 文档中心

https://docs.justlend.org/

---

## 业务流程

### 核心角色

| 角色                   | 说明                                     |
| ---------------------- | ---------------------------------------- |
| **Renter（付款者）**   | 支付 TRX、发起租赁/续租/退租操作         |
| **Receiver（接收者）** | 接收租赁能量的地址，可与 Renter 相同     |
| **合约**               | JustLend EnergyRental 合约，管理租赁订单 |

### 费用构成

```
总费用 = Energy Fee（能量费） + Security Deposit（保证金） + Penalty（清算罚金）
```

| 费用项               | 说明                           | 计算公式                                                          |
| -------------------- | ------------------------------ | ----------------------------------------------------------------- |
| **Energy Fee**       | 按租赁时长收取的能量使用费     | `trxAmount × dailyRate × (租赁秒数 + liquidateThreshold) / 86400` |
| **Security Deposit** | 保证金（1 天租金），退租时退还 | `trxAmount × dailyRate`                                           |
| **Penalty**          | 清算罚金（最低 20 TRX）        | `trxAmount × 0.008%`，不足 20 TRX 按 20 TRX 计                    |

### 完整租赁生命周期

```
                   ┌─────────────────────────────────────────────────────────────┐
                   │                         开始                                │
                   └────────────────────────┬────────────────────────────────────┘
                                            │
                                            ▼
                   ┌─────────────────────────────────────────────────────────────┐
                   │              ① 预估费用  POST /fee                          │
                   │     ┌─ 首租: 传 rental_energy + duration_hours              │
                   │     └─ 续租: 传 rental_energy + duration_hours              │
                   │               + current_energy + remaining_hours            │
                   └────────────────────────┬────────────────────────────────────┘
                                            │
                                            ▼
                   ┌─────────────────────────────────────────────────────────────┐
                   │              ② 执行租赁/续租                                │
                   │     ┌─ 首租 → POST /rent                                   │
                   │     └─ 续租 → POST /rerent                                  │
                   └────────────────────────┬────────────────────────────────────┘
                                            │
                    ┌───────────────────────┼───────────────────────┐
                    │                       │                       │
                    ▼                       ▼                       ▼
          ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────────┐
          │ ③ 按需退租       │   │ ④ 查询订单       │   │ ⑤ 到期自动退还      │
          │ POST /return    │   │ GET /orderInfo   │   │ (合约自动处理)       │
          │ /by/renter      │   │                  │   │                     │
          │ or              │   │                  │   │                     │
          │ /by/receiver    │   │                  │   │                     │
          └─────────────────┘   └─────────────────┘   └─────────────────────┘
```

### 续租三种模式

当 `current_energy` 和 `remaining_hours` 不为空时，系统自动识别为**续租**，通过 delta 方式计算：

```
续租需支付额 = New(当前能量+新增能量, 剩余时长+新增时长) - Old(当前能量, 剩余时长)
```

| 模式             | rental_energy | duration_hours | 说明                   |  保证金变化   |
| ---------------- | :-----------: | :------------: | ---------------------- | :-----------: |
| 🕐 **只加时间**  |     `"0"`     |      `>0`      | 不新增能量，只延长时间 | **不新增** ✅ |
| ⚡ **只加能量**  |    `>"0"`     |      `0`       | 新增能量，时长不变     |  **新增** 🆕  |
| 🕐+⚡ **同时加** |    `>"0"`     |      `>0`      | 新增能量 + 延长时间    |  **新增** 🆕  |

> **注意**：续租**不产生清算罚金**，因为罚金已在首租时收取。

---

## 接口总览

| 接口                 | Method | Endpoint                                        | 用途                                      | 是否需要私钥 |
| :------------------- | :----: | :---------------------------------------------- | :---------------------------------------- | :----------: |
| **① 预估费用**       |  POST  | `/fee`                                          | 首租/续租前查询预估费用（展示给用户确认） |      ❌      |
| **② 租赁**           |  POST  | `/rent`                                         | 首租：创建新的租赁订单                    |      ✅      |
| **③ 续租**           |  POST  | `/rerent`                                       | 续租：在原有订单基础上增加能量或延长时间  |      ✅      |
| **④ 退租（付款者）** |  POST  | `/return/by/renter`                             | 由付款者发起退租，可取回保证金            |      ✅      |
| **⑤ 退租（接收者）** |  POST  | `/return/by/receiver`                           | 由能量接收者发起退租                      |      ✅      |
| **⑥ 查询订单**       |  GET   | `/orderInfo/{renter}/{receiver}/{resourceType}` | 查询链上租赁订单信息                      |      ❌      |

---

## HTTP 接口 (8080)

### ① 预估费用

> 首租/续租前调用，获取预估费用展示给用户。**不涉及上链，无需私钥。**

- **Method:** `POST`
- **Endpoint:** `/fee`
- **自动识别首租/续租：**
  - 不传 `current_energy` / `remaining_hours` → **首租**（全量计算）
  - 传 `current_energy` + `remaining_hours` → **续租**（delta 差额计算）

**请求参数：**

```json
{
  "rental_energy": "100000", // 首租: 总能量 | 续租: 新增能量 (>=0)
  "duration_hours": 24, // 首租: 总时长 | 续租: 新增时长 (>=0)
  "resource_type": 1, // 1: 能量  0: 带宽
  "current_energy": "600000", // [续租必传] 当前链上能量
  "remaining_hours": 5 // [续租必传] 剩余小时数
}
```

**续租三种模式传参示例：**

<details>
<summary>🕐 只加时间（不新增能量）</summary>

```json
{
  "rental_energy": "0",
  "duration_hours": 3,
  "resource_type": 1,
  "current_energy": "600000",
  "remaining_hours": 2
}
```

</details>

<details>
<summary>⚡ 只加能量（不延长时间）</summary>

```json
{
  "rental_energy": "200000",
  "duration_hours": 0,
  "resource_type": 1,
  "current_energy": "600000",
  "remaining_hours": 2
}
```

</details>

<details>
<summary>🕐+⚡ 同时加能量和时间</summary>

```json
{
  "rental_energy": "200000",
  "duration_hours": 3,
  "resource_type": 1,
  "current_energy": "600000",
  "remaining_hours": 2
}
```

</details>

**响应结果：**

```json
{
  "ok": true,
  "data": {
    "rentalEnergy": "100000",
    "preCost": "129.302399",
    "minRefundTrx": "33.116287",
    "trxAmount": "108184.171287",
    "energyPerTrx": "9.24349642",
    "durationHours": 24,
    "note": "cost = Energy Fee + Security Deposit + Liquidation Penalty (min 20 TRX)"
  }
}
```

| 返回字段        | 类型   | 说明                                             |
| --------------- | ------ | ------------------------------------------------ |
| `rentalEnergy`  | string | 传入的租赁能量数量                               |
| `preCost`       | string | 需支付的预估总费用（TRX），即 `prepayCost / 1e6` |
| `minRefundTrx`  | string | 最少可退还金额（TRX）                            |
| `trxAmount`     | string | 合约 `rentResource` 的 amount 参数（TRX）        |
| `energyPerTrx`  | string | 当前 1 TRX 可换取的能源数量                      |
| `durationHours` | int    | 租赁时长                                         |
| `note`          | string | 费用构成说明                                     |

---

### ② 租赁（首租）

> 首次租赁能量，创建新的链上订单。

- **Method:** `POST`
- **Endpoint:** `/rent`

**请求参数：**

```json
{
  "renter_private_key": "", // 付款者私钥（hex）
  "receiver": "", // 接收能量的地址（可与 renter 相同）
  "rental_energy": "100000", // 租赁的能量数量
  "duration_hours": 24, // 租赁时长（小时）
  "resource_type": 1, // 1: 能量  0: 带宽
  "extra_deposit_sun": 0, // 额外保证金（sun），一般传 0
  "current_energy": "", // 首租不传
  "remaining_hours": 0 // 首租不传
}
```

**响应结果：**

```json
{
  "ok": true,
  "data": {
    "txId": "8c0ca1530a27a917d2effe682508269b3f9300056af29e48ff43e20f6728d614"
  }
}
```

---

### ③ 续租

> 在原有订单基础上增加能量或延长时间。续租使用合约 `rentResource`，与首租相同的合约方法，系统自动计算差额费用。

- **Method:** `POST`
- **Endpoint:** `/rerent`

**请求参数：**

```json
{
  "renter_private_key": "", // 付款者私钥（hex）
  "receiver": "", // 接收能量的地址（与首租保持一致）
  "rental_energy": "200000", // 新增能量（只加时间时传 "0"）
  "duration_hours": 3, // 新增时长（只加能量时传 0）
  "resource_type": 1, // 1: 能量  0: 带宽
  "extra_deposit_sun": 0, // 额外保证金（sun），一般传 0
  "current_energy": "600000", // 当前链上能量（必传）
  "remaining_hours": 5 // 剩余小时数（必传）
}
```

**响应结果：**

```json
{
  "ok": true,
  "data": {
    "txId": "e62f351b089cc2440820b0ce4f638ea787e196c0cf0203be7cfa3604d9479fdd"
  }
}
```

---

### ④ 退租 - 由付款者退租

> 由 Renter（付款者）调用，退还部分或全部能量，取回保证金。

- **Method:** `POST`
- **Endpoint:** `/return/by/renter`

**请求参数：**

```json
{
  "renter_private_key": "", // 付款者私钥（hex）
  "receiver": "", // 接收能量的地址（订单中的 receiver）
  "return_energy": "0", // 退还的 TRX 量，传 "0" 表示全部退还
  "resource_type": 1 // 1: 能量  0: 带宽
}
```

**响应结果：**

```json
{
  "ok": true,
  "data": {
    "txId": ""
  }
}
```

---

### ⑤ 退租 - 由能量接收者退租

> 由 Receiver（能量接收者）调用退租。注意：传入的是 **Receiver 的私钥**，不是 Renter 的。

- **Method:** `POST`
- **Endpoint:** `/return/by/receiver`

**请求参数：**

```json
{
  "receiver_private_key_hex": "", // 能量接收者私钥（hex）
  "renter": "", // 订单的付款者（renter）地址
  "return_energy": "0", // 退还的 TRX 量，传 "0" 表示全部退还
  "resource_type": 1 // 1: 能量  0: 带宽
}
```

**响应结果：**

```json
{
  "ok": true,
  "data": {
    "txId": ""
  }
}
```

---

### ⑥ 查询订单

> 查询链上租赁订单信息。**不上链，无需私钥。**

- **Method:** `GET`
- **Endpoint:** `/orderInfo/{renter}/{receiver}/{resourceType:[0-1]+}`

**路径参数：**

| 参数           | 说明                     |
| -------------- | ------------------------ |
| `renter`       | 付款者地址（Base58）     |
| `receiver`     | 能量接收者地址（Base58） |
| `resourceType` | `1`=能量, `0`=带宽       |

**响应结果：**

```json
{
  "ok": true,
  "data": {
    "order": {
      "Amount": "600000.00",
      "SecurityDeposit": 600000000,
      "RentIndex": 1
    }
  }
}
```

| 返回字段          | 类型      | 说明                                     |
| ----------------- | --------- | ---------------------------------------- |
| `Amount`          | string    | 当前委托的 TRX 数量（已除以 1e6 格式化） |
| `SecurityDeposit` | \*big.Int | 当前保证金（sun）                        |
| `RentIndex`       | \*big.Int | 租金指数，标识续租次数                   |

---

## 快速开始

```shell
git clone https://github.com/mazezen/justlend-energy.git
cd justlend-energy
go run main.go
```

服务启动后监听 `http://0.0.0.0:8080`。

---

## 功能清单

| 功能             | 状态 | 说明                                               |
| :--------------- | :--: | :------------------------------------------------- |
| 预估费用（首租） |  ✅  | 全量计算，含 energyFee + securityDeposit + penalty |
| 预估费用（续租） |  ✅  | delta 差额计算，支持三种模式                       |
| 租赁（首租）     |  ✅  | 创建新订单                                         |
| 续租             |  ✅  | 在原有订单上增加能量或延长时间                     |
| 退租（付款者）   |  ✅  | 由 Renter 退租                                     |
| 退租（接收者）   |  ✅  | 由 Receiver 退租                                   |
| 订单查询         |  ✅  | 查询链上订单信息                                   |

---

## 贡献

**欢迎大家为改进本项目做出贡献！您可以通过 Pull Request 提交代码，或在 Issues 部分留下您的反馈。**
