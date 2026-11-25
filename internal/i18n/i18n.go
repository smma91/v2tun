// Package i18n translates v2tun's texts. English is the source language: every
// UI / log string is written in English and translated through i18n.json
// (English key -> fa / ar / zh), which each program provides with SetSource.
package i18n

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	source  atomic.Value // func() []byte
	once    sync.Once
	dict    map[string]map[string]string
	curLang atomic.Value // "en", "fa", "ar", "zh"
)

// SetSource tells where the dictionary comes from (read on first use).
func SetSource(f func() []byte) { source.Store(f) }

func load() {
	once.Do(func() {
		if f, ok := source.Load().(func() []byte); ok {
			json.Unmarshal(f(), &dict)
		}
	})
}

func ValidLang(l string) string {
	switch l {
	case "en", "fa", "ar", "zh":
		return l
	}
	return "en"
}

func SetLang(l string) { curLang.Store(ValidLang(l)) }

func Lang() string {
	l, _ := curLang.Load().(string)
	return ValidLang(l)
}

// T returns the current-language version of an English string.
func T(s string) string {
	l := Lang()
	if l == "en" {
		return s
	}
	load()
	if v := dict[s][l]; v != "" {
		return v
	}
	return s
}

// translated is an error whose text is produced in the language that is
// current when it is shown, not when it was created (logs keep errors).
type translated struct {
	format string
	args   []any
}

func (e *translated) Error() string { return fmt.Sprintf(T(e.format), e.args...) }

// Err creates a translatable error from an English format string.
func Err(format string, args ...any) error { return &translated{format, args} }

var (
	reverseOnce sync.Once
	reverse     map[string]string // any translation -> English key
)

// Retranslate turns a message that was stored in some language into the
// current one (e.g. a subscription error saved while another language was on).
func Retranslate(s string) string {
	load()
	reverseOnce.Do(func() {
		reverse = map[string]string{}
		for k, m := range dict {
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

func RetranslateArgs(args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		if s, ok := a.(string); ok {
			a = Retranslate(s)
		}
		out[i] = a
	}
	return out
}
