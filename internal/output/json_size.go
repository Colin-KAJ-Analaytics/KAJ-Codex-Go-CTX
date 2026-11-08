package output

import (
	"encoding/json"
	"strconv"

	"goctx/internal/tokens"
)

// jsonSizer accounts for the same encoding as marshalEnvelope without encoding
// the entire selected response for every candidate. Items are encoded once;
// array separators, omission counts, and the size field are accounted for here.
type jsonSizer struct {
	base                  int
	compact               bool
	priorCost, priorCount int
	omissionCost          map[string]int
}

func newJSONSizer(env Envelope, prior []Omission) (jsonSizer, error) {
	env.Files, env.Symbols, env.Routes, env.Tests, env.ReadPlan = nil, nil, nil, nil, nil
	env.Omitted = nil
	env.ActualEstimatedTokens = 0
	data, err := marshalEnvelope(env)
	s := jsonSizer{base: len(data), compact: env.CompactJSON, omissionCost: map[string]int{}}
	if err != nil {
		return s, err
	}
	for _, o := range prior {
		n, err := s.itemCost(o, 0)
		if err != nil {
			return s, err
		}
		s.priorCost += n
		s.priorCount++
	}
	for _, kind := range []string{"files", "symbols", "routes", "tests", "read_plan"} {
		n, err := s.itemCost(Omission{Kind: kind, Count: 1, Reason: "budget or item limit"}, 0)
		if err != nil {
			return s, err
		}
		s.omissionCost[kind] = n
	}
	return s, nil
}

func (s jsonSizer) separator() int {
	if s.compact {
		return 1
	}
	// The first pretty-printed item replaces [] with a multiline array. Later
	// items need two fewer bytes than that first-item delta.
	return -2
}

func (s jsonSizer) itemCost(item any, existing int) (int, error) {
	var data []byte
	var err error
	if s.compact {
		data, err = json.Marshal([]any{item})
	} else {
		data, err = json.MarshalIndent([]any{item}, "  ", "  ")
	}
	if err != nil {
		return 0, err
	}
	n := len(data) - 2
	if existing > 0 {
		n += s.separator()
	}
	return n, nil
}

func (s jsonSizer) estimate(selectedBytes int, missing map[string]int) int64 {
	omissionBytes, count := s.priorCost, s.priorCount
	for kind, n := range missing {
		if n > 0 {
			omissionBytes += s.omissionCost[kind] + len(strconv.Itoa(n)) - 1
			count++
		}
	}
	if count > 0 {
		omissionBytes += s.separator() * (count - 1)
	}
	total := s.base + selectedBytes + omissionBytes
	n := tokens.Estimate(int64(total))
	for i := 0; i < 10; i++ {
		next := tokens.Estimate(int64(total + len(strconv.FormatInt(n, 10)) - 1))
		if next == n {
			return n
		}
		n = next
	}
	return n
}
