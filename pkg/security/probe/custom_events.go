//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"encoding/json"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/mailru/easyjson"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	// LostEventsRuleID is the rule ID for the lost_events_* events
	LostEventsRuleID = "lost_events"
	// RulesetLoadedRuleID is the rule ID for the ruleset_loaded events
	RulesetLoadedRuleID = "ruleset_loaded"
	// NoisyProcessRuleID is the rule ID for the noisy_process events
	NoisyProcessRuleID = "noisy_process"
	// AbnormalPathRuleID is the rule ID for the abnormal_path events
	AbnormalPathRuleID = "abnormal_path"
	// SelfTestRuleID is the rule ID for the self_test events
	SelfTestRuleID = "self_test"
)

// AllCustomRuleIDs returns the list of custom rule IDs
func AllCustomRuleIDs() []string {
	return []string{
		LostEventsRuleID,
		RulesetLoadedRuleID,
		NoisyProcessRuleID,
		AbnormalPathRuleID,
		SelfTestRuleID,
	}
}

func newCustomEvent(eventType model.EventType, marshaler easyjson.Marshaler) *CustomEvent {
	return &CustomEvent{
		eventType: eventType,
		marshaler: marshaler,
	}
}

// CustomEvent is used to send custom security events to Datadog
type CustomEvent struct {
	eventType model.EventType
	tags      []string
	marshaler easyjson.Marshaler
}

// Clone returns a copy of the current CustomEvent
func (ce *CustomEvent) Clone() CustomEvent {
	return CustomEvent{
		eventType: ce.eventType,
		tags:      ce.tags,
		marshaler: ce.marshaler,
	}
}

// GetTags returns the tags of the custom event
func (ce *CustomEvent) GetTags() []string {
	return append(ce.tags, "type:"+ce.GetType())
}

// GetType returns the type of the custom event as a string
func (ce *CustomEvent) GetType() string {
	return ce.eventType.String()
}

// GetEventType returns the event type
func (ce *CustomEvent) GetEventType() model.EventType {
	return ce.eventType
}

// MarshalJSON is the JSON marshaller function of the custom event
func (ce *CustomEvent) MarshalJSON() ([]byte, error) {
	return easyjson.Marshal(ce.marshaler)
}

// String returns the string representation of a custom event
func (ce *CustomEvent) String() string {
	d, err := json.Marshal(ce)
	if err != nil {
		return err.Error()
	}
	return string(d)
}

func newRule(ruleDef *rules.RuleDefinition) *rules.Rule {
	return &rules.Rule{
		Rule:       &eval.Rule{ID: ruleDef.ID},
		Definition: ruleDef,
	}
}

// EventLostRead is the event used to report lost events detected from user space
// easyjson:json
type EventLostRead struct {
	Timestamp time.Time `json:"date"`
	Name      string    `json:"map"`
	Lost      float64   `json:"lost"`
}

// NewEventLostReadEvent returns the rule and a populated custom event for a lost_events_read event
func NewEventLostReadEvent(mapName string, lost float64) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: LostEventsRuleID,
		}), newCustomEvent(model.CustomLostReadEventType, EventLostRead{
			Name:      mapName,
			Lost:      lost,
			Timestamp: time.Now(),
		})
}

// EventLostWrite is the event used to report lost events detected from kernel space
// easyjson:json
type EventLostWrite struct {
	Timestamp time.Time         `json:"date"`
	Name      string            `json:"map"`
	Lost      map[string]uint64 `json:"per_event"`
}

// NewEventLostWriteEvent returns the rule and a populated custom event for a lost_events_write event
func NewEventLostWriteEvent(mapName string, perEventPerCPU map[string]uint64) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: LostEventsRuleID,
		}), newCustomEvent(model.CustomLostWriteEventType, EventLostWrite{
			Name:      mapName,
			Lost:      perEventPerCPU,
			Timestamp: time.Now(),
		})
}

// RuleLoaded defines a loaded rule
// easyjson:json
type RuleState struct {
	ID         string `json:"id"`
	Version    string `json:"version,omitempty"`
	Expression string `json:"expression"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
}

// PolicyState is used to report policy was loaded
// easyjson:json
type PolicyState struct {
	Name    string       `json:"name"`
	Version string       `json:"version"`
	Source  string       `json:"source"`
	Rules   []*RuleState `json:"rules"`
}

// RulesetLoadedEvent is used to report that a new ruleset was loaded
// easyjson:json
type RulesetLoadedEvent struct {
	Timestamp time.Time      `json:"date"`
	Policies  []*PolicyState `json:"policies"`
}

func PolicyStateFromRuleDefinition(def *rules.RuleDefinition) *PolicyState {
	return &PolicyState{
		Name:    def.Policy.Name,
		Version: def.Policy.Version,
		Source:  def.Policy.Source,
	}
}

func RuleStateFromDefinition(def *rules.RuleDefinition, status string, message string) *RuleState {
	return &RuleState{
		ID:         def.ID,
		Version:    def.Version,
		Expression: def.Expression,
		Status:     status,
		Message:    message,
	}
}

// NewRuleSetLoadedEvent returns the rule and a populated custom event for a new_rules_loaded event
func NewRuleSetLoadedEvent(rs *rules.RuleSet, err *multierror.Error) (*rules.Rule, *CustomEvent) {
	mp := make(map[string]*PolicyState)

	var policyState *PolicyState
	var exists bool

	for _, policy := range rs.GetPolicies() {
		// rule successfully loaded
		for _, ruleDef := range policy.Rules {
			policyName := ruleDef.Policy.Name

			if policyState, exists = mp[policyName]; !exists {
				policyState = PolicyStateFromRuleDefinition(ruleDef)
				mp[policyName] = policyState
			}
			policyState.Rules = append(policyState.Rules, RuleStateFromDefinition(ruleDef, "loaded", ""))
		}
	}

	// rules ignored due to errors
	if err != nil && err.Errors != nil {
		for _, err := range err.Errors {
			if rerr, ok := err.(*rules.ErrRuleLoad); ok {
				policyName := rerr.Definition.Policy.Name

				if _, exists := mp[policyName]; !exists {
					policyState = PolicyStateFromRuleDefinition(rerr.Definition)
					mp[policyName] = policyState
				}
				policyState.Rules = append(policyState.Rules, RuleStateFromDefinition(rerr.Definition, string(rerr.Type()), rerr.Err.Error()))
			}
		}
	}

	var policies []*PolicyState
	for _, policy := range mp {
		policies = append(policies, policy)
	}

	return newRule(&rules.RuleDefinition{
			ID: RulesetLoadedRuleID,
		}), newCustomEvent(model.CustomRulesetLoadedEventType, RulesetLoadedEvent{
			Timestamp: time.Now(),
			Policies:  policies,
		})
}

// NoisyProcessEvent is used to report that a noisy process was temporarily discarded
// easyjson:json
type NoisyProcessEvent struct {
	Timestamp      time.Time     `json:"date"`
	Count          uint64        `json:"pid_count"`
	Threshold      int64         `json:"threshold"`
	ControlPeriod  time.Duration `json:"control_period"`
	DiscardedUntil time.Time     `json:"discarded_until"`
	Pid            uint32        `json:"pid"`
	Comm           string        `json:"comm"`
}

// NewNoisyProcessEvent returns the rule and a populated custom event for a noisy_process event
func NewNoisyProcessEvent(count uint64,
	threshold int64,
	controlPeriod time.Duration,
	discardedUntil time.Time,
	pid uint32,
	comm string,
	timestamp time.Time) (*rules.Rule, *CustomEvent) {

	return newRule(&rules.RuleDefinition{
			ID: NoisyProcessRuleID,
		}), newCustomEvent(model.CustomNoisyProcessEventType, NoisyProcessEvent{
			Timestamp:      timestamp,
			Count:          count,
			Threshold:      threshold,
			ControlPeriod:  controlPeriod,
			DiscardedUntil: discardedUntil,
			Pid:            pid,
			Comm:           comm,
		})
}

func resolutionErrorToEventType(err error) model.EventType {
	switch err.(type) {
	case ErrTruncatedParents, ErrTruncatedParentsERPC:
		return model.CustomTruncatedParentsEventType
	default:
		return model.UnknownEventType
	}
}

// AbnormalPathEvent is used to report that a path resolution failed for a suspicious reason
// easyjson:json
type AbnormalPathEvent struct {
	Timestamp           time.Time        `json:"date"`
	Event               *EventSerializer `json:"triggering_event"`
	PathResolutionError string           `json:"path_resolution_error"`
}

// NewAbnormalPathEvent returns the rule and a populated custom event for a abnormal_path event
func NewAbnormalPathEvent(event *Event, pathResolutionError error) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: AbnormalPathRuleID,
		}), newCustomEvent(resolutionErrorToEventType(event.GetPathResolutionError()), AbnormalPathEvent{
			Timestamp:           event.ResolveEventTimestamp(),
			Event:               NewEventSerializer(event),
			PathResolutionError: pathResolutionError.Error(),
		})
}

// SelfTestEvent is used to report a self test result
// easyjson:json
type SelfTestEvent struct {
	Timestamp time.Time `json:"date"`
	Success   []string  `json:"succeeded_tests"`
	Fails     []string  `json:"failed_tests"`
}

// NewSelfTestEvent returns the rule and the result of the self test
func NewSelfTestEvent(success []string, fails []string) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: SelfTestRuleID,
		}), newCustomEvent(model.CustomSelfTestEventType, SelfTestEvent{
			Timestamp: time.Now(),
			Success:   success,
			Fails:     fails,
		})
}
