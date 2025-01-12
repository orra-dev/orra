/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

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
	TaskZero                      = "task0"
	ResultAggregatorID            = "result_aggregator"
	FailureTrackerID              = "failure_tracker"
	WSPing                        = "ping"
	WSPong                        = "pong"
	HealthCheckGracePeriod        = 30 * time.Minute
	TaskTimeout                   = 30 * time.Second
	CompensationDataStoredLogType = "compensation_stored"
	CompensationAttemptedLogType  = "compensation_attempted"
	CompensationCompleteLogType   = "compensation_complete"
	CompensationPartialLogType    = "compensation_partial"
	CompensationFailureLogType    = "compensation_failure"
	CompensationExpiredLogType    = "compensation_expired"
	VersionHeader                 = "X-Orra-CP-Version"
	PauseExecutionCode            = "PAUSE_EXECUTION"
)

var (
	Version                   = "0.2.0"
	LogsRetentionPeriod       = time.Hour * 24
	DependencyPattern         = regexp.MustCompile(`^\$([^.]+)\.`)
	WSWriteTimeOut            = time.Second * 120
	WSMaxMessageBytes   int64 = 10 * 1024 // 10K
)

type Config struct {
	Port         int `envconfig:"default=8005"`
	OpenaiApiKey string
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
	Registered Status = iota
	Pending
	Processing
	Completed
	Failed
	NotActionable
	Paused
)

func (s Status) String() string {
	switch s {
	case Registered:
		return "registered"
	case Pending:
		return "pending"
	case Processing:
		return "processing"
	case Completed:
		return "completed"
	case Failed:
		return "failed"
	case NotActionable:
		return "not_actionable"
	case Paused:
		return "paused"
	default:
		return ""
	}
}

func (s Status) MarshalJSON() ([]byte, error) {
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
		return fmt.Errorf("invalid Status: %s", s)
	}
	return nil
}

type ServiceType int

const (
	Agent ServiceType = iota
	Service
)

func (st ServiceType) String() string {
	switch st {
	case Agent:
		return "agent"
	case Service:
		return "service"
	}
	return ""
}

func (st ServiceType) MarshalJSON() ([]byte, error) {
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

type CompensationStatus int

const (
	CompensationPending CompensationStatus = iota
	CompensationProcessing
	CompensationCompleted
	CompensationFailed
	CompensationPartial
	CompensationExpired
)

func (s CompensationStatus) String() string {
	switch s {
	case CompensationPending:
		return "pending"
	case CompensationProcessing:
		return "processing"
	case CompensationCompleted:
		return "completed"
	case CompensationFailed:
		return "failed"
	case CompensationPartial:
		return "partial"
	case CompensationExpired:
		return "expired"
	default:
		return "unknown"
	}
}

func (s CompensationStatus) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

func (s *CompensationStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	switch str {
	case "pending":
		*s = CompensationPending
	case "processing":
		*s = CompensationProcessing
	case "completed":
		*s = CompensationCompleted
	case "failed":
		*s = CompensationFailed
	case "partial":
		*s = CompensationPartial
	case "expired":
		*s = CompensationExpired
	default:
		return fmt.Errorf("invalid Compensation Status: %s", s)
	}
	return nil
}
