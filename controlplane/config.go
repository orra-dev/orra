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
