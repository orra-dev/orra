package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	"time"

	"github.com/olahol/melody"
	"github.com/rs/zerolog"
)

func NewWebSocketManager(logger zerolog.Logger) *WebSocketManager {
	m := melody.New()
	m.Config.ConcurrentMessageHandling = true
	m.Config.WriteWait = WSWriteTimeOut
	m.Config.MaxMessageSize = WSMaxMessageBytes

	return &WebSocketManager{
		melody:            m,
		logger:            logger,
		connMap:           make(map[string]*melody.Session),
		messageQueues:     make(map[string]*WebSocketMessageQueue),
		messageExpiration: time.Hour * 24, // Keep messages for 24 hours
		pingInterval:      m.Config.PingPeriod,
		pongWait:          m.Config.PongWait,
		serviceHealth:     make(map[string]bool),
	}
}

func (wsm *WebSocketManager) HandleConnection(serviceID string, serviceName string, s *melody.Session) {
	s.Set("serviceID", serviceID)
	s.Set("lastPong", time.Now().UTC())

	wsm.connMu.Lock()
	wsm.connMap[serviceID] = s
	wsm.connMu.Unlock()

	wsm.UpdateServiceHealth(serviceID, true)
	go wsm.pingRoutine(serviceID)

	wsm.logger.Info().
		Str("serviceID", serviceID).
		Str("serviceName", serviceName).
		Msg("New WebSocket connection established")
}

func (wsm *WebSocketManager) HandleDisconnection(serviceID string) {
	wsm.connMu.Lock()
	delete(wsm.connMap, serviceID)
	wsm.connMu.Unlock()

	wsm.UpdateServiceHealth(serviceID, false)
	wsm.logger.Info().Str("ServiceID", serviceID).Msg("WebSocket connection closed")
}

func (wsm *WebSocketManager) HandleMessage(s *melody.Session, msg []byte, fn ServiceFinder) {
	var messageWrapper struct {
		ID      string          `json:"id"`
		Payload json.RawMessage `json:"payload"`
	}

	var messagePayload TaskResult

	if err := json.Unmarshal(msg, &messageWrapper); err != nil {
		wsm.logger.Error().Err(err).Msg("Failed to unmarshal wrapped WebSocket messageWrapper")
		return
	}

	if err := json.Unmarshal(messageWrapper.Payload, &messagePayload); err != nil {
		wsm.logger.Error().Err(err).Msg("Failed to unmarshal WebSocket messageWrapper payload")
		return
	}

	switch messagePayload.Type {
	case WSPong:
		s.Set("lastPong", time.Now().UTC())
		wsm.UpdateServiceHealth(messagePayload.ServiceID, true)
	case "task_status":
		wsm.UpdateServiceHealth(messagePayload.ServiceID, true)
		wsm.logger.
			Info().
			Str("IdempotencyKey", string(messagePayload.IdempotencyKey)).
			Str("ServiceID", messagePayload.ServiceID).
			Str("TaskID", messagePayload.TaskID).
			Str("ExecutionID", messagePayload.ExecutionID).
			Msgf("Task status: %s", messagePayload.Status)
	case "task_result":
		wsm.UpdateServiceHealth(messagePayload.ServiceID, true)
		wsm.handleTaskResult(messagePayload, fn)
	default:
		wsm.logger.Warn().Str("type", messagePayload.Type).Msg("Received unknown messageWrapper type")
	}

	if err := wsm.acknowledgeMessageReceived(s, messageWrapper.ID); err != nil {
		wsm.logger.Error().Err(err).Msg("Failed to handle messageWrapper acknowledgement")
		return
	}
}

func (wsm *WebSocketManager) acknowledgeMessageReceived(s *melody.Session, id string) error {
	if isPong := id == WSPong; isPong {
		return nil
	}

	ack := struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{
		Type: "ACK",
		ID:   id,
	}

	acknowledgement, err := json.Marshal(ack)
	if err != nil {
		return fmt.Errorf("failed to marshal acknowledgement: %w", err)
	}

	if err := s.Write(acknowledgement); err != nil {
		wsm.logger.Error().Err(err).Msg("Failed to send ACK")
		return fmt.Errorf("failed to send acknowledgement of receipt: %w", err)
	}

	return nil
}

func (wsm *WebSocketManager) handleTaskResult(message TaskResult, fn ServiceFinder) {
	service, err := fn(message.ServiceID)
	if err != nil {
		wsm.logger.Error().
			Err(err).
			Str("serviceID", message.ServiceID).
			Msg("Failed to get service when handling task result")
		return
	}

	service.IdempotencyStore.UpdateExecutionResult(
		message.IdempotencyKey,
		message.Result,
		parseError(message.Error),
	)
}

func parseError(errStr string) error {
	if errStr == "" {
		return nil
	}
	return fmt.Errorf(errStr)
}

func (wsm *WebSocketManager) SendTask(serviceID string, task *Task) error {
	wsm.connMu.RLock()
	session, connected := wsm.connMap[serviceID]
	wsm.connMu.RUnlock()

	message, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to convert message to JSON for service %s: %w", serviceID, err)
	}

	if !connected {
		return nil
	}

	return session.Write(message)
}

func (wsm *WebSocketManager) pingRoutine(serviceID string) {
	ticker := time.NewTicker(wsm.pingInterval)
	defer ticker.Stop()

	for {
		<-ticker.C

		wsm.connMu.RLock()
		session, exists := wsm.connMap[serviceID]
		wsm.connMu.RUnlock()

		if !exists {
			wsm.logger.Warn().
				Str("ServiceID", serviceID).
				Msg("Service connection has already been closed")
			return
		}

		if err := session.Write([]byte(WSPing)); err != nil {
			wsm.logger.Warn().
				Str("ServiceID", serviceID).
				Err(err).
				Msg("Failed to send ping, closing connection")

			wsm.UpdateServiceHealth(serviceID, false)

			if err := session.Close(); err != nil {
				return
			}
			return
		}

		// Check for pong response
		lastPong, ok := session.Get("lastPong")
		if !ok || time.Since(lastPong.(time.Time)) > wsm.pongWait {
			wsm.logger.Warn().
				Str("ServiceID", serviceID).
				Msg("Pong timeout, closing connection")

			wsm.UpdateServiceHealth(serviceID, false)

			if err := session.Close(); err != nil {
				return
			}

			return
		}
		wsm.UpdateServiceHealth(serviceID, true)
	}
}

func (wsm *WebSocketManager) UpdateServiceHealth(serviceID string, isHealthy bool) {
	wsm.healthMu.Lock()
	wsm.serviceHealth[serviceID] = isHealthy
	wsm.healthMu.Unlock()
}

// CleanupExpiredMessages cleans up expired messages
func (wsm *WebSocketManager) CleanupExpiredMessages() {
	wsm.messageQueuesMu.RLock()
	defer wsm.messageQueuesMu.RUnlock()

	for serviceID, queue := range wsm.messageQueues {
		queue.mu.Lock()
		var next *list.Element
		for e := queue.Front(); e != nil; e = next {
			next = e.Next()
			msg := e.Value.(*WebSocketQueuedMessage)
			if time.Since(msg.Time) > wsm.messageExpiration {
				queue.Remove(e)
			}
		}
		queue.mu.Unlock()

		wsm.logger.Debug().Str("serviceID", serviceID).Int("queueLength", queue.Len()).Msg("Cleaned up expired messages")
	}
}

func (wsm *WebSocketManager) IsServiceHealthy(serviceID string) bool {
	wsm.healthMu.RLock()
	defer wsm.healthMu.RUnlock()
	healthy, exists := wsm.serviceHealth[serviceID]
	return exists && healthy
}
