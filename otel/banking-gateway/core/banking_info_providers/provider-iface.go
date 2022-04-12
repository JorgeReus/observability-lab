package banking_info_providers

type BankingInfoProvider interface {
	Query() (map[string]interface{}, error)
}
