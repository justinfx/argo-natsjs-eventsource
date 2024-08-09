# Nats Jetstream to Office 365 Incoming Connector Webhook

A demo of using a Nats Jetstream source message to trigger an Office 365 incoming webhook message (ie MS Teams).

## Setup

```
kubectl apply -f .
```

## Usage

Set some existing Teams incoming webhook:

```
O365_WEBHOOK=https://xxxxx.webhook.office.com/xxxxxxxxx
```

Set up the message we want to send:

```
MSG=$(cat <<-END
{
    "title": "Argo-Events: nats jetstream -> O365", 
    "message": "I was triggered from a nats jetstream message", 
    "webhook": "${O365_WEBHOOK}"
}
END
)
```

Publish a Nats message to trigger the O365 sensor. This assumes the topic is part of an existing Nats Stream.

```
nats pub some.subject.we.want.to.consume.foo "$MSG"  
```
