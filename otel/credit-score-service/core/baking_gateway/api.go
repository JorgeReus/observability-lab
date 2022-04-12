package banking_gateway

type BankingGatewayRequest struct {
	UserId               string                 `json:"userId"`
	BankingInstitutionId string                 `json:"bankingInstitutionId"`
	BankingCredentials   map[string]string      `json:"bankingCredentials"`
	Span                 string                 `json:"span"`
	TracingInformation   map[string]interface{} `json:"tracingInformation"`
}

type BankingGatesWayResponse struct {
	Data map[string]interface{} `json:"data"`
}

type Client interface {
	Send(resp *BankingGatewayRequest) error
	// Recv(handlerFunc func(msg *BankingGatesWayResponse) error) error
	Recv() (float64, error)
}
