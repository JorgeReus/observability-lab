package credit_score

type CreditScoreRequest struct {
	UserId               string                 `json:"userId"`
	BankingInstitutionId string                 `json:"bankingInstitutionId"`
	BankingCredentials   map[string]string      `json:"bankingCredentials"`
	Span                 string                 `json:"span"`
	TracingInformation   map[string]interface{} `json:"tracingInformation"`
}

type CreditScoreResponse struct {
	Score int `json:"score"`
}

type Client interface {
	Recv(handlerFunc func(msg *CreditScoreRequest) error) error
	Send(resp *CreditScoreResponse) error
}
