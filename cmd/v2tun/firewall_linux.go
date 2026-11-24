//go:build linux

package main

import (
	"net/netip"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

// nftables tables owned by v2tun. They are kernel state that outlives the
// process, so they are also removed by the watchdog and at the next start
// (cleanupNetwork).
const (
	ksTable  = "v2tun"     // kill switch / leak guard (inet)
	dnsTable = "v2tun_dns" // DNS capture (ip)
)

type killSwitch struct{ on bool }

const ipsDstNAT = 1 << 5 // conntrack status bit IPS_DST_NAT

func ifname(n string) []byte {
	b := make([]byte, 16)
	copy(b, n+"\x00")
	return b
}

func oif(op expr.CmpOp, name string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: op, Register: 1, Data: ifname(name)},
	}
}

func markIs(op expr.CmpOp) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyMARK, Register: 1},
		&expr.Cmp{Op: op, Register: 1, Data: binaryutil.NativeEndian.PutUint32(directMark)},
	}
}

func family(f byte) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{f}},
	}
}

// daddrIn matches the destination address against a prefix (IPv4 or IPv6).
func daddrIn(p netip.Prefix) []expr.Any {
	f, off, n := byte(unix.NFPROTO_IPV4), uint32(16), uint32(4)
	if p.Addr().Is6() {
		f, off, n = unix.NFPROTO_IPV6, 24, 16
	}
	mask := make([]byte, n)
	for i := 0; i < p.Bits(); i++ {
		mask[i/8] |= 0x80 >> (i % 8)
	}
	return append(family(f),
		&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: off, Len: n},
		&expr.Bitwise{SourceRegister: 1, DestRegister: 1, Len: n, Mask: mask, Xor: make([]byte, n)},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: p.Masked().Addr().AsSlice()},
	)
}

func l4(proto byte) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{proto}},
	}
}

func dport(port uint16) []expr.Any {
	return []expr.Any{
		&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.BigEndian.PutUint16(port)},
	}
}

func join(parts ...[]expr.Any) []expr.Any {
	var out []expr.Any
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

var (
	accept = []expr.Any{&expr.Verdict{Kind: expr.VerdictAccept}}
	// reject (not drop): apps fail at once instead of waiting for a timeout.
	reject = []expr.Any{&expr.Reject{Type: unix.NFT_REJECT_ICMPX_UNREACH, Code: unix.NFT_REJECT_ICMPX_ADMIN_PROHIBITED}}
)

func deleteTable(c *nftables.Conn, fam nftables.TableFamily, name string) {
	c.DelTable(&nftables.Table{Family: fam, Name: name})
	c.Flush() // fails harmlessly when the table does not exist
}

// enableKillSwitch: everything that does not leave through the tunnel is
// rejected, except loopback, v2tun's own marked connections (the built-in
// core, direct rules), DHCP and IPv6 link-local traffic, and optionally the
// local network. Without "full" only the leaks are closed: IPv6 to the
// internet and DNS outside the tunnel. apps (programs allowed out on Windows)
// is not needed: here every program's traffic enters the tunnel first and
// v2tun forwards what must go direct itself, with the mark.
func enableKillSwitch(tunLUID uint64, apps []string, allowLAN, full bool) (*killSwitch, error) {
	c, err := nftables.New()
	if err != nil {
		return nil, err
	}
	deleteTable(c, nftables.TableFamilyINet, ksTable)
	t := c.AddTable(&nftables.Table{Family: nftables.TableFamilyINet, Name: ksTable})
	policy := nftables.ChainPolicyAccept
	ch := c.AddChain(&nftables.Chain{Name: "output", Table: t, Type: nftables.ChainTypeFilter,
		Hooknum: nftables.ChainHookOutput, Priority: nftables.ChainPriorityFilter, Policy: &policy})
	add := func(e []expr.Any) { c.AddRule(&nftables.Rule{Table: t, Chain: ch, Exprs: e}) }

	add(join(oif(expr.CmpOpEq, "lo"), accept))
	add(join(oif(expr.CmpOpEq, tunName), accept))
	add(join(markIs(expr.CmpOpEq), accept))
	// DNS redirected into the tunnel by captureDNS: the packet is rerouted to
	// the tunnel, but the hook still reports the original interface.
	add(join([]expr.Any{
		&expr.Ct{Register: 1, Key: expr.CtKeySTATUS},
		&expr.Bitwise{SourceRegister: 1, DestRegister: 1, Len: 4,
			Mask: binaryutil.NativeEndian.PutUint32(ipsDstNAT), Xor: make([]byte, 4)},
		&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: make([]byte, 4)},
	}, accept))
	add(join(l4(unix.IPPROTO_UDP), dport(67), accept)) // DHCP
	for _, p := range []string{"fe80::/10", "ff00::/8"} {
		add(join(daddrIn(netip.MustParsePrefix(p)), accept))
	}
	if full {
		if allowLAN {
			for _, p := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "224.0.0.0/4", "255.255.255.255/32", "fc00::/7"} {
				add(join(daddrIn(netip.MustParsePrefix(p)), accept))
			}
		}
		add(reject)
	} else {
		add(join(family(unix.NFPROTO_IPV6), reject))
		add(join(l4(unix.IPPROTO_UDP), dport(53), reject))
		add(join(l4(unix.IPPROTO_TCP), dport(53), reject))
	}
	if err := c.Flush(); err != nil {
		return nil, err
	}
	return &killSwitch{on: true}, nil
}

func (k *killSwitch) Close() {
	if k == nil || !k.on {
		return
	}
	if c, err := nftables.New(); err == nil {
		deleteTable(c, nftables.TableFamilyINet, ksTable)
	}
	k.on = false
}

// captureDNS sends every DNS query that would leave through the real network
// (for example systemd-resolved asking the router) into the tunnel instead,
// to the given server: the tunnel answers it (FakeIP) or forwards it through
// the proxy. v2tun's own marked queries are left alone.
func captureDNS(target netip.Addr) error {
	c, err := nftables.New()
	if err != nil {
		return err
	}
	deleteTable(c, nftables.TableFamilyIPv4, dnsTable)
	t := c.AddTable(&nftables.Table{Family: nftables.TableFamilyIPv4, Name: dnsTable})
	out := c.AddChain(&nftables.Chain{Name: "output", Table: t, Type: nftables.ChainTypeNAT,
		Hooknum: nftables.ChainHookOutput, Priority: nftables.ChainPriorityNATDest})
	// Older kernels only enable NAT with a postrouting NAT chain present.
	c.AddChain(&nftables.Chain{Name: "postrouting", Table: t, Type: nftables.ChainTypeNAT,
		Hooknum: nftables.ChainHookPostrouting, Priority: nftables.ChainPriorityNATSource})
	to := target.As4()
	for _, proto := range []byte{unix.IPPROTO_UDP, unix.IPPROTO_TCP} {
		c.AddRule(&nftables.Rule{Table: t, Chain: out, Exprs: join(
			oif(expr.CmpOpNeq, "lo"), oif(expr.CmpOpNeq, tunName), markIs(expr.CmpOpNeq),
			l4(proto), dport(53),
			[]expr.Any{
				&expr.Immediate{Register: 1, Data: to[:]},
				&expr.NAT{Type: expr.NATTypeDestNAT, Family: unix.NFPROTO_IPV4, RegAddrMin: 1},
			})})
	}
	return c.Flush()
}

func releaseDNS() {
	if c, err := nftables.New(); err == nil {
		deleteTable(c, nftables.TableFamilyIPv4, dnsTable)
	}
}

// cleanupNetwork removes firewall state left by a v2tun that was killed.
func cleanupNetwork() {
	if !hasPrivileges() {
		return
	}
	if c, err := nftables.New(); err == nil {
		deleteTable(c, nftables.TableFamilyINet, ksTable)
		deleteTable(c, nftables.TableFamilyIPv4, dnsTable)
	}
}
