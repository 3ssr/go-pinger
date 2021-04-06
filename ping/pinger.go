package ping

import (
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type Pinger struct {
	target       string
	pingCount    int
	timeout      time.Duration // 每一次ping的超时时长
	interval     time.Duration // 每一次ping的间隔
	targetAddr   *net.IPAddr
	id           int
	sequence     int
	pkgSentCount int
	pkgRecvCount int
	OnRecv       func(*Packet)
	OnFinish     func()
	done         chan interface{} // 用作中断
}

type Packet struct {
	Bytes  []byte
	NBytes int
	IPAddr string
	Seq    int
	Rtt    time.Duration
	Ttl    int
}

func NewPinger(
	target string,
	pingCount int,
	timeout time.Duration,
	interval time.Duration,
) (*Pinger, error) {
	pinger := &Pinger{
		target:    target,
		pingCount: pingCount,
		timeout:   timeout,
		interval:  interval,
	}

	return pinger, nil
}

func (p *Pinger) Run() error {
	if len(p.target) == 0 {
		return fmt.Errorf("ping target cannot be nil")
	}

	err := p.resolveTargetAddr()
	if err != nil {
		return err
	}

	conn, err := icmp.ListenPacket("icmp", "127.0.0.1")
	if err != nil {
		return err
	}

	err = conn.IPv4PacketConn().SetControlMessage(ipv4.FlagTTL, true)
	if err != nil {
		return err
	}

	defer conn.Close()
	defer p.OnFinish()

	timeout := time.NewTicker(p.timeout)
	interval := time.NewTicker(p.interval)
	recvChannel := make(chan *Packet, 5)
	go p.parseIcmpPkg(conn, recvChannel)

	err = p.sendIcmpPkg(conn)
	if err != nil {
		return err
	}

	for {
		select {
		case <-p.done:
			return nil
		case <-timeout.C:
			return nil
		case pkg := <-recvChannel:
			p.processPkg(pkg)
		case <-interval.C:
			if p.pkgSentCount >= p.pingCount {
				interval.Stop()
				continue
			}

			err := p.sendIcmpPkg(conn)
			if err != nil {
				log.Println(err)
			}
		}

		if p.pkgRecvCount >= p.pingCount {
			return nil
		}
	}
}

func (p *Pinger) processPkg(pkg *Packet) {

}

func (p *Pinger) parseIcmpPkg(conn *icmp.PacketConn, recvChannel chan *Packet) error {
	for {
		select {
		case <-p.done:
			return nil
		default:
			bytes := make([]byte, 1000)
			err := conn.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
			if err != nil {
				return err
			}

			n, cm, _, err := conn.IPv4PacketConn().ReadFrom(bytes)
			if err != nil {
				return err
			}

			select {
			case <-p.done:
				return nil
			case recvChannel <- &Packet{NBytes: n, Bytes: bytes, Ttl: cm.TTL}:
			}
		}
	}
}

func (p *Pinger) resolveTargetAddr() error {
	ipAddr, err := net.ResolveIPAddr("ip", p.target)
	if err != nil {
		return err
	}

	p.targetAddr = ipAddr
	return nil
}

func (p *Pinger) sendIcmpPkg(conn *icmp.PacketConn) error {
	data := []byte{}

	// t := append(timeToBytes(time.Now()), uintToBytes(p.Tracker)...)
	// if remainSize := p.Size - timeSliceLength - trackerLength; remainSize > 0 {
	// 	t = append(t, bytes.Repeat([]byte{1}, remainSize)...)
	// }

	body := icmp.Echo{
		ID:   p.id,
		Seq:  p.sequence,
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

	_, err = conn.WriteTo(msgData, p.targetAddr)
	if err != nil {
		return err
	}

	p.pkgSentCount++
	p.sequence++

	return nil
}
