// internal/services/queue_service.go
// RabbitMQ 隊列服務

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"mail-proxy/internal/config"
	"mail-proxy/internal/models"
)

// QueueService RabbitMQ 隊列服務
type QueueService struct {
	cfg     *config.Config
	conn    *amqp.Connection
	channel *amqp.Channel
	mu      sync.RWMutex
}

// NewQueueService 建立隊列服務
func NewQueueService(cfg *config.Config) (*QueueService, error) {
	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	svc := &QueueService{
		cfg:     cfg,
		conn:    conn,
		channel: channel,
	}

	// 宣告隊列
	if err := svc.declareQueues(); err != nil {
		channel.Close()
		conn.Close()
		return nil, err
	}

	return svc, nil
}

// declareQueues 宣告所有隊列
func (s *QueueService) declareQueues() error {
	// 宣告死信交換器
	if err := s.channel.ExchangeDeclare(
		"dlx",    // name
		"direct", // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	); err != nil {
		return fmt.Errorf("failed to declare DLX: %w", err)
	}

	// 宣告主郵件隊列
	_, err := s.channel.QueueDeclare(
		s.cfg.MailQueueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-dead-letter-exchange": "dlx",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to declare mail queue: %w", err)
	}

	// 宣告重試隊列
	_, err = s.channel.QueueDeclare(
		s.cfg.RetryQueueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare retry queue: %w", err)
	}

	// 宣告失敗隊列
	_, err = s.channel.QueueDeclare(
		s.cfg.FailedQueueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare failed queue: %w", err)
	}

	// 綁定失敗隊列到 DLX
	if err := s.channel.QueueBind(
		s.cfg.FailedQueueName,
		"failed",
		"dlx",
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to bind failed queue: %w", err)
	}

	log.Println("RabbitMQ queues declared successfully")
	return nil
}

// PublishMail 發布郵件到隊列
func (s *QueueService) PublishMail(job *models.MailJob) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return s.channel.PublishWithContext(
		context.Background(),
		"",                  // exchange
		s.cfg.MailQueueName, // routing key
		false,               // mandatory
		false,               // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
		},
	)
}

// PublishRetry 發布到重試隊列
func (s *QueueService) PublishRetry(job *models.MailJob, delayMs int) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return s.channel.PublishWithContext(
		context.Background(),
		"",
		s.cfg.RetryQueueName,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
			Headers: amqp.Table{
				"x-retry-count": job.RetryCount,
				"x-delay":       delayMs,
			},
		},
	)
}

// PublishFailed 發布到失敗隊列
func (s *QueueService) PublishFailed(job *models.MailJob) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return s.channel.PublishWithContext(
		context.Background(),
		"",
		s.cfg.FailedQueueName,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
		},
	)
}

// Close 關閉連接
func (s *QueueService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.channel != nil {
		s.channel.Close()
	}
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
