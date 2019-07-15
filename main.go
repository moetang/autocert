package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
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
	var dbOpenLock = new(sync.Mutex)
	go func() {
		ticker := time.NewTicker(61 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			lsmSize1, vlogSize1 := badgerDb.Size()
			dbOpenLock.Lock()
			if !dbOpen {
				dbOpenLock.Unlock()
				return
			}
		again:
			err := db.RunValueLogGC(0.7)
			if err == nil {
				goto again
			}
			lsmSize2, vlogSize2 := badgerDb.Size()
			fmt.Printf("DB_GC: badger before GC, LSM %d, vlog %d. after GC, LSM %d, vlog %d\n", lsmSize1, vlogSize1, lsmSize2, vlogSize2)
			dbOpenLock.Unlock()
		}
	}()
	defer func() {
		fmt.Println("closing database...")
		dbOpenLock.Lock()
		dbOpen = false
		err = db.Close()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "closing error:", err)
		}
		dbOpenLock.Unlock()
	}()

	//TODO need configure listening address
	go startHttp(":8085")
	go StartJob()

	fmt.Println("server started.")

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
