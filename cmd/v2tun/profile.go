//go:build windows || linux

package main

import (
	"path/filepath"

	"v2tun/internal/profile"
)

// Servers, links and subscriptions live in internal/profile (shared with the
// mobile apps); these names keep the desktop code as it was.
type (
	Server       = profile.Server
	Subscription = profile.Subscription
	ServerStore  = profile.ServerStore
	serverDB     = profile.DB
)

var (
	loadServerDB   = profile.Load
	newServer      = profile.NewServer
	isSubURL       = profile.IsSubURL
	SplitSubURLs   = profile.SplitSubURLs
	fetchSub       = profile.FetchSub
	parseLink      = profile.ParseLink
	splitLinks     = profile.SplitLinks
	ovpnFileToLink = profile.OvpnFileToLink
	needsXray      = profile.NeedsXray
	firstNonEmpty  = profile.FirstNonEmpty
	truthy         = profile.Truthy
	atoiDef        = profile.AtoiDef
	str            = profile.Str
	sub            = profile.Sub
)

func init() {
	profile.StorePath = func() string { return filepath.Join(dataDir(), "servers.json") }
	profile.WriteFile = writeSecret
}

func storePath() string { return profile.StorePath() }
