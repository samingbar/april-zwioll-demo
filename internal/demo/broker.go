package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type BrokerMessage struct {
	ID          string             `json:"id"`
	Topic       string             `json:"topic"`
	Event       MediaDeliveryEvent `json:"event"`
	RawMessage  []byte             `json:"rawMessage"`
	PublishedAt time.Time          `json:"publishedAt"`
}

type Broker struct {
	mutex     sync.Mutex
	nextID    int
	consumers []chan BrokerMessage
}

func NewBroker() *Broker {
	return &Broker{
		consumers: make([]chan BrokerMessage, 0),
	}
}

func (broker *Broker) Publish(event MediaDeliveryEvent) (BrokerMessage, error) {
	event.ApplyDefaults()
	raw, err := json.Marshal(event)
	if err != nil {
		return BrokerMessage{}, fmt.Errorf("marshal broker event: %w", err)
	}

	broker.mutex.Lock()
	defer broker.mutex.Unlock()

	broker.nextID++
	message := BrokerMessage{
		ID:          fmt.Sprintf("msg_%06d", broker.nextID),
		Topic:       event.Topic,
		Event:       event,
		RawMessage:  raw,
		PublishedAt: time.Now().UTC(),
	}

	for _, consumer := range broker.consumers {
		consumer <- message
	}

	return message, nil
}

func (broker *Broker) Subscribe(ctx context.Context) <-chan BrokerMessage {
	consumer := make(chan BrokerMessage, 128)

	broker.mutex.Lock()
	broker.consumers = append(broker.consumers, consumer)
	broker.mutex.Unlock()

	go func() {
		<-ctx.Done()
		broker.mutex.Lock()
		defer broker.mutex.Unlock()
		for index, candidate := range broker.consumers {
			if candidate == consumer {
				broker.consumers = append(broker.consumers[:index], broker.consumers[index+1:]...)
				close(consumer)
				return
			}
		}
	}()

	return consumer
}
