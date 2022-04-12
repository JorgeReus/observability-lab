package usecases

import (
	banking_gateway "credit-score-service/core/baking_gateway"
	"credit-score-service/core/credit_score"
	"fmt"
	"log"
)

func CalculateScoreHandler(creditScoreClient credit_score.Client, bankingGatewayClient banking_gateway.Client) error {
	go creditScoreClient.Recv(func(msg *credit_score.CreditScoreRequest) error {
		log.Println(fmt.Sprintf("Calculate Score Request Recieved: %v", msg.UserId))
		err := bankingGatewayClient.Send(&banking_gateway.BankingGatewayRequest{
			UserId:               msg.UserId,
			BankingInstitutionId: msg.BankingInstitutionId,
			BankingCredentials:   msg.BankingCredentials,
			Span:                 msg.Span,
			TracingInformation:   msg.TracingInformation,
		})
		return err
	})

	return nil
}
