package main

import (
	"net/netip"
	"path/filepath"
	"strings"
)

// Split-tunnel modes. "exclude": everything uses the tunnel except the list.
// "include": only the list uses the tunnel, the rest goes out directly.
const (
	modeExclude = "exclude"
	modeInclude = "include"
)

type rules struct {
	appMode    string
	appPaths   map[string]bool // full paths, lower case
	appNames   map[string]bool // bare exe names, lower case
	domainMode string
	suffixes   []string // "example.com" matches example.com and *.example.com
	prefixes   []netip.Prefix
}

func compileRules(s Settings) *rules {
	r := &rules{appMode: s.AppMode, domainMode: s.DomainMode, appPaths: map[string]bool{}, appNames: map[string]bool{}}
	for _, a := range s.Apps {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if strings.ContainsAny(a, `\/`) {
			r.appPaths[filepath.Clean(a)] = true
		} else {
			r.appNames[a] = true
		}
	}
	for _, d := range s.Domains {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(strings.TrimPrefix(d, "*"), ".")
		d = strings.TrimSuffix(d, ".")
		if d == "" {
			continue
		}
		if p, err := netip.ParsePrefix(d); err == nil {
			r.prefixes = append(r.prefixes, p.Masked())
		} else if a, err := netip.ParseAddr(d); err == nil {
			r.prefixes = append(r.prefixes, netip.PrefixFrom(a, a.BitLen()))
		} else {
			r.suffixes = append(r.suffixes, d)
		}
	}
	return r
}

func (r *rules) hasApps() bool    { return len(r.appPaths)+len(r.appNames) > 0 }
func (r *rules) hasDomains() bool { return len(r.suffixes)+len(r.prefixes) > 0 }

// needsDomain: some rule depends on the destination name / IP.
func (r *rules) needsDomain() bool { return r.hasDomains() }

func (r *rules) appListed(path string) bool {
	if path == "" {
		return false
	}
	p := strings.ToLower(path)
	return r.appPaths[p] || r.appNames[filepath.Base(p)]
}

func (r *rules) domainListed(domain string, ip netip.Addr) bool {
	for _, p := range r.prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if domain == "" {
		return false
	}
	for _, s := range r.suffixes {
		if domain == s || strings.HasSuffix(domain, "."+s) {
			return true
		}
	}
	return false
}

// appDirect: should this application bypass the tunnel?
func (r *rules) appDirect(path string) bool {
	if !r.hasApps() {
		return false
	}
	listed := r.appListed(path)
	if r.appMode == modeInclude {
		return !listed
	}
	return listed
}

// domainDirect: should this destination bypass the tunnel?
func (r *rules) domainDirect(domain string, ip netip.Addr) bool {
	if !r.hasDomains() {
		return false
	}
	listed := r.domainListed(domain, ip)
	if r.domainMode == modeInclude {
		return !listed
	}
	return listed
}
