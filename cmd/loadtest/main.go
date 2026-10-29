package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hamza3256/gochat/internal/wsclient"
	"github.com/hamza3256/gochat/pkg/hist"
	"github.com/hamza3256/gochat/pkg/wire"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "server host:port")
	conns := flag.Int("connections", 1000, "concurrent WebSocket connections")
	ramp := flag.Int("ramp-rate", 0, "new connections per second (0 = all at once)")
	msgRate := flag.Float64("msg-rate", 0, "messages per connection per second (0 = as fast as possible)")
	duration := flag.Duration("duration", 5*time.Second, "send window after connections are up")
	payload := flag.Int("payload", 64, "opaque payload bytes (timestamp is first 8)")
	mode := flag.String("mode", "echo", "echo or pub")
	roomSize := flag.Int("room-size", 8, "connections per room in pub mode")
	flag.Parse()

	if *payload < 8 {
		*payload = 8
	}
	if *roomSize < 1 {
		*roomSize = 1
	}

	fmt.Fprintf(os.Stderr, "loadtest addr=%s conns=%d mode=%s duration=%s go=%s\n",
		*addr, *conns, *mode, *duration, runtime.Version())

	startCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		errs    atomic.Uint64
		sent    atomic.Uint64
		recvOK  atomic.Uint64
		samples = make([][]int64, *conns)
		wg      sync.WaitGroup
		ready   sync.WaitGroup
	)
	ready.Add(*conns)

	for i := 0; i < *conns; i++ {
		if *ramp > 0 && i > 0 {
			time.Sleep(time.Second / time.Duration(*ramp))
		}
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			local := make([]int64, 0, 4096)
			defer func() { samples[id] = local }()

			c, err := wsclient.Dial(*addr)
			if err != nil {
				errs.Add(1)
				ready.Done()
				return
			}
			defer c.Close()

			room := uint32(id / *roomSize)
			if *mode == "pub" {
				if err := c.WriteBinary(wire.Append(nil, wire.OpJoin, room, nil)); err != nil {
					errs.Add(1)
					ready.Done()
					return
				}
			}
			ready.Done()
			<-startCh

			var ticker *time.Ticker
			if *msgRate > 0 {
				ticker = time.NewTicker(time.Duration(float64(time.Second) / *msgRate))
				defer ticker.Stop()
			}

			body := make([]byte, *payload)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if ticker != nil {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
				ts := time.Now().UnixNano()
				binary.LittleEndian.PutUint64(body, uint64(ts))
				op := wire.OpEcho
				if *mode == "pub" {
					op = wire.OpPub
				}
				env := wire.Append(nil, op, room, body)
				if err := c.WriteBinary(env); err != nil {
					errs.Add(1)
					return
				}
				sent.Add(1)
				if err := waitOwn(c, uint64(ts)); err != nil {
					errs.Add(1)
					return
				}
				recvOK.Add(1)
				local = append(local, time.Now().UnixNano()-ts)
			}
		}(i)
	}

	ready.Wait()
	fmt.Fprintf(os.Stderr, "connected (errors=%d), sending for %s\n", errs.Load(), *duration)
	close(startCh)
	time.Sleep(*duration)
	cancel()
	wg.Wait()

	var all []int64
	for _, s := range samples {
		all = append(all, s...)
	}
	hist.SortInPlace(all)

	var sum int64
	for _, v := range all {
		sum += v
	}
	avg := int64(0)
	if len(all) > 0 {
		avg = sum / int64(len(all))
	}

	fmt.Printf("connections %d\n", *conns)
	fmt.Printf("errors      %d\n", errs.Load())
	fmt.Printf("sent        %d\n", sent.Load())
	fmt.Printf("matched     %d\n", recvOK.Load())
	fmt.Printf("p50_us      %d\n", toUs(hist.Percentile(all, 50)))
	fmt.Printf("p90_us      %d\n", toUs(hist.Percentile(all, 90)))
	fmt.Printf("p99_us      %d\n", toUs(hist.Percentile(all, 99)))
	fmt.Printf("p999_us     %d\n", toUs(hist.Percentile(all, 99.9)))
	fmt.Printf("max_us      %d\n", toUs(hist.Percentile(all, 100)))
	fmt.Printf("avg_us      %d\n", toUs(avg))
	secs := duration.Seconds()
	if secs > 0 {
		fmt.Printf("msg_per_s   %.0f\n", float64(recvOK.Load())/secs)
	}
}

func waitOwn(c *wsclient.Conn, ts uint64) error {
	for {
		got, err := c.ReadBinary()
		if err != nil {
			return err
		}
		_, _, payload, err := wire.Parse(got)
		if err != nil || len(payload) < 8 {
			continue
		}
		if binary.LittleEndian.Uint64(payload) == ts {
			return nil
		}
	}
}

func toUs(ns int64) int64 {
	return ns / 1000
}
