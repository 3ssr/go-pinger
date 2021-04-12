package ping

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
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
		target:            target,
		count:             count,
		timeout:           timeout,
		interval:          interval,
		packetRecvChannel: make(chan *Packet, 10),
		done:              make(chan interface{}, 1),
		id:                os.Getpid(),
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
	conn, err := icmp.ListenPacket("ip4:icmp", "")
	if err != nil {
		return err
	}

	// 3. 让ip数据包带上ttl值
	err = conn.IPv4PacketConn().SetControlMessage(ipv4.FlagTTL, true)
	if err != nil {
		return err
	}

	defer conn.Close()
	go func() {
		err := p.waitEchoReply(conn)
		if err != nil {
			log.Printf("wait echo reply error %+v\n", err)
		}
	}()

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
			p.done <- ""
			return nil
		}
	}
}

func (p *Pinger) CheckTarget() error {
	if len(p.target) == 0 {
		return fmt.Errorf("ping target cannot be nil")
	}

	ipAddr, err := net.ResolveIPAddr("ip", p.target)
	if err != nil {
		return err
	}

	p.targetAddr = ipAddr

	return nil
}

func (p *Pinger) processPkg(pkg *Packet) error {
	p.statistic.RecvCount++
	p.OnRecv(pkg)
	return nil
}

func (p *Pinger) MatchId(id int) bool {
	return p.id == id
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
				return errors.New("not a echo reply packet")
			}

			// 判断icmp响应的id
			switch body := msg.Body.(type) {
			case *icmp.Echo:
				if !p.MatchId(body.ID) {
					return fmt.Errorf("id not match")
				}

				timeData := body.Data[0:8]
				sendTime := bytesToTime(timeData)

				p.packetRecvChannel <- &Packet{
					Bytes:  bytes,
					NBytes: dataLength,
					Seq:    body.Seq,
					Ttl:    cm.TTL,
					Rtt:    time.Now().Sub(sendTime),
					IPAddr: cm.Src.String(),
				}
			default:
				return fmt.Errorf("invalid ICMP echo reply; type: '%T'", body)
			}
		}
	}
}

func (p *Pinger) sendEchoRequest(conn *icmp.PacketConn) error {
	data := []byte{}
	// 将当前时间放入data中，用来计算rtt
	data = append(timeToBytes(time.Now()))

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
		return fmt.Errorf("send echo request failed %v\n", err)
	}

	p.statistic.SentCount++
	p.sequence++

	return nil
}

func bytesToTime(b []byte) time.Time {
	var nsec int64
	for i := uint8(0); i < 8; i++ {
		nsec += int64(b[i]) << ((7 - i) * 8)
	}
	return time.Unix(nsec/1000000000, nsec%1000000000)
}

func timeToBytes(t time.Time) []byte {
	nsec := t.UnixNano()
	b := make([]byte, 8)
	for i := uint8(0); i < 8; i++ {
		b[i] = byte((nsec >> ((7 - i) * 8)) & 0xff)
	}
	return b
}
