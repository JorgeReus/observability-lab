#!/usr/bin/env python
import logging

import boto3
import json
from botocore.exceptions import ClientError
from opentelemetry import trace, baggage
from opentelemetry.trace import NonRecordingSpan, SpanContext, TraceFlags
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import ConsoleSpanExporter, BatchSpanProcessor
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator


trace.set_tracer_provider(TracerProvider())
trace.get_tracer_provider().add_span_processor(BatchSpanProcessor(ConsoleSpanExporter()))

tracer = trace.get_tracer(__name__)
logger = logging.getLogger(__name__)

sqs = boto3.resource('sqs', endpoint_url="http://localhost:4566")

def get_queue(name):
    """
    Gets an SQS queue by name.

    :param name: The name that was used to create the queue.
    :return: A Queue object.
    """
    try:
        queue = sqs.get_queue_by_name(QueueName=name)
        logger.info("Got queue '%s' with URL=%s", name, queue.url)
    except ClientError as error:
        logger.exception("Couldn't get queue named %s.", name)
        raise error
    else:
        return queue

def send_message(queue, message, message_attributes=None):
    """
    Send a message to an Amazon SQS queue.

    :param queue: The queue that receives the message.
    :param message_body: The body text of the message.
    :param message_attributes: Custom attributes of the message. These are key-value
                               pairs that can be whatever you want.
    :return: The response from SQS that contains the assigned message ID.
    """
    if not message_attributes:
        message_attributes = {}

    try:
        msg_body = json.dumps(message)
        response = queue.send_message(
            MessageBody=msg_body,
            MessageAttributes=message_attributes
        )
    except ClientError as error:
        logger.exception("Send message failed: %s", message_body)
        raise error
    else:
        return response


prop = TraceContextTextMapPropagator()
carrier = {}

with tracer.start_as_current_span("parent") as span:
    prop.inject(carrier=carrier)

queue = get_queue("credit-score-requests")
send_message(queue, {
    "userId": "39fdea69-1167-440b-8cc1-6334caeb0066",
    "bankingInstitutionId": "e2d52938-171a-4647-950f-15c316fd3748",
    "bankingCredentials": {"user": "1", "password": "asdasd"},
    "span": "100000",
    "tracingInformation": carrier
})
