package main

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger"
)

func StartJob() {

	logline("start scheduling job...")

	duration := 30 * time.Minute
	// 4 hour per schedule
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	for {
		scheduleTime := <-ticker.C
		logline("start processing jobs...", scheduleTime)
		startJobProcessing()
		logline("next schedule:", scheduleTime.Add(duration))
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
					domainList = append(domainList, struct {
						Domain *Domain
						Key    []byte
					}{Domain: domain, Key: item.Key()})
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
			logline("[job] start processing pending domain:", v.Domain.Domain)
			err := jobProcessPending(v.Domain.AccountMail, v.Domain)
			if err != nil {
				logline("process pending domain:", v.Domain.Domain, "error.", err)
			}
		case IssueChallenging:
			logline("[job] start processing challenging domain:", v.Domain.Domain)
			err := jobProcessChallenging(v.Domain.AccountMail, v.Domain)
			if err != nil {
				logline("process challenging domain:", v.Domain.Domain, "error.", err)
			}
		case IssueAvailable:
			//TODO check issue time. if already 4 months, try to reissue
			logline("[job] start processing available domain:", v.Domain.Domain)
		default:
			logline("unknown domain status:", v.Domain.Status)
		}
	}
}

func jobProcessChallenging(mail string, domain *Domain) error {
	acc, err := QueryAccountByMail(mail)
	if err != nil {
		logline("invoke QueryAccountByMail error:", err)
		return err
	}

	//acc, err = client.LoadAccount(acc)
	//if err != nil {
	//	logline("load account error:", err)
	//	return err
	//}

	priv, cert, err := client.UpdateChallenge(acc, domain)
	if err != nil {
		logline("update challeging error when do acme operations:", err)
		// rollback status to pending in order to redo challenge work
		// If we can recognize whether we should redo challenge, this code could be changed
		domain.Status = IssuePending
		// update db
		domainData, _ := json.Marshal(domain)
		err2 := db.Update(func(txn *badger.Txn) error {
			return txn.Set(DomainTable(domain.Domain), domainData)
		})
		if err2 != nil {
			logline("update domain to pending error for domain rollback:", domain.Domain)
			return err2
		}
		return err
	}

	//TODO need remove
	logline("private file:" + string(priv))
	logline("cert file:" + string(cert))

	// write files
	err = WritePemPrivateKeyFile(filepath.Join("certs", domain.Domain+"_"+time.Now().Format(time.RFC3339Nano))+".key", priv)
	if err != nil {
		logline("write private key error.", err)
		return err
	}
	err = WritePemCertFile(filepath.Join("certs", domain.Domain+"_"+time.Now().Format(time.RFC3339Nano))+".cert", priv)
	if err != nil {
		logline("write cert error.", err)
		return err
	}

	domain.Status = IssueAvailable
	domain.IssueTime = time.Now().Format(time.RFC3339Nano)
	// update db
	domainData, _ := json.Marshal(domain)
	err = db.Update(func(txn *badger.Txn) error {
		return txn.Set(DomainTable(domain.Domain), domainData)
	})
	if err != nil {
		logline("update domain to available error for domain:", domain.Domain)
		return err
	}
	return nil
}

func jobProcessPending(mail string, domain *Domain) error {
	acc, err := QueryAccountByMail(mail)
	if err != nil {
		logline("invoke QueryAccountByMail error:", err)
		return err
	}

	acc, err = client.LoadAccount(acc)
	if err != nil {
		logline("load account error:", err)
		return err
	}

	orderdata, chaldata, token, err := client.AcquireChallenging(acc, domain)
	if err != nil {
		logline("acquire challenging error:", err)
		return err
	}
	domain.ChallengeData = string(chaldata)
	domain.OrderData = string(orderdata)
	//TODO update token to dns provider
	logline("acquired token:", token)

	// update status to challenging
	domain.Status = IssueChallenging

	// update db
	domainData, _ := json.Marshal(domain)
	err = db.Update(func(txn *badger.Txn) error {
		return txn.Set(DomainTable(domain.Domain), domainData)
	})
	if err != nil {
		logline("update domain to challenging error for domain:", domain.Domain)
		return err
	}

	return nil
}
