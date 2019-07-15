package main

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/dgraph-io/badger"
)

func StartJob() {
	now := time.Now()
	newT := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	next := newT.AddDate(0, 0, 1)
	//next := now.Add(10 * time.Second)

	logline("start scheduling... next:", next)

	firstTimer := time.NewTimer(next.Sub(now))
	<-firstTimer.C
	firstTimer.Stop()

	duration := 4 * time.Hour
	// 4 hour per schedule
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	scheduleTime := time.Now()
	for {
		logline("start processing jobs...", scheduleTime)
		startJobProcessing()
		logline("next schedule:", scheduleTime.Add(duration))
		scheduleTime = <-ticker.C
	}
}

func startJobProcessing() {
	defer func() {
		err := recover()
		if err != nil {
			logline("processing panic:", err)
		}
	}()

	maxCnt := 10
	domainList := make([]struct {
		Domain *Domain
		Key    []byte
	}, 0, maxCnt)

	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(DomainTablePrefix)
		idx := 0
		for it.Seek(prefix); it.ValidForPrefix(prefix) && idx < maxCnt; it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				domain := new(Domain)
				err := json.Unmarshal(v, domain)
				if err != nil {
					return err
				}
				if domain.Status != IssueAvailable {
					domainList[idx] = struct {
						Domain *Domain
						Key    []byte
					}{Domain: domain, Key: item.Key()}
					idx++
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		logline("processing error.", err)
		return
	}

	for _, v := range domainList {
		switch v.Domain.Status {
		case IssuePending:
			err := jobProcessPending(v.Domain.AccountMail, v.Domain)
			if err != nil {
				logline("process domain:", v.Domain.Domain, "error.", err)
			}
		default:
			logline("unknown domain status:", v.Domain.Status)
		}
	}
}

func jobProcessPending(mail string, domain *Domain) error {
	var accountData []byte
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(AccountTable(mail))
		if err != nil {
			return err
		}
		err = item.Value(func(val []byte) error {
			accountData = val
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logline("query error:", err)
		return err
	}

	accData, err := base64.StdEncoding.DecodeString(string(accountData))
	if err != nil {
		logline("base64 decode error:", err)
		return err
	}
	acc := new(Account)
	err = json.Unmarshal(accData, acc)
	if err != nil {
		logline("json unmarshal error:", err)
		return err
	}

	acc, err = client.LoadAccount(acc)
	if err != nil {
		logline("load account error:", err)
		return err
	}

	//TODO do acme

	return nil
}
