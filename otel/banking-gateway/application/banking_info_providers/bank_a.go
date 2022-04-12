package banking_info_providers

import (
	"fmt"
	"math/rand"
	"time"
)

type BankA struct {
	Username      *string
	Password      *string
	StartFromYear int
}

func randInt(lower, upper int) int {
	rand.Seed(time.Now().UnixNano())
	rng := upper - lower
	return rand.Intn(rng) + lower
}

func (n *BankA) Query() (map[string]interface{}, error) {
	// Example Provider
	resp := make(map[string]interface{})
	currentYear, _, _ := time.Now().Date()
	months := []string{
		"January",
		"February",
		"March",
		"April",
		"May",
		"June",
		"July",
		"August",
		"September",
		"October",
		"November",
		"December",
	}
	for year := n.StartFromYear; year < currentYear; year++ {
		monthlyScore := make(map[string]int)
		for _, month := range months {
			monthlyScore[month] = randInt(-1000, 1000)
		}
		resp[fmt.Sprint(year)] = monthlyScore
	}
	return resp, nil
}
