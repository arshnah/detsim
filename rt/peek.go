package rt

import (
	"os"
	"strconv"
)

// DecisionLimitFromEnv reads a decision limit from envVar. 0 means no limit.
func DecisionLimitFromEnv(envVar string) int {
	v := os.Getenv(envVar)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
