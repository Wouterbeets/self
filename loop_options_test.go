package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoopOptionParsing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     map[string]string
		args    []string
		want    loopOptions
		problem string
	}{
		{name: "defaults", args: []string{"--", "mind", "--timeout", "untouched"}, want: loopOptions{12, 2, 30 * time.Minute, "", []string{"mind", "--timeout", "untouched"}}},
		{name: "environment", env: map[string]string{"MAX_PASSES": "4", "SETTLE": "3", "TIMEOUT": "8s", "ASK": "from env", "MIND": "mind --flag"}, want: loopOptions{4, 3, 8 * time.Second, "from env", []string{"sh", "-c", "mind --flag"}}},
		{name: "overrides invalid defaults", env: map[string]string{"MAX_PASSES": "bad", "SETTLE": "0", "TIMEOUT": "no", "ASK": "old", "MIND": "old mind"}, args: []string{"--max-passes=5", "--settle=1", "--timeout=2s", "--ask=", "--", "mind"}, want: loopOptions{5, 1, 2 * time.Second, "", []string{"mind"}}},
		{name: "separator as ask", args: []string{"--ask", "--", "--", "mind"}, want: loopOptions{12, 2, 30 * time.Minute, "--", []string{"mind"}}},
		{name: "standard positional mind", args: []string{"mind", "--settle", "9"}, want: loopOptions{12, 2, 30 * time.Minute, "", []string{"mind", "--settle", "9"}}},
		{name: "last override wins", args: []string{"--settle=bad", "--settle=3", "--", "mind"}, want: loopOptions{12, 3, 30 * time.Minute, "", []string{"mind"}}},
		{name: "unknown", args: []string{"--typo=2"}, problem: "flag provided but not defined"},
		{name: "missing value", args: []string{"--timeout"}, problem: "flag needs an argument"},
		{name: "missing mind", problem: "no mind configured"},
		{name: "zero passes", args: []string{"--max-passes=0", "--", "mind"}, problem: "positive integer"},
		{name: "negative settle", args: []string{"--settle=-1", "--", "mind"}, problem: "positive integer"},
		{name: "zero duration", args: []string{"--timeout=0s", "--", "mind"}, problem: "positive Go duration"},
		{name: "bad environment", env: map[string]string{"TIMEOUT": "bad", "MIND": "mind"}, problem: "positive Go duration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"MAX_PASSES", "SETTLE", "TIMEOUT", "ASK", "MIND"} {
				t.Setenv("SELF_LOOP_"+key, tc.env[key])
			}
			got, err := parseLoopOptions(tc.args)
			if tc.problem != "" {
				if err == nil || !strings.Contains(err.Error(), tc.problem) {
					t.Fatalf("error=%v, want %q", err, tc.problem)
				}
			} else if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, error=%v; want %+v", got, err, tc.want)
			}
		})
	}
}
