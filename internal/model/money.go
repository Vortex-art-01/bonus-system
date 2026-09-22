package model

import (
	"fmt"
	"math/big"
)

type Money int64

const kopecksPerPoint = 100

func ParseMoney(s string) (Money, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, fmt.Errorf("parse money %q: not a decimal number", s)
	}
	r.Mul(r, big.NewRat(kopecksPerPoint, 1))

	kopecks, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if rem.Abs(rem).Lsh(rem, 1).Cmp(r.Denom()) >= 0 {
		kopecks.Add(kopecks, big.NewInt(int64(r.Sign())))
	}
	if !kopecks.IsInt64() {
		return 0, fmt.Errorf("parse money %q: out of range", s)
	}
	return Money(kopecks.Int64()), nil
}

func (m Money) String() string {
	abs := uint64(m)
	if m < 0 {
		abs = -abs
	}
	sign := ""
	if m < 0 {
		sign = "-"
	}
	points, kopecks := abs/kopecksPerPoint, abs%kopecksPerPoint
	switch {
	case kopecks == 0:
		return fmt.Sprintf("%s%d", sign, points)
	case kopecks%10 == 0:
		return fmt.Sprintf("%s%d.%d", sign, points, kopecks/10)
	default:
		return fmt.Sprintf("%s%d.%02d", sign, points, kopecks)
	}
}

func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(m.String()), nil
}

func (m *Money) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	parsed, err := ParseMoney(string(data))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}
