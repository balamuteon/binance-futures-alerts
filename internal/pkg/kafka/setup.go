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


func EnsureTopicExists(ctx context.Context, kafkaBroker, topicName string) error {
	log.Printf("Проверка и создание топика '%s' на брокере %s...", topicName, kafkaBroker)

	// Используем Dial, а не DialLeader, так как нам нужен любой брокер для получения метаданных
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
			NumPartitions:     1,
			ReplicationFactor: 1, // Для локального кластера из одного брокера
		},
	}

	err = controllerConn.CreateTopics(topicConfigs...)
	// Ошибка "Topic with this name already exists" (код 36) является нормальной, игнорируем ее.
	// Для разных версий Kafka текст ошибки может отличаться, поэтому лучше проверять код ошибки.
	if err != nil {
		// Ошибка "Topic with this name already exists" является нормальной, игнорируем ее.
		if err.Error() == "[36] Topic with this name already exists" {
			log.Printf("[Ingestor] Топик '%s' уже существует.", topicName)
			return nil
		}
		return err
	}

	log.Printf("Топик '%s' успешно создан.", topicName)
	return nil
}
