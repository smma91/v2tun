//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// English is the source language: every UI / log string is written in English
// and translated through frontend/i18n.json (shared with the web UI).
var (
	i18nOnce sync.Once
	i18nDict map[string]map[string]string
	curLang  atomic.Value // "en", "fa", "ar", "zh"
)

func validLang(l string) string {
	switch l {
	case "en", "fa", "ar", "zh":
		return l
	}
	return "en"
}

func setLang(l string) { curLang.Store(validLang(l)) }

func getLang() string {
	l, _ := curLang.Load().(string)
	return validLang(l)
}

// trError is an error whose text is produced in the language that is current
// when it is shown, not when it was created (logs keep errors around).
type trError struct {
	format string
	args   []any
}

func (e *trError) Error() string { return fmt.Sprintf(T(e.format), e.args...) }

// trErr creates a translatable error from an English format string.
func trErr(format string, args ...any) error { return &trError{format, args} }

var (
	reverseOnce sync.Once
	reverse     map[string]string // any translation -> English key
)

// retranslate turns a message that was stored in some language into the
// current one (e.g. a subscription error saved while another language was on).
func retranslate(s string) string {
	T("") // make sure the dictionary is loaded
	reverseOnce.Do(func() {
		reverse = map[string]string{}
		for k, m := range i18nDict {
			reverse[k] = k
			for _, v := range m {
				reverse[v] = k
			}
		}
	})
	if k, ok := reverse[s]; ok {
		return T(k)
	}
	return s
}

func retranslateArgs(args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		if s, ok := a.(string); ok {
			a = retranslate(s)
		}
		out[i] = a
	}
	return out
}

// T returns the current-language version of an English string.
func T(s string) string {
	l := getLang()
	if l == "en" {
		return s
	}
	i18nOnce.Do(func() {
		if b, err := assets.ReadFile("frontend/i18n.json"); err == nil {
			json.Unmarshal(b, &i18nDict)
		}
	})
	if v := i18nDict[s][l]; v != "" {
		return v
	}
	return s
}
