package msg_broker

type BankingDataRequest struct {
	UserId               string                 `json:"userId"`
	BankingInstitutionId string                 `json:"bankingInstitutionId"`
	BankingCredentials   map[string]string      `json:"bankingCredentials"`
	Span                 string                 `json:"span"`
	TracingInformation   map[string]interface{} `json:"tracingInformation"`
}

type BankingDataResponse struct {
	Data map[string]interface{} `json:"data"`
}

type Client interface {
	Send(resp *BankingDataResponse) error
	Recv(handlerFunc func(msg *BankingDataRequest) error) error
}
