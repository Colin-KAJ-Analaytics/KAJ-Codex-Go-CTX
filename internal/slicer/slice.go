package slicer

import (
	"fmt"
	"os"
	"strings"

	"goctx/internal/indexer"
)

type Result struct {
	Source          string
	Matches         []indexer.SymbolInfo
	Available       []indexer.SymbolInfo
	Ambiguous       bool
	NotFound        bool
	RequestedSymbol string
}

func SliceFile(path, symbol string) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	syms, err := indexer.SymbolsInFile(path)
	if err != nil {
		return Result{}, err
	}
	var matches []indexer.SymbolInfo
	for _, s := range syms {
		if matchesSymbol(s, symbol) {
			matches = append(matches, s)
		}
	}
	res := Result{Available: syms, Matches: matches, RequestedSymbol: symbol}
	if len(matches) == 0 {
		res.NotFound = true
		return res, nil
	}
	if len(matches) > 1 {
		res.Ambiguous = true
		return res, nil
	}
	lines := strings.SplitAfter(string(data), "\n")
	start := matches[0].StartLine
	end := matches[0].EndLine
	if start < 1 || end < start || start > len(lines) {
		return Result{}, fmt.Errorf("invalid symbol span for %s", symbol)
	}
	if end > len(lines) {
		end = len(lines)
	}
	res.Source = strings.Join(lines[start-1:end], "")
	return res, nil
}

func matchesSymbol(s indexer.SymbolInfo, query string) bool {
	query = strings.TrimSpace(query)
	if query == s.Name || query == indexer.DisplayName(s) {
		return true
	}
	if s.Receiver == "" {
		return false
	}
	recv := strings.TrimPrefix(strings.TrimSuffix(s.Receiver, ")"), "(*")
	return query == recv+"."+s.Name || query == "(*"+recv+")."+s.Name
}
