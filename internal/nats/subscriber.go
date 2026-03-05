package nats

import (
	"encoding/json"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type Subscriber struct {
	nc *nats.Conn
}

func NewSubscriber(nc *nats.Conn) *Subscriber {
	return &Subscriber{nc: nc}
}

func (s *Subscriber) Subscribe() error {
	subject := "dev.auth.v1.user.registered"
	queue := "network-service"

	_, err := s.nc.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
		var payload interface{}
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			zap.L().Error("failed to unmarshal event payload",
				zap.String("subject", subject),
				zap.Error(err))
			return
		}

		zap.L().Info("Received event",
			zap.String("subject", subject),
			zap.Any("payload", payload))
	})

	if err != nil {
		return err
	}

	zap.L().Info("Subscribed to NATS subject", zap.String("subject", subject))
	return nil
}
