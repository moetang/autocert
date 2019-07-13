package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dgraph-io/badger"
)

var client *AcmeClient
var db *badger.DB

func main() {

	//TODO production
	client = newAcmeClient(false)

	// certificate output
	err := os.MkdirAll(strings.Join([]string{".", "certs"}, string(os.PathSeparator)), os.FileMode(0755))
	if err != nil {
		panic(err)
	}

	// init db
	badgerDb, err := badger.Open(badger.DefaultOptions(strings.Join([]string{".", "store"}, string(os.PathSeparator))))
	if err != nil {
		panic(err)
	}
	db = badgerDb
	var dbOpen = true
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
		again:
			err := db.RunValueLogGC(0.7)
			if err == nil {
				goto again
			}
		}
	}()
	defer func() {
		err = db.Close()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "closing error:", err)
		}
	}()

	//TODO
	startHttp(":8085")

	// waiting for exit signal
	waitSignal()
}

func waitSignal() {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		done <- true
	}()
	<-done
}
