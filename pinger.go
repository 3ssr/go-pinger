package ping

import (
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// ICMP type 8, Echo request

const (
	timeSliceLength = 8
	trackerLength   = 8
)

type Pinger struct {
	target            string           // ping的目标地址
	count             int              // ping次数
	timeout           time.Duration    // 每一次ping的超时时长
	interval          time.Duration    // 每一次ping的间隔
	targetAddr        *net.IPAddr      // 解析后的ping地址
	id                int              // icmp echo request协议中的identifier
	sequence          int              // icmp echo request协议中的sequence
	statistic         PingStatistic    // ping的统计信息
	done              chan interface{} // 用作中断
	packetRecvChannel chan *Packet     // icmp响应包处理channel
	OnRecv            func(*Packet)
	OnFinish          func(statistic PingStatistic)
}

type PingStatistic struct {
	SentCount int // 发送包的数量
	RecvCount int // 收到的包的数量
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
	count int,
	timeout time.Duration,
	interval time.Duration,
) (*Pinger, error) {
	pinger := &Pinger{
		target:   target,
		count:    count,
		timeout:  timeout,
		interval: interval,
	}

	return pinger, nil
}

func (p *Pinger) Run() error {
	defer func() {
		if p.OnFinish != nil {
			p.OnFinish(p.statistic)
		}
	}()

	// 1. 检查ping地址合法性
	err := p.CheckTarget()
	if err != nil {
		return err
	}

	// 2. 监听ping的响应包
	conn, err := icmp.ListenPacket("ip4:icmp", "127.0.0.1")
	if err != nil {
		return err
	}

	// 3. 让ip数据包带上ttl值
	err = conn.IPv4PacketConn().SetControlMessage(ipv4.FlagTTL, true)
	if err != nil {
		return err
	}

	defer conn.Close()
	go p.waitEchoReply(conn)

	// 4. 声明超时定时器
	timeout := time.NewTicker(p.timeout)
	// 5. 声明定时探测定时器
	interval := time.NewTicker(p.interval)

	err = p.sendEchoRequest(conn)
	if err != nil {
		return err
	}

	for {
		select {
		case <-p.done:
			return nil
		case <-timeout.C:
			return nil
		case pkg := <-p.packetRecvChannel:
			p.processPkg(pkg)
		case <-interval.C:
			if p.statistic.SentCount >= p.count {
				interval.Stop()
				continue
			}

			err := p.sendEchoRequest(conn)
			if err != nil {
				log.Println(err)
			}
		}

		if p.statistic.RecvCount >= p.count {
			return nil
		}
	}
}

func (p *Pinger) CheckTarget() error {
	if len(p.target) == 0 {
		return fmt.Errorf("ping target cannot be nil")
	}

	err := p.resolveTargetAddr()
	if err != nil {
		return err
	}

	return nil
}

func (p *Pinger) processPkg(pkg *Packet) error {
	fmt.Print(pkg)
	return nil
}

func (p *Pinger) MatchId(id int) bool {
	return true
}

// 处理收到的icmp包
func (p *Pinger) waitEchoReply(conn *icmp.PacketConn) error {
	for {
		select {
		case <-p.done:
			return nil
		default:
			bytes := make([]byte, 1000)
			// 设置读取超时时长
			// err := conn.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
			// if err != nil {
			// 	return err
			// }

			// 读取响应信息
			dataLength, cm, _, err := conn.IPv4PacketConn().ReadFrom(bytes)
			if err != nil {
				return err
			}

			// 处理icmp响应包
			msg, err := icmp.ParseMessage(1, bytes)
			if err != nil {
				return fmt.Errorf("error parsing icmp message: %w", err)
			}

			// 判断icmp响应类型
			if msg.Type != ipv4.ICMPTypeEchoReply {
				log.Println("not a echo reply packet")
				return nil
			}

			// 判断icmp响应的id
			switch body := msg.Body.(type) {
			case *icmp.Echo:
				if !p.MatchId(body.ID) {
					return fmt.Errorf("id not match")
				}

				p.packetRecvChannel <- &Packet{
					Bytes:  bytes,
					NBytes: dataLength,
					Seq:    body.Seq,
					Ttl:    cm.TTL,
				}
			default:
				return fmt.Errorf("invalid ICMP echo reply; type: '%T'", body)
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

func (p *Pinger) sendEchoRequest(conn *icmp.PacketConn) error {
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

	p.statistic.SentCount++
	p.sequence++

	return nil
}
