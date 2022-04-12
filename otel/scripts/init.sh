#!/bin/env bash

set -e

aws --endpoint-url=http://localstack:4566 sqs create-queue --queue-name banking-requests
aws --endpoint-url=http://localstack:4566 sqs create-queue --queue-name banking-responses

aws --endpoint-url=http://localstack:4566 sqs create-queue --queue-name credit-score-requests
aws --endpoint-url=http://localstack:4566 sqs create-queue --queue-name credit-score-responses
