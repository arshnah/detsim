package rt

import (
	"os"
	"strconv"
)

// SeedFromEnv reads an int64 seed from the named environment variable, or returns fallback.
func SeedFromEnv(name string, fallback int64) int64 {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	seed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return seed
}

// SchedFromEnv builds a Sched from traceEnv's trace file if set, otherwise from seedEnv's seed.
func SchedFromEnv(seedEnv, traceEnv string, fallbackSeed int64) (*Sched, error) {
	if path := os.Getenv(traceEnv); path != "" {
		trace, err := LoadTrace(path)
		if err != nil {
			return nil, err
		}
		return NewSchedFromTraceLenient(trace), nil
	}
	return NewSched(SeedFromEnv(seedEnv, fallbackSeed)), nil
}
