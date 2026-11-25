package profile

// TestURL is used to measure real delay ("auto" picks the fastest server).
const TestURL = "https://www.gstatic.com/generate_204"

// Outbounds converts servers into sing-box outbounds and endpoints, followed
// by the groups every v2tun core has:
//
//   - "auto": urltest, the fastest working server
//   - "select": the user's choice ("auto" or one server)
//   - "direct"
//
// xhttp, if set, provides the outbound for servers only Xray-core can run
// (VLESS XHTTP); without it they are skipped. present lists the servers that
// made it into the configuration.
func Outbounds(servers []*Server, selected string, xhttp func(*Server) (map[string]any, error)) (outbounds, endpoints []any, present map[string]bool, err error) {
	var tags []string
	present = map[string]bool{}
	for _, s := range servers {
		var ob map[string]any
		var err error
		if NeedsXray(s.Link) {
			if xhttp == nil {
				continue
			}
			ob, err = xhttp(s)
		} else {
			ob, err = s.Outbound()
		}
		if err != nil || ob == nil {
			continue
		}
		ob["tag"] = s.ID
		// WireGuard, OpenVPN and OpenConnect are "endpoints" in sing-box.
		if t := ob["type"]; t == "wireguard" || t == "openvpn-client" || t == "openconnect" {
			endpoints = append(endpoints, ob)
		} else {
			outbounds = append(outbounds, ob)
		}
		tags = append(tags, s.ID)
		present[s.ID] = true
	}
	if len(tags) == 0 {
		return nil, nil, nil, trErr("No usable server in the list")
	}
	if selected == "" || (selected != "auto" && !present[selected]) {
		selected = "auto"
	}
	// Switching servers (auto "fastest" or a manual choice) only affects NEW
	// connections: open ones (downloads, calls, games) keep their server until
	// they end, instead of being cut and timing out. A higher tolerance also
	// stops "auto" from hopping between servers of similar speed.
	outbounds = append(outbounds,
		map[string]any{"type": "urltest", "tag": "auto", "outbounds": tags, "url": TestURL,
			"interval": "3m", "tolerance": 150, "idle_timeout": "30m", "interrupt_exist_connections": false},
		map[string]any{"type": "selector", "tag": "select", "outbounds": append([]string{"auto"}, tags...),
			"default": selected, "interrupt_exist_connections": false},
		map[string]any{"type": "direct", "tag": "direct"},
	)
	return outbounds, endpoints, present, nil
}
