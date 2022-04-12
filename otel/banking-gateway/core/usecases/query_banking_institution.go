package usecases

import (
	bank_impl "banking-gateway/application/banking_info_providers"
	"banking-gateway/core/banking_info_providers"
	"banking-gateway/core/msg_broker"
	msg_broker_iface "banking-gateway/core/msg_broker"
	"errors"
	"fmt"
)

func BankingInstitutionReqConsumer(client msg_broker.Client) error {
	err := client.Recv(func(msg *msg_broker.BankingDataRequest) error {
		var provider banking_info_providers.BankingInfoProvider
		// Banking Mock Call
		userName := msg.BankingCredentials["username"]
		password := msg.BankingCredentials["password"]

		provider = &bank_impl.BankA{
			Username:      &userName,
			Password:      &password,
			StartFromYear: 2015,
		}
		resp, err := provider.Query()
		if err != nil {
			return errors.New(fmt.Sprintf("Error querying bank %s :%v", msg.BankingInstitutionId, err))
		}

		err = client.Send(&msg_broker_iface.BankingDataResponse{
			Data: map[string]interface{}{
				"userId":               msg.UserId,
				"bankingInstitutionId": msg.BankingInstitutionId,
				"tracingInformation":   msg.TracingInformation,
				"scores":               resp,
			},
		})
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
