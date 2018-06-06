package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"./data"

	"github.com/eminom/go-coap"
)

const (
	FILE_SIZE_LIMIT = 300 * 1024 * 1024
	SegmentSize     = 512
)

var (
	IsDigit = regexp.MustCompile(`\d+`)
)

type WorkItem struct {
	dest net.Addr
	buf  []byte
}

type Scatter struct {
	size     int
	segCount int
	oCh      chan *WorkItem
	chunks   []data.Piece
}

func NewScatter(inpath string, ch chan *WorkItem) *Scatter {
	rv := &Scatter{oCh: ch}
	return rv.doInit(inpath)
}

func (s *Scatter) doInit(inpath string) *Scatter {

	stat, err := os.Stat(inpath)
	if err != nil || stat.IsDir() {
		log.Printf("error for %v:%v (or directory)", inpath, err)
		return nil
	}
	if stat.Size() > FILE_SIZE_LIMIT {
		log.Printf("file too large")
		return nil
	}
	s.size = int(stat.Size())
	s.segCount = (s.size + (SegmentSize - 1)) / SegmentSize
	s.chunks = make([]data.Piece, s.segCount)

	fin, err := os.Open(inpath)
	if err != nil {
		log.Printf("error opening %v:%v", inpath, err)
		return nil
	}
	defer fin.Close()
	var buff [SegmentSize]byte
	log.Printf("loading `%v`...", inpath)
	log.Printf("%v byte(s)", s.size)
	log.Printf("%v segment(s)", s.segCount)
	for i := 0; ; i++ {
		n, err := fin.Read(buff[:])
		if err != nil {
			break
		}
		s.chunks[i] = data.NewPiece(buff[:n])
	}

	log.Printf("loading done")
	return s
}

func (s *Scatter) GenForbidden(req coap.Message, addr net.Addr) {

	forb := coap.Message{
		Type:      coap.Acknowledgement,
		Code:      coap.Forbidden,
		MessageID: req.MessageID,
		Token:     req.Token,
		Payload:   []byte("not very well"),
	}
	chunk, err := forb.MarshalBinary()
	if err != nil {
		log.Printf("error marshal: %v", err)
		return
	}
	s.oCh <- &WorkItem{addr, chunk}
}

func (s *Scatter) Dispatch(req coap.Message, from net.Addr) {
	// log.Printf("dispatching")
	paths := req.Path()

	var resp *coap.Message
	(func() {
		if len(paths) >= 2 {
			if paths[0] == "file" {
				if IsDigit.MatchString(paths[1]) {
					if idx, err := strconv.Atoi(paths[1]); nil == err && idx >= 0 && idx < s.segCount {
						if len(paths) >= 3 {
							if sel, err := strconv.Atoi(paths[2]); nil == err && sel >= 0 && sel < 2 {
								// 0 for sig
								switch sel {
								case 0:
									resp = &coap.Message{
										Code:    coap.Content,
										Payload: s.chunks[idx].Sig,
									}
								case 1:
									resp = &coap.Message{
										Code:    coap.Content,
										Payload: s.chunks[idx].Chunk,
									}
								default:
									log.Printf("invalid sub[2]")
								}
							}
						}
					}
				} else {
					switch paths[1] {
					case "segs":
						resp = &coap.Message{
							Code:    coap.Content,
							Payload: []byte(fmt.Sprintf("%v", s.segCount)),
						}
					}
				}
			}
		}
	})()

	if nil == resp {
		resp = &coap.Message{
			Code:    coap.BadRequest,
			Payload: []byte("Reply from CoAP server"),
		}
	}

	// fin
	resp.Token = req.Token
	resp.Type = coap.Acknowledgement
	resp.MessageID = req.MessageID
	ob, err := resp.MarshalBinary()
	if err != nil {
		panic(err)
	}
	s.oCh <- &WorkItem{from, ob}
}

func main() {
	var fAddr = flag.String("addr", ":16666", "address for listening")
	var fContent = flag.String("target", "tmp/bin.7z", "zip")
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Ltime)

	addr, err := net.ResolveUDPAddr("udp", *fAddr)
	if err != nil {
		panic(err)
	}

	startScatter(addr, *fContent)

	log.Printf("bye")
}

func startScatter(addr *net.UDPAddr, inputpath string) {

	ctx, doCancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go runScatter(addr, inputpath, &wg, ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Kill, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Printf("cancelling...")
	doCancel()
	wg.Wait()
}

func runScatter(addr *net.UDPAddr, inpath string, wg *sync.WaitGroup, ctx context.Context) {
	defer func() {
		log.Printf("leaving scatter process")
		wg.Done()
	}()

	uSock, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}

	var bigBuff [1024 * 1024]byte
	workCh := make(chan *WorkItem, 1024)

	scatter := NewScatter(inpath, workCh)
	log.Printf("scatter ready")

	// write-procedure. one by one.
	wg.Add(1)
	go func() {
		defer func() {
			log.Printf("leaving write-proc")
			wg.Done()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case wi := <-workCh:
				uSock.WriteTo(wi.buf, wi.dest)
			}
		}
	}()

	const ParserCount = 4
	bytesDisCh := make(chan *WorkItem, 1024)
	for i := 0; i < ParserCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case wi := <-bytesDisCh:
					msg, err := coap.ParseMessage(wi.buf)
					// TODO
					if err == nil {
						switch msg.Type {
						case coap.Confirmable:
							if msg.Code == coap.GET {
								scatter.Dispatch(msg, wi.dest)
							} else {
								log.Printf("request type not supported: %v", msg.Type)
								scatter.GenForbidden(msg, wi.dest)
							}
						default:
							log.Printf("ignore msg: %v", msg.Type)
						}
					} else {
						log.Printf("not CoAP")
					}
				}
			}
		}()
	}

	// reader procedure
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		uSock.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, from, err := uSock.ReadFrom(bigBuff[:])
		if err != nil {
			if nErr, ok := err.(net.Error); !ok || !nErr.Temporary() {
				log.Printf("network error: %T: %v", err, err)
				return
			}
			continue
		}

		// log.Printf("new req")

		dbuff := bytes.Repeat(bigBuff[:n], 1)
		newWi := &WorkItem{from, dbuff}
	S100:
		for {
			select {
			case bytesDisCh <- newWi:
				break S100
			default:
				log.Printf("buff channel cramped **")
				time.Sleep(1 * time.Second)
			}
		}
	}

}
