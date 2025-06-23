package kafka

import (
	"context"
	"log"
	"net"
	"strconv"

	"github.com/segmentio/kafka-go"
)

var (
	kafkaBroker = "kafka:9093"
)

// EnsureTopicExists проверяет и создает топик, если его нет.
// Теперь это переиспользуемая функция.
func EnsureTopicExists(ctx context.Context, brokerAddress, topicName string) error {
	log.Printf("Проверка и создание топика '%s' на брокере %s...", topicName, brokerAddress)

	// Используем Dial, а не DialLeader, так как нам нужен любой брокер для получения метаданных
	conn, err := kafka.Dial("tcp", brokerAddress)
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	var controllerConn *kafka.Conn
	controllerConn, err = kafka.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	topicConfigs := []kafka.TopicConfig{
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
