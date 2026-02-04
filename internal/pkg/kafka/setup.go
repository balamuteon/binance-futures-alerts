package kafka

import (
	"context"
	"log"
	"net"
	"strconv"
	"time"

	kafkaGO "github.com/segmentio/kafka-go"
)

type ReaderOption func(*kafkaGO.ReaderConfig)

type KafkaReader struct {
	Reader *kafkaGO.Reader
}

func NewWriter(topic, kafkaBroker string) *kafkaGO.Writer {
	return &kafkaGO.Writer{
		Addr:     kafkaGO.TCP(kafkaBroker),
		Topic:    topic,
		Balancer: &kafkaGO.LeastBytes{},
	}
}

func NewReader(kafkaBroker []string, groupId, topic string, opts ...ReaderOption) KafkaReader {
	config := kafkaGO.ReaderConfig{
		Brokers:        kafkaBroker,
		GroupID:        groupId,
		Topic:          topic,
		MinBytes:       1,
		MaxBytes:       1e6,
		MaxWait:        10 * time.Second,
		CommitInterval: time.Second,
	}

	for _, opt := range opts {
		opt(&config)
	}

	return KafkaReader{Reader: kafkaGO.NewReader(config)}
}

func WithMinBytes(minBytes int) ReaderOption {
	return func(c *kafkaGO.ReaderConfig) {
		c.MinBytes = minBytes
	}
}

func WithMaxBytes(maxBytes int) ReaderOption {
	return func(c *kafkaGO.ReaderConfig) {
		c.MaxBytes = maxBytes
	}
}

func EnsureTopicExists(ctx context.Context, kafkaBroker, topicName string, partitions int) error {
	log.Printf("Проверка и создание топика '%s' на брокере %s...", topicName, kafkaBroker)
	if partitions <= 0 {
		partitions = 1
	}

	conn, err := kafkaGO.Dial("tcp", kafkaBroker)
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	var controllerConn *kafkaGO.Conn
	controllerConn, err = kafkaGO.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	topicConfigs := []kafkaGO.TopicConfig{
		{
			Topic:             topicName,
			NumPartitions:     partitions,
			ReplicationFactor: 1,
		},
	}

	err = controllerConn.CreateTopics(topicConfigs...)
	if err != nil {
		if err.Error() == "[36] Topic with this name already exists" {
			log.Printf("[Ingestor] Топик '%s' уже существует.", topicName)
			return nil
		}
		return err
	}

	log.Printf("Топик '%s' успешно создан.", topicName)
	return nil
}
