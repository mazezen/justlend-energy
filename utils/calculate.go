package utils

import (
	"math/big"

	"github.com/shopspring/decimal"
)

func StringToFloat(s string) float64 {
	temp, _ := decimal.NewFromString(s)
	result, _ := temp.Float64()
	return result
}

func FloatToString(a float32, fix int32) string {
	temp := decimal.NewFromFloat32(a)
	return temp.StringFixed(fix)
}

func StringToFloat64(s string) float64 {
	temp, _ := decimal.NewFromString(s)
	result, _ := temp.Float64()
	return result
}

func Float64ToString(a float64, fix int32) string {
	temp := decimal.NewFromFloat(a)
	return temp.StringFixed(fix)
}

func StringToInt(s string) int {
	temp, _ := decimal.NewFromString(s)
	result := temp.IntPart()
	return int(result)
}

func StringToInt64(s string) int64 {
	temp, _ := decimal.NewFromString(s)
	result := temp.BigInt()
	return result.Int64()
}

func Int64ToString(a int64, fix int32) string {
	temp := decimal.NewFromInt(a)
	return temp.StringFixed(fix)
}

func StringToBigInt(s string) *big.Int {
	temp, _ := decimal.NewFromString(s)
	result := temp.BigInt()
	return result
}

func BigIntToString(a *big.Int, fix int32) string {
	temp := decimal.NewFromBigInt(a, fix)
	return temp.StringFixed(fix)
}

func Sub(a, b string, fix int32) string {
	da, _ := decimal.NewFromString(a)
	db, _ := decimal.NewFromString(b)
	return da.Sub(db).Truncate(fix).String()
}

func Add(a, b string, fix int32) string {
	da, _ := decimal.NewFromString(a)
	db, _ := decimal.NewFromString(b)
	return da.Add(db).Truncate(fix).String()
}

func Mul(a, b string, fix int32) string {
	da, _ := decimal.NewFromString(a)
	db, _ := decimal.NewFromString(b)
	return da.Mul(db).Truncate(fix).String()
}

func Div(a, b string, fix int32) string {
	da, _ := decimal.NewFromString(a)
	db, _ := decimal.NewFromString(b)
	return da.Div(db).Truncate(fix).String()
}

func GE(a, b string) bool {
	da, _ := decimal.NewFromString(a)
	db, _ := decimal.NewFromString(b)
	return da.GreaterThanOrEqual(db)
}

func GreaterThan(a, b string) bool {
	da, _ := decimal.NewFromString(a)
	db, _ := decimal.NewFromString(b)
	return da.GreaterThan(db)
}

func LessThan(a, b string) bool {
	da, _ := decimal.NewFromString(a)
	db, _ := decimal.NewFromString(b)
	return da.LessThan(db)
}

func LessThanOrEqual(a, b string) bool {
	da, _ := decimal.NewFromString(a)
	db, _ := decimal.NewFromString(b)
	return da.LessThanOrEqual(db)
}

func Equal(a, b string) bool {
	da, _ := decimal.NewFromString(a)
	db, _ := decimal.NewFromString(b)
	return da.Equal(db)
}

func RoundDown(a string, fix int32) string {
	da, _ := decimal.NewFromString(a)
	temp := da.RoundDown(fix)
	return temp.Truncate(fix).String()
}

func RoundUp(a string, fix int32) string {
	da, _ := decimal.NewFromString(a)
	temp := da.RoundUp(fix)
	return temp.Truncate(fix).String()
}
