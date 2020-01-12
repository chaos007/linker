package redis

import (
	"errors"
	"sync"

	"github.com/go-redis/redis"
	"github.com/wpajqz/linker/broker"
)

type redisBroker struct {
	client *redis.Client
	pb     sync.Map
}

func (rb *redisBroker) Publish(topic string, message interface{}) error {
	_, err := rb.client.Publish(topic, message).Result()
	if err != nil {
		return err
	}

	return nil
}

func (rb *redisBroker) Subscribe(nodeID, topic string, process func([]byte)) error {
	var ps *redis.PubSub
	if v, ok := rb.pb.Load(nodeID); ok {
		ps = v.(*redis.PubSub)
		err := ps.Subscribe(topic)
		if err != nil {
			return err
		}
	} else {
		ps = rb.client.Subscribe(topic)
		rb.pb.Store(nodeID, ps)
	}

	ch := ps.Channel()
	go func() {
		for msg := range ch {
			if msg.Channel == topic {
				go process([]byte(msg.Payload))
			}
		}
	}()

	return nil
}

func (rb *redisBroker) UnSubscribe(nodeID, topic string) error {
	if v, ok := rb.pb.Load(nodeID); ok {
		return v.(*redis.PubSub).Unsubscribe(topic)
	}

	return errors.New("node's subscriber is not found")
}

func (rb *redisBroker) UnSubscribeAll(nodeID string) error {
	if v, ok := rb.pb.Load(nodeID); ok {
		return v.(*redis.PubSub).Close()
	}

	return errors.New("node's subscriber is not found")
}

func NewBroker(opts ...Option) broker.Broker {
	options := Options{
		Address: "127.0.0.1:6379",
	}

	for _, o := range opts {
		o(&options)
	}

	rc := redis.NewClient(&redis.Options{
		Addr:     options.Address,
		Password: options.Password,
		DB:       options.DB,
	})

	return &redisBroker{client: rc, pb: sync.Map{}}
}
