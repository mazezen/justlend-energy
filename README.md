## JustLend Energy
> 本项目使用 Golang 在 JustLend 平台上实现了[费率预估] |  [能源租赁] | [退租] | [订单查询]等功能

### JustLend文档中心: https://docs.justlend.org/


### 快速开始

> 你可以下载此项目自行编译: 步骤如下:

```shel
git clone https://github.com/mazezen/justlend-energy.git
cd justlend-energy
go run main.go
```



### 功能

1. **预估费用**  (✅)
2. **租赁**   (✅)
3. **退租**   (✅)
   * **由付款者退租**   (✅)
   * **由能量接收者退租 **  (✅)
4. **订单查询 **  (✅)





### HTTP接口 (8080)

#### 1.  预估费用

	* 描述: 获取当前租赁需要的费用
	* Method: POST
	* Endpoint: /fee

**请求参数**

```json
{
    "rental_energy": 60000000, // 租赁的能量数量 60000000 能量
    "duration_hours": 1, // 租赁时长 1小时
    "resource_type": 1 // 资源 1: 能量 0: 带宽
}
```

**响应结果**

```json
{
    "data": {
        "cost": 40003694,
        "durationHours": 24,
        "energyPerTrx": "9.21056504",
        "note": "cost = Energy Fee + Security Deposit + Liquidation Penalty (min 20 TRX)",
        "rentalEnergy": 60000000,
        "trxAmount": 6666667
    },
    "ok": true
}
```



#### 2. 租赁

 * 描述: 租赁能量
 * Method: POST
 * Endpoint: /rent

**请求参数**

```json
{
    "renter_private_key": "", // 付款者私钥
    "receiver": "", // 接收能量的地址（可与 renter 相同）
    "rental_energy": 60000000, // 租赁的能量数量
    "duration_hours": 1, // 租赁时长（小时）
    "resource_type": 1, // 1: 能量  0: 带宽
    "extra_deposit_sun": 0 // 额外保证金（sun），可传 nil 或 0
}
```

**响应结果**

```json
{
    "data": {
        "txId": ""
    },
    "ok": true
}
```



#### 3. 退租 - 由付款者退租

 * 描述: 退租
 * Method: POST
 * Endpoint: /return/by/renter

**请求参数**

```json
{
    "renter_private_key": "", // 付款者私钥
    "receiver": "", // 接收能量的地址（订单中的 receiver）
    "return_energy": 0, // 部分退还的 TRX 量，传 nil 或 0 表示全部退还
    "resource_type": 1, // 1: 能量  0: 带宽
}
```

**响应结果**

```json
{
    "data": {
        "txId": ""
    },
    "ok": true
}
```



#### 4. 退租 - 由能量接受者退租

 * 描述: 退租
 * Method: POST
 * Endpoint: /return/by/receiver

**请求参数**

```json
{
    "receiver_private_key_hex": "", // 必须是 receiver（接收者）的私钥
    "renter": "", // 订单的付款者（renter）地址
    "return_energy": 0, // 部分退还的 TRX 量，传 nil 或 0 表示全部退还
    "resource_type": 1, // 1: 能量  0: 带宽
}
```

**响应结果**

```json
{
    "data": {
        "txId": ""
    },
    "ok": true
}
```



#### 5. 租赁订单 

 * 描述: 查询租赁订单
 * Method: GET
 * Endpoint: /orderInfo/{renter}/{receiver}/{resourceType:[0-1]+}

**请求参数**

无

**响应结果**

```json
{
    "data": {
        "order": {
            "Amount": 6666667,
            "SecurityDeposit": 40000667,
            "RentIndex": 965452107781204722
        }
    },
    "ok": true
}
```



### 贡献

 **欢迎大家为改进本项目做出贡献！您可以通过 Pull Request 提交代码，或在 Issues 部分留下您的反馈。**
