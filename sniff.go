package main

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// sniffHost extracts the target host name from the first bytes a client
// sends: the SNI of a TLS ClientHello, or the Host header of plain HTTP.
func sniffHost(b []byte) string {
	if h := tlsSNI(b); h != "" {
		return h
	}
	return httpHost(b)
}

func tlsSNI(b []byte) string {
	// record header: type(1)=22 handshake, version(2), length(2)
	if len(b) < 5 || b[0] != 0x16 {
		return ""
	}
	b = b[5:]
	// handshake header: type(1)=1 ClientHello, length(3)
	if len(b) < 4 || b[0] != 0x01 {
		return ""
	}
	b = b[4:]
	if len(b) < 34 {
		return ""
	}
	b = b[34:] // client version + random
	skip := func(n int) bool {
		if len(b) < n {
			return false
		}
		b = b[n:]
		return true
	}
	if len(b) < 1 || !skip(1+int(b[0])) { // session id
		return ""
	}
	if len(b) < 2 || !skip(2+int(binary.BigEndian.Uint16(b))) { // cipher suites
		return ""
	}
	if len(b) < 1 || !skip(1+int(b[0])) { // compression methods
		return ""
	}
	if len(b) < 2 {
		return ""
	}
	ext := b[2:]
	if n := int(binary.BigEndian.Uint16(b)); n < len(ext) {
		ext = ext[:n]
	}
	for len(ext) >= 4 {
		typ := binary.BigEndian.Uint16(ext)
		l := int(binary.BigEndian.Uint16(ext[2:]))
		if len(ext) < 4+l {
			return ""
		}
		data := ext[4 : 4+l]
		ext = ext[4+l:]
		if typ != 0 { // server_name
			continue
		}
		if len(data) < 2 {
			return ""
		}
		list := data[2:]
		for len(list) >= 3 {
			nt := list[0]
			nl := int(binary.BigEndian.Uint16(list[1:]))
			if len(list) < 3+nl {
				return ""
			}
			if nt == 0 {
				return string(list[3 : 3+nl])
			}
			list = list[3+nl:]
		}
	}
	return ""
}

func httpHost(b []byte) string {
	i := bytes.IndexByte(b, ' ')
	if i < 3 || i > 7 { // GET, POST, PUT, HEAD, DELETE, OPTIONS, PATCH
		return ""
	}
	for _, line := range strings.Split(string(b), "\r\n")[1:] {
		if len(line) > 5 && strings.EqualFold(line[:5], "host:") {
			h := strings.TrimSpace(line[5:])
			if j := strings.LastIndexByte(h, ':'); j > 0 && !strings.Contains(h[j:], "]") {
				h = h[:j]
			}
			return h
		}
	}
	return ""
}
