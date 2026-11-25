// Package profile holds what v2tun knows about proxy servers: parsing share
// links, subscriptions (base64, sing-box, Clash), OpenVPN profiles, and the
// stored server list. It is shared by the desktop program and the mobile apps.
package profile

import "v2tun/internal/i18n"

// trErr creates a translatable error from an English format string.
func trErr(format string, args ...any) error { return i18n.Err(format, args...) }
