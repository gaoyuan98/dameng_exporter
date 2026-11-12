package collector

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

// ResultCollector optionally reports business completeness without changing Collect.
type ResultCollector interface {
	CollectWithResult(chan<- prometheus.Metric) CollectorOutcome
}

type OutcomeState string

const (
	OutcomeSuccess OutcomeState = "success"
	OutcomeEmpty   OutcomeState = "empty"
	OutcomePartial OutcomeState = "partial"
	OutcomeFailed  OutcomeState = "failed"
	OutcomeUnknown OutcomeState = "unknown"
)

type CollectorOutcome struct {
	State         OutcomeState
	Reason        string
	DataOrigin    string
	CacheFallback bool
}

func unknownOutcome() CollectorOutcome {
	return CollectorOutcome{State: OutcomeUnknown, Reason: "legacy_unclassified", DataOrigin: "unknown"}
}

func databaseOutcome(state OutcomeState, reason string) CollectorOutcome {
	return CollectorOutcome{State: state, Reason: reason, DataOrigin: "database"}
}

func queryFailure(ctx context.Context, err error) CollectorOutcome {
	reason := "query_error"
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		reason = "query_timeout"
	}
	return databaseOutcome(OutcomeFailed, reason)
}

// Reading errors take precedence over an unverified cache recovery. Only fresh,
// valid rows count toward a usable database result.
func tableOutcome(validRows int, readReason string, cacheFallback bool) CollectorOutcome {
	result := databaseOutcome(OutcomeSuccess, "")
	result.CacheFallback = cacheFallback
	switch {
	case readReason != "":
		result.State, result.Reason = OutcomeFailed, readReason
		if validRows > 0 {
			result.State = OutcomePartial
		}
	case cacheFallback:
		result.State, result.Reason = OutcomeUnknown, "cache_recovery_unverified"
	case validRows == 0:
		result.State = OutcomeEmpty
	}
	return result
}
