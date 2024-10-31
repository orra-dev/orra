package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/vrischmann/envconfig"
)

const (
	TaskZero           = "task0"
	ResultAggregatorID = "result_aggregator"
	FailureTrackerID   = "failure_tracker"
	WSPing             = "ping"
	WSPong             = "pong"
	MaxServiceDowntime = 30 * time.Minute
)

var (
	LogsRetentionPeriod       = time.Hour * 24
	DependencyPattern         = regexp.MustCompile(`^\$([^.]+)\.`)
	WSWriteTimeOut            = time.Second * 120
	WSMaxMessageBytes   int64 = 10 * 1024 // 10K
)

type Config struct {
	Port       int `envconfig:"default=8005"`
	OpenApiKey string
}

func Load() (Config, error) {
	var cfg Config
	err := envconfig.Init(&cfg)
	if err != nil {
		return Config{}, err
	}
	return cfg, err
}

type Status int

const (
	Registered Status = iota + 1
	Pending
	Processing
	Completed
	Failed
	NotActionable
	Paused
)

func (s *Status) String() string {
	return [...]string{"registered", "pending", "processing", "completed", "failed", "not_actionable", "paused"}[*s-1]
}

func (s *Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *Status) UnmarshalJSON(data []byte) error {
	var val string
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "registered":
		*s = Registered
	case "pending":
		*s = Pending
	case "processing":
		*s = Processing
	case "completed":
		*s = Completed
	case "failed":
		*s = Failed
	case "not_actionable":
		*s = NotActionable
	case "paused":
		*s = Paused
	default:
		return fmt.Errorf("invalid Status: %+v", s)
	}
	return nil
}

type ServiceType int

const (
	Agent ServiceType = iota
	Service
)

func (st *ServiceType) String() string {
	return [...]string{"agent", "service"}[*st]
}

func (st *ServiceType) MarshalJSON() ([]byte, error) {
	return json.Marshal(st.String())
}

func (st *ServiceType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "agent":
		*st = Agent
	case "service":
		*st = Service
	default:
		return fmt.Errorf("invalid ServiceType: %s", s)
	}
	return nil
}
