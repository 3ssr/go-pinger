package pinger

import (
	"fmt"
	"net"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type Pinger struct {
	Target    string
	PingCount int

	targetAddr *net.IPAddr
	id         int
	sequence   int

	pkgSentCount int
}

func NewPinger(target string) (*Pinger, error) {
	pinger := &Pinger{
		Target:    target,
		PingCount: 5,
	}

	return pinger, nil
}

func (pinger *Pinger) Run() error {
	if len(pinger.Target) == 0 {
		return fmt.Errorf("ping target cannot be nil")
	}

	err := pinger.resolveAddr()
	if err != nil {
		return err
	}

	icmp.ListenPacket("ip4")

	return nil
}

func (pinger *Pinger) resolveAddr() error {
	ipAddr, err := net.ResolveIPAddr("ip", pinger.Target)
	if err != nil {
		return err
	}

	pinger.targetAddr = ipAddr
	return nil
}

func (pinger *Pinger) sendIcmp(conn *icmp.PacketConn) error {
	data := []byte{}
	body := icmp.Echo{
		ID:   pinger.id,
		Seq:  pinger.sequence,
		Data: data,
	}

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &body,
	}

	msgData, err := msg.Marshal(nil)
	if err != nil {
		return err
	}

	_, err = conn.WriteTo(msgData, pinger.targetAddr)
	if err != nil {
		return err
	}

	pinger.pkgSentCount++
	pinger.sequence++

	return nil
}
