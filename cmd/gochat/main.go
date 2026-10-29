package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/hamza3256/gochat/internal/engine"
)

func main() {
	listen := flag.String("listen", ":8080", "IPv4 listen address (host:port)")
	internal := flag.String("internal", ":6060", "health, stats, and pprof address")
	reactors := flag.Int("reactors", 0, "reactor threads (0 = GOMAXPROCS)")
	maxConns := flag.Int("max-conns", 65536, "fd-indexed connection slab size")
	readBuf := flag.Int("read-buf", 4096, "per-connection read buffer size")
	flag.Parse()

	if *reactors <= 0 {
		*reactors = runtime.GOMAXPROCS(0)
	}

	srv, err := engine.NewServer(engine.Config{
		Addr:        *listen,
		NumReactors: *reactors,
		MaxConns:    *maxConns,
		ReadBufSize: *readBuf,
	})
	if err != nil {
		log.Fatal(err)
	}
	srv.Start()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(srv.Snapshot())
	})
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	go func() {
		log.Printf("internal %s", *internal)
		if err := http.ListenAndServe(*internal, mux); err != nil {
			log.Printf("internal server: %v", err)
		}
	}()

	log.Printf("gochat listening %s reactors=%d max-conns=%d go=%s",
		srv.Addr(), *reactors, *maxConns, runtime.Version())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Fprintln(os.Stderr, "shutting down")
	srv.Close()
}
